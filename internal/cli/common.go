package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return ""
}

type globalFlags struct {
	Format        string
	Profile       string
	Address       string
	Locale        string
	NoColor       bool
	Output        string
	WToken        string
	WRefreshToken string
	Cookies       []string
	Verbose       bool
}

const sharedGlobalFlagAnnotation = "wolt_cli_shared_global"

func addGlobalFlags(cmd *cobra.Command, flags *globalFlags) {
	_ = addSharedGlobalFlag(cmd, "format", func() {
		cmd.Flags().StringVar(&flags.Format, "format", "table", "Output format: table, json, or yaml.")
	})
	if addSharedGlobalFlag(cmd, "profile", func() {
		cmd.Flags().StringVar(&flags.Profile, "profile", "", "Legacy profile selector; only default is supported.")
	}) {
		markHidden(cmd, "profile")
	}
	_ = addSharedGlobalFlag(cmd, "address", func() {
		cmd.Flags().StringVar(&flags.Address, "address", "", "Temporary address override for this command. Geocoded to coordinates. Cannot be combined with --lat/--lon.")
	})
	_ = addSharedGlobalFlag(cmd, "locale", func() {
		cmd.Flags().StringVar(&flags.Locale, "locale", "en-FI", "Response locale in BCP-47 format, for example en-FI.")
	})
	_ = addSharedGlobalFlag(cmd, "no-color", func() {
		cmd.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable ANSI color codes in table output.")
	})
	if addSharedGlobalFlag(cmd, "wtoken", func() {
		cmd.Flags().StringVar(&flags.WToken, "wtoken", "", "Wolt token for authenticated endpoints.")
	}) {
		markHidden(cmd, "wtoken")
	}
	if addSharedGlobalFlag(cmd, "wrtoken", func() {
		cmd.Flags().StringVar(&flags.WRefreshToken, "wrtoken", "", "Wolt refresh token for automatic access token rotation.")
	}) {
		markHidden(cmd, "wrtoken")
	}
	if addSharedGlobalFlag(cmd, "cookie", func() {
		cmd.Flags().StringArrayVar(&flags.Cookies, "cookie", nil, "HTTP cookie header value to forward (repeatable).")
	}) {
		markHidden(cmd, "cookie")
	}
	_ = addSharedGlobalFlag(cmd, "verbose", func() {
		cmd.Flags().BoolVar(&flags.Verbose, "verbose", false, "Enable verbose output (prints upstream request trace and detailed error diagnostics).")
	})
}

func addSharedGlobalFlag(cmd *cobra.Command, name string, register func()) bool {
	if cmd.Flags().Lookup(name) != nil {
		return false
	}
	register()
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[sharedGlobalFlagAnnotation] = []string{"true"}
	return true
}

func markHidden(cmd *cobra.Command, name string) {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		flag.Hidden = true
	}
}

func resolveProfileLabel(profileName string) string {
	profile := strings.TrimSpace(profileName)
	if profile == "" {
		return "anonymous"
	}
	return profile
}

func resolveProfileLocation(
	ctx context.Context,
	deps Dependencies,
	address string,
	profileName string,
	format output.Format,
	locale string,
	outputPath string,
	auth *woltgateway.AuthContext,
	cmd *cobra.Command,
) (domain.Location, string, error) {
	return resolveLocation(ctx, deps, nil, nil, address, profileName, format, locale, outputPath, auth, cmd)
}

func parseOutputFormat(format string) (output.Format, error) {
	return output.ParseFormat(format)
}

func writeTable(cmd *cobra.Command, text string, outputPath string) error {
	if err := output.WriteOutput(cmd.OutOrStdout(), text, outputPath); err != nil {
		return err
	}
	return nil
}

func writeMachinePayload(cmd *cobra.Command, env output.Envelope, format output.Format, outputPath string) error {
	rendered, err := output.RenderPayload(env, format)
	if err != nil {
		return err
	}
	if err := output.WriteOutput(cmd.OutOrStdout(), rendered, outputPath); err != nil {
		return err
	}
	return nil
}

func emitError(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	code string,
	message string,
) error {
	return emitErrorWithWarnings(cmd, format, profile, locale, outputPath, code, message, nil)
}

