package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	profileservice "github.com/mekedron/wolt-cli/internal/service/profile"
)

const tokenRefreshLeeway = 30 * time.Second
const maxSupersededTokenHashes = 16

// ErrNotLoggedIn is returned from auth-required tools when no credentials are
// available in the persisted profile.
var ErrNotLoggedIn = errors.New("not logged in — run 'wolt login' in a terminal to sign in, then retry")
var errRefreshUnavailable = errors.New("no refresh token available")

type tokenRefreshError struct {
	cause error
}

func (e *tokenRefreshError) Error() string {
	return "wolt session refresh failed"
}

func (e *tokenRefreshError) Unwrap() error {
	return e.cause
}

type profileConfigError struct {
	cause error
}

func (e *profileConfigError) Error() string {
	return "profile configuration is unavailable"
}

func (e *profileConfigError) Unwrap() error {
	return e.cause
}

// loadProfile reads the current default profile from disk. A missing config is
// treated the same as a missing profile — callers see ErrNotLoggedIn.
func (tc *ToolCtx) loadProfile(ctx context.Context) (domain.Profile, error) {
	if tc.profiles == nil {
		return domain.Profile{}, ErrNotLoggedIn
	}
	profile, err := tc.profiles.Find(ctx, "")
	if err != nil {
		if errors.Is(err, configstore.ErrConfigNotFound) ||
			errors.Is(err, profileservice.ErrDefaultProfileNotFound) ||
			errors.Is(err, profileservice.ErrProfileNotFound) {
			return domain.Profile{}, ErrNotLoggedIn
		}
		return domain.Profile{}, &profileConfigError{cause: err}
	}
	return profile, nil
}

// buildAuthContext constructs an AuthContext from a profile. Tokens persisted
// to disk by the CLI are already normalized, so no parsing is needed here.
func buildAuthContext(profile domain.Profile) woltgateway.AuthContext {
	return woltgateway.AuthContext{
		WToken:       strings.TrimSpace(profile.WToken),
		RefreshToken: strings.TrimSpace(profile.WRefreshToken),
		Cookies:      append([]string(nil), profile.Cookies...),
	}
}

// optionalAuth returns the persisted auth context if available, or an empty
// one otherwise. Use for tools that benefit from auth (personalized results)
// but work unauthenticated too.
func (tc *ToolCtx) optionalAuth(ctx context.Context) woltgateway.AuthContext {
	profile, err := tc.loadProfile(ctx)
	if err != nil {
		return woltgateway.AuthContext{}
	}
	auth := buildAuthContext(profile)
	tc.applyProcessLocalAuth(&auth)
	return auth
}

// requireAuth returns the persisted profile + auth context, or ErrNotLoggedIn
// if the user has not signed in.
func (tc *ToolCtx) requireAuth(ctx context.Context) (domain.Profile, woltgateway.AuthContext, error) {
	profile, err := tc.loadProfile(ctx)
	if err != nil {
		return domain.Profile{}, woltgateway.AuthContext{}, err
	}
	auth := buildAuthContext(profile)
	tc.applyProcessLocalAuth(&auth)
	if !auth.CanAuthenticate() {
		return domain.Profile{}, woltgateway.AuthContext{}, ErrNotLoggedIn
	}
	return profile, auth, nil
}

// applyProcessLocalAuth overlays the most recent process-local access and
// rotated refresh tokens while the persisted bootstrap snapshot still
// identifies the same chain. A genuinely different persisted snapshot (for
// example after a new login in another process) takes ownership instead.
func (tc *ToolCtx) applyProcessLocalAuth(auth *woltgateway.AuthContext) {
	if auth == nil {
		return
	}
	tc.tokenRefreshMu.Lock()
	defer tc.tokenRefreshMu.Unlock()
	if tc.currentAccessToken == "" {
		return
	}
	if tc.hasProfileAuthHash && authContextHash(*auth) == tc.profileAuthHash {
		auth.WToken = tc.currentAccessToken
		if tc.currentRefreshToken != "" {
			auth.RefreshToken = tc.currentRefreshToken
		}
		return
	}
	tc.clearRefreshedTokenState()
}

// invokeWithRefresh calls fn with the supplied AuthContext. If the access
// token is JWT-expired going in, or fn returns a 401, it rotates tokens in
// memory via the refresh endpoint and retries once.
func invokeWithRefresh[T any](
	ctx context.Context,
	tc *ToolCtx,
	auth *woltgateway.AuthContext,
	fn func(woltgateway.AuthContext) (T, error),
) (T, error) {
	var zero T
	if auth == nil {
		return zero, fmt.Errorf("auth context is nil")
	}
	var proactiveRefreshErr error
	if tokenExpired(auth.WToken, time.Now().UTC(), tokenRefreshLeeway) {
		// The stale-token call may still succeed. If it returns 401, surface
		// this refresh failure instead of immediately hammering the refresh
		// endpoint a second time.
		proactiveRefreshErr = tc.refreshTokens(ctx, auth)
	}

	result, err := fn(*auth)
	if err == nil {
		return result, nil
	}
	if !woltgateway.HasStatus(err, http.StatusUnauthorized) {
		return result, err
	}
	if proactiveRefreshErr != nil {
		if errors.Is(proactiveRefreshErr, errRefreshUnavailable) {
			return zero, err
		}
		return zero, &tokenRefreshError{cause: proactiveRefreshErr}
	}

	if refreshErr := tc.refreshTokens(ctx, auth); refreshErr != nil {
		if errors.Is(refreshErr, errRefreshUnavailable) {
			return zero, err
		}
		return zero, &tokenRefreshError{cause: refreshErr}
	}
	return fn(*auth)
}