func emitErrorWithWarnings(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	code string,
	message string,
	warnings []string,
) error {
	if format == output.FormatTable {
		rendered := message
		for _, warning := range warnings {
			if strings.TrimSpace(warning) == "" {
				continue
			}
			rendered += "\nWarning: " + warning
		}
		if err := output.WriteOutput(cmd.OutOrStdout(), rendered, outputPath); err != nil {
			return err
		}
		return &exitError{code: 1}
	}
	env := output.BuildEnvelope(profile, locale, nil, append([]string{}, warnings...), map[string]any{
		"code":    code,
		"message": message,
	})
	if err := writeMachinePayload(cmd, env, format, outputPath); err != nil {
		return err
	}
	return &exitError{code: 1}
}

func resolveLocation(
	ctx context.Context,
	deps Dependencies,
	lat *float64,
	lon *float64,
	address string,
	profileName string,
	format output.Format,
	locale string,
	outputPath string,
	auth *woltgateway.AuthContext,
	cmd *cobra.Command,
) (domain.Location, string, error) {
	resolvedAddress := strings.TrimSpace(address)
	if resolvedAddress != "" {
		if lat != nil || lon != nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_INVALID_ARGUMENT",
				"Do not combine --address with --lat/--lon. Use either --address or both --lat and --lon.",
			)
		}
		if deps.Location == nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_LOCATION_RESOLVE_ERROR",
				"Location resolver is not available.",
			)
		}
		location, err := deps.Location.Get(ctx, resolvedAddress)
		if err != nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_LOCATION_RESOLVE_ERROR",
				err.Error(),
			)
		}
		return location, resolveProfileLabel(profileName), nil
	}

	if lat == nil && lon == nil {
		profile, err := deps.Profiles.Find(ctx, profileName)
		if err != nil {
			return domain.Location{}, "", profileError(err, format, profileName, locale, outputPath, cmd)
		}
		location, locationErr := resolveAccountLocation(ctx, deps, profile, auth)
		if locationErr == nil {
			return location, profile.Name, nil
		}
		if profile.Location.Lat != 0 || profile.Location.Lon != 0 || profile.IsDefault {
			return profile.Location, profile.Name, nil
		}
		return domain.Location{}, "", emitError(
			cmd,
			format,
			profile.Name,
			locale,
			outputPath,
			"WOLT_LOCATION_RESOLVE_ERROR",
			"unable to resolve location from Wolt account; use --address or sign in and set an address in Wolt",
		)
	}

	if lat == nil || lon == nil {
		return domain.Location{}, "", emitError(
			cmd,
			format,
			resolveProfileLabel(profileName),
			locale,
			outputPath,
			"WOLT_INVALID_ARGUMENT",
			"Both --lat and --lon must be provided together, or omit both to use Wolt account address.",
		)
	}

	return domain.Location{Lat: *lat, Lon: *lon}, resolveProfileLabel(profileName), nil
}

func profileError(err error, format output.Format, profileName string, locale string, outputPath string, cmd *cobra.Command) error {
	message := err.Error()
	if strings.TrimSpace(profileName) == "" {
		profileName = "default"
	}
	return emitError(cmd, format, profileName, locale, outputPath, "WOLT_PROFILE_ERROR", message)
}

func emitUpstreamError(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	verbose bool,
	err error,
	warnings ...string,
) error {
	if err == nil {
		err = woltgateway.ErrUpstream
	}

	classifiedErr := err
	refresh := false
	var refreshFailure *cliTokenRefreshError
	if errors.As(err, &refreshFailure) {
		classifiedErr = refreshFailure.refreshErr
		refresh = true
	}

	code := "WOLT_UPSTREAM_ERROR"
	message := woltgateway.ErrUpstream.Error() + " (use --verbose for details)"
	if classifiedCode, classifiedMessage, ok := classifyCLIUpstreamError(classifiedErr, refresh); ok {
		code = classifiedCode
		message = classifiedMessage
	}
	if verbose {
		message = classifiedErr.Error()
	}
	return emitErrorWithWarnings(
		cmd,
		format,
		profile,
		locale,
		outputPath,
		code,
		message,
		warnings,
	)
}