func (tc *ToolCtx) refreshTokens(ctx context.Context, auth *woltgateway.AuthContext) error {
	if auth == nil {
		return fmt.Errorf("auth context is nil")
	}

	staleAccess := strings.TrimSpace(auth.WToken)
	tc.tokenRefreshMu.Lock()
	defer tc.tokenRefreshMu.Unlock()

	reused, expected, err := tc.reuseNewerAccessToken(ctx, auth, staleAccess)
	if err != nil {
		return err
	}
	if reused {
		return nil
	}

	refreshToken := strings.TrimSpace(auth.RefreshToken)
	if refreshToken == "" {
		return errRefreshUnavailable
	}
	if tc.wolt == nil {
		return fmt.Errorf("wolt client unavailable")
	}
	result, err := tc.wolt.RefreshAccessToken(ctx, refreshToken, *auth)
	if err != nil {
		return err
	}
	if err := tc.applyTokenRefresh(auth, staleAccess, result); err != nil {
		return err
	}
	if persisted, persistErr := tc.persistRefreshedAccess(
		ctx,
		expected,
		auth.WToken,
	); persistErr != nil {
		tc.logger.Warn("failed to persist refreshed MCP access token", "err", persistErr)
	} else if persisted {
		expected.AccessToken = strings.TrimSpace(auth.WToken)
		tc.profileAuthHash = credentialHash(expected)
		tc.hasProfileAuthHash = true
	}
	return nil
}

// reuseNewerAccessToken avoids rotating a refresh-token chain twice when a
// concurrent request or another process already supplied newer credentials.
// tokenRefreshMu must be held by the caller.
func (tc *ToolCtx) reuseNewerAccessToken(
	ctx context.Context,
	auth *woltgateway.AuthContext,
	staleAccess string,
) (bool, configstore.Credentials, error) {
	// Notice credentials changed by another process while this one waited for
	// the lock. A different persisted token is treated as a fresh external
	// login and a blank profile as an external logout.
	profile, err := tc.loadProfile(ctx)
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			tc.clearRefreshedTokenState()
			*auth = woltgateway.AuthContext{}
			return false, configstore.Credentials{}, ErrNotLoggedIn
		}
		return false, configstore.Credentials{}, err
	}
	persisted := buildAuthContext(profile)
	persistedCredentials := configstore.CredentialsFromProfile(profile)
	if !persisted.CanAuthenticate() {
		tc.clearRefreshedTokenState()
		*auth = woltgateway.AuthContext{}
		return false, configstore.Credentials{}, ErrNotLoggedIn
	}
	persistedAccess := strings.TrimSpace(persisted.WToken)
	persistedHash := authContextHash(persisted)
	expectedHash := authContextHash(*auth)
	sameProfileChain := persistedHash == expectedHash
	if tc.hasProfileAuthHash {
		sameProfileChain = persistedHash == tc.profileAuthHash
	}
	if !sameProfileChain {
		// Explicit login/logout replaced the persisted bootstrap credentials.
		// Adopt them without allowing this process's old chain to leak across.
		tc.clearRefreshedTokenState()
		*auth = persisted
		if persistedAccess == "" {
			return strings.TrimSpace(persisted.RefreshToken) == "",
				persistedCredentials,
				nil
		}
		if persistedAccess == staleAccess && strings.TrimSpace(persisted.RefreshToken) != "" {
			return false, persistedCredentials, nil
		}
		return true, persistedCredentials, nil
	}

	// Persisted credentials still belong to this process's pinned chain.
	// Reuse the newest process-local token instead of rotating that chain
	// independently for every concurrent request.
	if tc.currentAccessToken != "" &&
		staleAccess != tc.currentAccessToken &&
		(tc.accessTokenWasSuperseded(staleAccess) || staleAccess == persistedAccess) {
		*auth = persisted
		auth.WToken = tc.currentAccessToken
		if tc.currentRefreshToken != "" {
			auth.RefreshToken = tc.currentRefreshToken
		}
		return true, persistedCredentials, nil
	}
	return false, persistedCredentials, nil
}