func classifyCLIUpstreamError(err error, refresh bool) (string, string, bool) {
	var upstreamErr *woltgateway.UpstreamRequestError
	if errors.As(err, &upstreamErr) {
		switch upstreamErr.StatusCode {
		case 0:
			return "WOLT_UPSTREAM_TEMPORARY", "Could not reach Wolt. Check the network connection and retry.", true
		case 401:
			return "WOLT_AUTH_REQUIRED", "Your Wolt session expired or is missing. Run \"wolt login\" to refresh.", true
		case 403:
			return "WOLT_FORBIDDEN", "Wolt rejected this request; the current account or operation is not allowed.", true
		case 404:
			if refresh {
				return "WOLT_UNSUPPORTED_ENDPOINT", "Wolt's session refresh endpoint is unavailable; update wolt-cli and retry.", true
			}
			return "WOLT_NOT_FOUND", "The requested Wolt resource was not found.", true
		case 410:
			if woltgateway.LooksLikeOutdatedClient(upstreamErr.Body) {
				return "WOLT_CLIENT_OUTDATED", "Wolt rejected the configured client version; update wolt-cli and retry.", true
			}
			return "WOLT_UNSUPPORTED_ENDPOINT", "This Wolt API operation is unavailable or unsupported; update wolt-cli and retry.", true
		case 405, 501:
			return "WOLT_UNSUPPORTED_ENDPOINT", "This Wolt API operation is unavailable or unsupported; update wolt-cli and retry.", true
		case 429:
			return "WOLT_RATE_LIMITED", "Wolt is rate-limiting requests; retry later.", true
		case 408, 425, 500, 502, 503, 504:
			return "WOLT_UPSTREAM_TEMPORARY", "Wolt is temporarily unavailable; retry later.", true
		}
		if upstreamErr.StatusCode >= 200 &&
			upstreamErr.StatusCode < 300 &&
			errors.Is(err, woltgateway.ErrInvalidResponse) {
			return "WOLT_UPSTREAM_INVALID_RESPONSE", "Wolt returned an invalid response; retry later.", true
		}
		if upstreamErr.StatusCode > 0 {
			return "WOLT_UPSTREAM_ERROR",
				fmt.Sprintf("%s (status %d, use --verbose for details)", woltgateway.ErrUpstream.Error(), upstreamErr.StatusCode),
				true
		}
	}
	if errors.Is(err, woltgateway.ErrInvalidResponse) {
		return "WOLT_UPSTREAM_INVALID_RESPONSE", "Wolt returned an invalid response; retry later.", true
	}
	return "", "", false
}

func splitCSV(value string) map[string]struct{} {
	result := map[string]struct{}{}
	if strings.TrimSpace(value) == "" {
		return result
	}
	for _, part := range strings.Split(value, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}
		result[token] = struct{}{}
	}
	return result
}

// browserOpenCommand is the package-level hook that resolves the platform-specific
// argv used to open a URL. It exists so tests can swap in a recording stub.
var browserOpenCommand = defaultBrowserOpenCommand

// openBrowser launches the user's default browser at the given URL.
// Returns an error if the URL is empty, the OS is unsupported, or the
// underlying launcher exits non-zero.
func openBrowser(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("openBrowser: empty URL")
	}
	name, args, err := browserOpenCommand(target)
	if err != nil {
		return err
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

func defaultBrowserOpenCommand(target string) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{target}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	default:
		return "xdg-open", []string{target}, nil
	}
}

func normalizeCookieInputs(raw []string) []string {
	cookies := make([]string, 0, len(raw))
	for _, cookie := range raw {
		trimmed := strings.TrimSpace(cookie)
		if trimmed == "" {
			continue
		}
		cookies = append(cookies, trimmed)
	}
	return cookies
}

func buildAuthContext(flags globalFlags) woltgateway.AuthContext {
	auth := woltgateway.AuthContext{
		WToken: normalizeWToken(flags.WToken),
	}
	auth.RefreshToken = extractRefreshToken(flags.WRefreshToken)
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = normalizeRefreshToken(flags.WRefreshToken)
	}
	auth.Cookies = normalizeCookieInputs(flags.Cookies)
	if auth.WToken == "" {
		auth.WToken = extractWTokenFromCookieInputs(auth.Cookies)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshToken(flags.WToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshTokenFromCookieInputs(auth.Cookies)
	}
	return auth
}