func (tc *ToolCtx) persistRefreshedAccess(
	ctx context.Context,
	expected configstore.Credentials,
	accessToken string,
) (bool, error) {
	if tc.config == nil {
		return false, nil
	}
	swapped := false
	err := configstore.ApplyUpdate(
		ctx,
		tc.config,
		func(cfg *domain.Config) (bool, error) {
			if !configstore.CompareAndSwapAccess(cfg, expected, accessToken) {
				return false, nil
			}
			swapped = true
			return true, nil
		},
	)
	return swapped, err
}

// clearRefreshedTokenState drops every process-local reference to a refresh
// chain after an external logout or login takes ownership of persisted auth.
// tokenRefreshMu must be held by the caller.
func (tc *ToolCtx) clearRefreshedTokenState() {
	tc.currentAccessToken = ""
	tc.currentRefreshToken = ""
	tc.supersededTokenHashes = nil
	tc.supersededTokenOrder = nil
	tc.profileAuthHash = [32]byte{}
	tc.hasProfileAuthHash = false
}

// applyTokenRefresh installs a successful refresh response in memory while
// retaining only fingerprints of superseded access tokens.
// tokenRefreshMu must be held by the caller.
func (tc *ToolCtx) applyTokenRefresh(
	auth *woltgateway.AuthContext,
	staleAccess string,
	result woltgateway.TokenRefreshResult,
) error {
	access := strings.TrimSpace(result.AccessToken)
	if access == "" {
		return fmt.Errorf("%w: refresh response missing access token", woltgateway.ErrInvalidResponse)
	}
	if !tc.hasProfileAuthHash {
		// Pin the complete persisted bootstrap snapshot, including refresh-only
		// profiles and same-access-token re-logins with different cookies.
		tc.profileAuthHash = authContextHash(*auth)
		tc.hasProfileAuthHash = true
	}
	tc.rememberSupersededAccessToken(staleAccess)
	if tc.currentAccessToken != "" && tc.currentAccessToken != access {
		tc.rememberSupersededAccessToken(tc.currentAccessToken)
	}
	tc.currentAccessToken = access
	auth.WToken = access
	if r := strings.TrimSpace(result.RefreshToken); r != "" {
		// The rotated refresh token belongs only to this process. Explicit
		// login/logout remain the sole writers of shared credential state.
		auth.RefreshToken = r
	}
	tc.currentRefreshToken = strings.TrimSpace(auth.RefreshToken)
	return nil
}

// accessTokenWasSuperseded checks only fingerprints of old credentials. Raw
// bearer tokens are not retained beyond the current token needed for requests.
// tokenRefreshMu must be held by the caller.
func (tc *ToolCtx) accessTokenWasSuperseded(accessToken string) bool {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return false
	}
	hash := accessTokenHash(accessToken)
	_, ok := tc.supersededTokenHashes[hash]
	return ok
}

// rememberSupersededAccessToken retains a small FIFO of fingerprints for
// in-flight requests. The complete profile fingerprint separately recognizes
// the persisted bootstrap credentials.
// tokenRefreshMu must be held by the caller.
func (tc *ToolCtx) rememberSupersededAccessToken(accessToken string) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return
	}
	hash := accessTokenHash(accessToken)
	if tc.supersededTokenHashes == nil {
		tc.supersededTokenHashes = make(map[[32]byte]struct{}, maxSupersededTokenHashes)
	}
	if _, exists := tc.supersededTokenHashes[hash]; exists {
		return
	}
	if len(tc.supersededTokenOrder) == maxSupersededTokenHashes {
		delete(tc.supersededTokenHashes, tc.supersededTokenOrder[0])
		copy(tc.supersededTokenOrder, tc.supersededTokenOrder[1:])
		tc.supersededTokenOrder = tc.supersededTokenOrder[:maxSupersededTokenHashes-1]
	}
	tc.supersededTokenHashes[hash] = struct{}{}
	tc.supersededTokenOrder = append(tc.supersededTokenOrder, hash)
}

func accessTokenHash(accessToken string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(accessToken)))
}

func authContextHash(auth woltgateway.AuthContext) [32]byte {
	parts := []string{
		strings.TrimSpace(auth.WToken),
		strings.TrimSpace(auth.RefreshToken),
	}
	for _, cookie := range auth.Cookies {
		if cookie = strings.TrimSpace(cookie); cookie != "" {
			parts = append(parts, cookie)
		}
	}
	return sha256.Sum256([]byte(strings.Join(parts, "\x00")))
}

func credentialHash(credentials configstore.Credentials) [32]byte {
	return authContextHash(woltgateway.AuthContext{
		WToken:       credentials.AccessToken,
		RefreshToken: credentials.RefreshToken,
		Cookies:      append([]string(nil), credentials.Cookies...),
	})
}

// tokenExpired returns true when the JWT's exp claim is in the past (with
// leeway). It quietly returns false on parse errors so the caller falls
// through to the reactive 401 path.
func tokenExpired(token string, now time.Time, leeway time.Duration) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some tokens use padded encoding.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return false
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return false
	}
	expiry := time.Unix(claims.Exp, 0).UTC()
	return now.After(expiry.Add(-leeway))
}