func loadAuthContextWithProfile(
	ctx context.Context,
	deps Dependencies,
	flags globalFlags,
) (woltgateway.AuthContext, error) {
	auth := buildAuthContext(flags)
	if auth.CanAuthenticate() || deps.Profiles == nil {
		return auth, nil
	}
	profile, err := deps.Profiles.Find(ctx, flags.Profile)
	if err != nil {
		return woltgateway.AuthContext{}, err
	}
	if len(auth.Cookies) == 0 {
		auth.Cookies = normalizeCookieInputs(profile.Cookies)
	}
	if strings.TrimSpace(auth.WToken) == "" {
		auth.WToken = normalizeWToken(profile.WToken)
	}
	if strings.TrimSpace(auth.WToken) == "" {
		auth.WToken = extractWTokenFromCookieInputs(auth.Cookies)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = normalizeRefreshToken(profile.WRefreshToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshToken(profile.WToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshTokenFromCookieInputs(auth.Cookies)
	}
	return auth, nil
}

func buildAuthContextWithProfile(ctx context.Context, deps Dependencies, flags globalFlags) woltgateway.AuthContext {
	auth, _ := loadAuthContextWithProfile(ctx, deps, flags)
	return auth
}

func loadRequiredAuth(
	ctx context.Context,
	deps Dependencies,
	flags globalFlags,
	format output.Format,
	cmd *cobra.Command,
) (woltgateway.AuthContext, error) {
	profileName := defaultProfileName(flags.Profile)
	auth, err := loadAuthContextWithProfile(ctx, deps, flags)
	if err != nil {
		return woltgateway.AuthContext{}, profileError(
			err,
			format,
			profileName,
			flags.Locale,
			flags.Output,
			cmd,
		)
	}
	if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
		return woltgateway.AuthContext{}, err
	}
	return auth, nil
}

func requireAuth(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	auth woltgateway.AuthContext,
) error {
	if auth.CanAuthenticate() {
		return nil
	}
	return emitError(
		cmd,
		format,
		profile,
		locale,
		outputPath,
		"WOLT_AUTH_REQUIRED",
		"Authentication is required. Run \"wolt login\" first.",
	)
}

func defaultProfileName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "default"
	}
	return trimmed
}

type cliTokenRefreshError struct {
	requestErr error
	refreshErr error
}

func (e *cliTokenRefreshError) Error() string {
	return fmt.Sprintf("%v: automatic token refresh failed: %v", e.requestErr, e.refreshErr)
}

func (e *cliTokenRefreshError) Unwrap() []error {
	return []error{e.requestErr, e.refreshErr}
}

func refreshAuthContext(
	ctx context.Context,
	deps Dependencies,
	auth *woltgateway.AuthContext,
	persistence *credentialPersistence,
) (bool, []string, error) {
	warnings := []string{}
	if auth == nil {
		return false, warnings, fmt.Errorf("auth context is nil")
	}
	refreshToken := strings.TrimSpace(auth.RefreshToken)
	if refreshToken == "" {
		return false, warnings, nil
	}
	result, err := deps.Wolt.RefreshAccessToken(ctx, refreshToken, *auth)
	if err != nil {
		return false, warnings, err
	}
	accessToken := normalizeWToken(result.AccessToken)
	if accessToken == "" {
		return false, warnings, fmt.Errorf("%w: refresh response did not include access token", woltgateway.ErrInvalidResponse)
	}
	auth.WToken = accessToken
	if candidate := normalizeRefreshToken(result.RefreshToken); candidate != "" {
		// In-memory only — keep walking the chain within this process, but the
		// browser-style bootstrap token in the config stays pinned.
		auth.RefreshToken = candidate
	}
	warnings = append(warnings, "access token refreshed automatically")
	attempted, persisted, persistErr := persistence.persistAccess(ctx, auth.WToken)
	warnings = append(
		warnings,
		credentialPersistenceWarnings(
			"refreshed access token",
			attempted,
			persisted,
			persistErr,
			false,
		)...,
	)
	return true, warnings, nil
}

func invokeWithAuthAutoRefresh[T any](
	ctx context.Context,
	deps Dependencies,
	flags globalFlags,
	auth *woltgateway.AuthContext,
	invoke func(woltgateway.AuthContext) (T, error),
) (T, []string, error) {
	if auth == nil {
		var zero T
		return zero, nil, fmt.Errorf("auth context is nil")
	}
	persistence := newCredentialPersistence(
		ctx,
		deps,
		*auth,
		allowAutomaticCredentialPersistence(flags),
	)
	return invokeWithAuthAutoRefreshUsingPersistence(
		ctx,
		deps,
		flags,
		auth,
		invoke,
		persistence,
	)
}

func invokeWithAuthAutoRefreshUsingPersistence[T any](
	ctx context.Context,
	deps Dependencies,
	flags globalFlags,
	auth *woltgateway.AuthContext,
	invoke func(woltgateway.AuthContext) (T, error),
	persistence *credentialPersistence,
) (T, []string, error) {
	var zero T
	warnings := []string{}
	if auth == nil {
		return zero, warnings, fmt.Errorf("auth context is nil")
	}
	var proactiveRefreshErr error
	if tokenExpired(auth.WToken, time.Now().UTC(), 30*time.Second) {
		// Opportunistic re-sync from a running Chrome. Mirrors browser
		// behaviour: if the user has wolt.com open, their cookies are
		// almost certainly fresher than ours, and adopting them keeps the
		// CLI on the same refresh chain as the browser.
		if chromeAuth, found, _ := pullOpportunisticChromeAuth(ctx, ""); found && chromeAuthIsFresherThan(chromeAuth, auth.WToken) {
			if err := adoptChromeAuth(auth, chromeAuth); err == nil {
				persistence.disable()
				warnings = append(warnings, "adopted fresher Wolt session from running Chrome")
			}
		}
	}
	if tokenExpired(auth.WToken, time.Now().UTC(), 30*time.Second) {
		_, refreshWarnings, refreshErr := refreshAuthContext(ctx, deps, auth, persistence)
		warnings = append(warnings, refreshWarnings...)
		if refreshErr != nil {
			proactiveRefreshErr = refreshErr
			warning := "automatic token refresh failed before request"
			if flags.Verbose {
				warning += ": " + refreshErr.Error()
			}
			warnings = append(warnings, warning)
		}
	}

	result, err := invoke(*auth)
	if err == nil {
		return result, warnings, nil
	}
	if !woltgateway.HasStatus(err, http.StatusUnauthorized) {
		return result, warnings, err
	}
	if proactiveRefreshErr != nil {
		if retryResult, retryErr, attempted := retryWithChromeAuth(
			ctx,
			auth,
			invoke,
			persistence,
		); attempted && retryErr == nil {
			warnings = append(warnings, "recovered Wolt session from running Chrome after refresh failure")
			return retryResult, warnings, nil
		}
		return result, warnings, &cliTokenRefreshError{
			requestErr: err,
			refreshErr: proactiveRefreshErr,
		}
	}

	refreshed, refreshWarnings, refreshErr := refreshAuthContext(ctx, deps, auth, persistence)
	warnings = append(warnings, refreshWarnings...)
	if refreshErr != nil {
		// Refresh chain is dead — last-ditch attempt to recover from Chrome
		// before surrendering. If the user has a running browser session,
		// adopting it gives us a working chain again without a re-login.
		if retryResult, retryErr, attempted := retryWithChromeAuth(
			ctx,
			auth,
			invoke,
			persistence,
		); attempted && retryErr == nil {
			warnings = append(warnings, "recovered Wolt session from running Chrome after refresh failure")
			return retryResult, warnings, nil
		}
		return result, warnings, &cliTokenRefreshError{
			requestErr: err,
			refreshErr: refreshErr,
		}
	}
	if !refreshed {
		return result, warnings, err
	}

	retryResult, retryErr := invoke(*auth)
	return retryResult, warnings, retryErr
}

func retryWithChromeAuth[T any](
	ctx context.Context,
	auth *woltgateway.AuthContext,
	invoke func(woltgateway.AuthContext) (T, error),
	persistence *credentialPersistence,
) (T, error, bool) {
	var zero T
	chromeAuth, found, _ := pullOpportunisticChromeAuth(ctx, "")
	if !found {
		return zero, nil, false
	}
	if err := adoptChromeAuth(auth, chromeAuth); err != nil {
		return zero, err, true
	}
	persistence.disable()
	result, err := invoke(*auth)
	return result, err, true
}

func credentialPersistenceWarnings(
	label string,
	attempted bool,
	persisted bool,
	err error,
	verbose bool,
) []string {
	if !attempted || persisted {
		return nil
	}
	if err == nil {
		return []string{label + " was not persisted because saved credentials changed concurrently"}
	}
	warning := "failed to persist " + label
	if verbose {
		warning += ": " + err.Error()
	}
	return []string{warning}
}
