package mcpserver

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type refreshingStubWolt struct {
	*stubWolt
	refreshFn func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error)
}

func (s *refreshingStubWolt) RefreshAccessToken(
	ctx context.Context,
	refreshToken string,
	auth woltgateway.AuthContext,
) (woltgateway.TokenRefreshResult, error) {
	return s.refreshFn(ctx, refreshToken, auth)
}

func TestInvokeWithRefreshConcurrent401RotatesTokenOnce(t *testing.T) {
	const callers = 24

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var refreshCalls atomic.Int32
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			refreshCalls.Add(1)
			if refreshToken != "bootstrap-refresh" {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh token %q", refreshToken)
			}
			if auth.WToken != "stale-access" {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected stale access token %q", auth.WToken)
			}
			return woltgateway.TokenRefreshResult{
				AccessToken:  "fresh-access",
				RefreshToken: "rotated-in-memory",
			}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:          "default",
			WToken:        "stale-access",
			WRefreshToken: "bootstrap-refresh",
		}},
	})

	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			auth := woltgateway.AuthContext{
				WToken:       "stale-access",
				RefreshToken: "bootstrap-refresh",
			}
			got, err := invokeWithRefresh(ctx, tc, &auth, func(current woltgateway.AuthContext) (string, error) {
				if current.WToken == "stale-access" {
					return "", &woltgateway.UpstreamRequestError{StatusCode: 401, Body: "expired"}
				}
				if current.WToken != "fresh-access" {
					return "", fmt.Errorf("unexpected access token %q", current.WToken)
				}
				return "ok", nil
			})
			if err != nil {
				errs <- err
				return
			}
			if got != "ok" {
				errs <- fmt.Errorf("result = %q, want ok", got)
			}
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(errs)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("concurrent token refresh did not complete before timeout: %v", ctx.Err())
	}

	for err := range errs {
		t.Error(err)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("RefreshAccessToken calls = %d, want 1", got)
	}
}

func TestInvokeWithRefreshRotatesRejectedProcessLocalToken(t *testing.T) {
	const (
		staleAccess     = "stale-access"
		rejectedAccess  = "rejected-access"
		recoveredAccess = "recovered-access"
		refreshToken    = "bootstrap-refresh"
	)

	refreshInputs := []string{}
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, gotRefreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			if gotRefreshToken != refreshToken {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh token %q", gotRefreshToken)
			}
			refreshInputs = append(refreshInputs, auth.WToken)
			switch auth.WToken {
			case staleAccess:
				return woltgateway.TokenRefreshResult{AccessToken: rejectedAccess}, nil
			case rejectedAccess:
				return woltgateway.TokenRefreshResult{AccessToken: recoveredAccess}, nil
			default:
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected access token %q", auth.WToken)
			}
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:          "default",
			WToken:        staleAccess,
			WRefreshToken: refreshToken,
		}},
	})
	auth := woltgateway.AuthContext{WToken: staleAccess, RefreshToken: refreshToken}

	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if auth.WToken != rejectedAccess {
		t.Fatalf("initial access token = %q, want %q", auth.WToken, rejectedAccess)
	}

	got, err := invokeWithRefresh(context.Background(), tc, &auth, func(current woltgateway.AuthContext) (string, error) {
		switch current.WToken {
		case rejectedAccess:
			return "", &woltgateway.UpstreamRequestError{StatusCode: 401, Body: "expired"}
		case recoveredAccess:
			return "ok", nil
		default:
			return "", fmt.Errorf("unexpected request access token %q", current.WToken)
		}
	})
	if err != nil {
		t.Fatalf("invoke after process-local rotation: %v", err)
	}
	if got != "ok" || auth.WToken != recoveredAccess {
		t.Fatalf("result = %q, access token = %q", got, auth.WToken)
	}
	if want := []string{staleAccess, rejectedAccess}; !slices.Equal(refreshInputs, want) {
		t.Fatalf("refresh inputs = %v, want %v", refreshInputs, want)
	}
}

func TestInvokeWithRefreshReusesProcessLocalAccessToken(t *testing.T) {
	const (
		staleAccess = "expired-access-placeholder"
		freshAccess = "refreshed-access-placeholder"
		refresh     = "refresh-placeholder"
	)

	var refreshCalls int
	var staleRequests int
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			refreshCalls++
			if refreshToken != refresh || auth.WToken != staleAccess {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh input")
			}
			return woltgateway.TokenRefreshResult{AccessToken: freshAccess}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:          "default",
			WToken:        staleAccess,
			WRefreshToken: refresh,
		}},
	})

	invoke := func() woltgateway.AuthContext {
		t.Helper()
		_, auth, err := tc.requireAuth(context.Background())
		if err != nil {
			t.Fatalf("requireAuth: %v", err)
		}
		got, err := invokeWithRefresh(context.Background(), tc, &auth, func(current woltgateway.AuthContext) (string, error) {
			if current.WToken == staleAccess {
				staleRequests++
				return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			if current.WToken != freshAccess {
				return "", fmt.Errorf("unexpected request access token")
			}
			return "ok", nil
		})
		if err != nil || got != "ok" {
			t.Fatalf("invokeWithRefresh = %q, %v", got, err)
		}
		return auth
	}

	firstAuth := invoke()
	if firstAuth.WToken != freshAccess {
		t.Fatalf("first request did not retain refreshed access token")
	}
	secondAuth := invoke()
	if secondAuth.WToken != freshAccess {
		t.Fatalf("second request did not reuse process-local access token")
	}
	if refreshCalls != 1 || staleRequests != 1 {
		t.Fatalf(
			"refresh calls = %d, stale requests = %d; want 1, 1",
			refreshCalls,
			staleRequests,
		)
	}
}

func TestRequireAuthKeepsNewestTokenAcrossRepeatedProcessLocalRotations(t *testing.T) {
	const refreshToken = "bootstrap-refresh"
	accessTokens := []string{"access-a", "access-b", "access-c", "access-d"}
	refreshIndex := 0
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, gotRefresh string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			if gotRefresh != refreshToken || refreshIndex >= len(accessTokens)-1 {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh request")
			}
			if auth.WToken != accessTokens[refreshIndex] {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf(
					"refresh access token = %q, want %q",
					auth.WToken,
					accessTokens[refreshIndex],
				)
			}
			refreshIndex++
			return woltgateway.TokenRefreshResult{AccessToken: accessTokens[refreshIndex]}, nil
		},
	}
	profiles := &stubProfiles{profile: domain.Profile{
		Name:          "default",
		WToken:        accessTokens[0],
		WRefreshToken: refreshToken,
	}}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: profiles})
	auth := buildAuthContext(profiles.profile)

	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("refresh A -> B: %v", err)
	}
	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("refresh B -> C: %v", err)
	}
	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("refresh C -> D: %v", err)
	}

	_, loaded, err := tc.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("requireAuth: %v", err)
	}
	if loaded.WToken != accessTokens[3] {
		t.Fatalf("requireAuth access token = %q, want newest %q", loaded.WToken, accessTokens[3])
	}
	if refreshIndex != 3 {
		t.Fatalf("refreshes = %d, want 3", refreshIndex)
	}
}

func TestSupersededAccessTokenFingerprintsAreBounded(t *testing.T) {
	tc := newToolCtx(Deps{})
	total := maxSupersededTokenHashes + 4
	tc.tokenRefreshMu.Lock()
	for index := 0; index < total; index++ {
		tc.rememberSupersededAccessToken(fmt.Sprintf("access-token-%d", index))
	}
	if len(tc.supersededTokenHashes) != maxSupersededTokenHashes ||
		len(tc.supersededTokenOrder) != maxSupersededTokenHashes {
		t.Fatalf(
			"superseded token cache sizes = %d/%d, want %d",
			len(tc.supersededTokenHashes),
			len(tc.supersededTokenOrder),
			maxSupersededTokenHashes,
		)
	}
	if tc.accessTokenWasSuperseded("access-token-0") {
		t.Fatal("oldest unpinned token fingerprint was not evicted")
	}
	if !tc.accessTokenWasSuperseded(fmt.Sprintf("access-token-%d", total-1)) {
		t.Fatal("newest token fingerprint was not retained")
	}
	tc.tokenRefreshMu.Unlock()
}

func TestToolErrClassifiesUpstreamStatusWithoutLeakingBody(t *testing.T) {
	secretBody := strings.Repeat("sensitive-upstream-body ", 100)
	tests := []struct {
		name       string
		status     int
		retryAfter time.Duration
		want       string
		notWant    string
	}{
		{name: "network", status: 0, want: "could not reach Wolt"},
		{name: "unauthorized", status: 401, want: "wolt login"},
		{name: "forbidden", status: 403, want: "not allowed", notWant: "wolt login"},
		{name: "not found", status: 404, want: "not found"},
		{name: "unsupported", status: 410, want: "no longer supported"},
		{name: "rate limit", status: 429, retryAfter: 1500 * time.Millisecond, want: "retry after 2s"},
		{name: "temporary", status: 503, want: "temporarily unavailable"},
		{name: "other client error", status: 422, want: "rejected the request"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := toolErr(&woltgateway.UpstreamRequestError{
				Method:     "GET",
				URL:        "https://example.test/private?token=do-not-leak",
				StatusCode: tc.status,
				Body:       secretBody,
				RetryAfter: tc.retryAfter,
			})
			got := err.Error()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("toolErr = %q, want substring %q", got, tc.want)
			}
			for _, forbidden := range []string{"sensitive-upstream-body", "do-not-leak", tc.notWant} {
				if forbidden != "" && strings.Contains(got, forbidden) {
					t.Fatalf("toolErr leaked/contained forbidden text %q: %q", forbidden, got)
				}
			}
		})
	}
}

func TestToolErrKeepsRefreshFailureDistinctFromExpiredSession(t *testing.T) {
	err := toolErr(&tokenRefreshError{cause: &woltgateway.UpstreamRequestError{
		StatusCode: 503,
		Body:       "do not leak",
	}})
	got := err.Error()
	if !strings.Contains(got, "session refresh failed") || !strings.Contains(got, "temporarily unavailable") {
		t.Fatalf("toolErr = %q", got)
	}
	if strings.Contains(got, "session expired") || strings.Contains(got, "do not leak") {
		t.Fatalf("refresh failure was misclassified or leaked body: %q", got)
	}
}

func TestHandleAccountStatusUsesNestedUserName(t *testing.T) {
	tc := newToolCtx(Deps{
		Wolt: &stubWolt{
			userMeFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"user": map[string]any{
						"name": map[string]any{
							"first_name": "Nested",
							"last_name":  "Account",
						},
						"email": "nested@example.test",
					},
				}, nil
			},
		},
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "access-token",
		}},
	})

	_, out, err := tc.handleAccountStatus(context.Background(), nil, AccountStatusInput{})
	if err != nil {
		t.Fatalf("handleAccountStatus: %v", err)
	}
	if out.Summary != "authenticated as Nested Account" {
		t.Fatalf("summary = %q, want nested account name", out.Summary)
	}
}

func TestAccountStatusSummaryHasCleanFallback(t *testing.T) {
	if got := accountStatusSummary(map[string]any{"user": map[string]any{}}); got != "authenticated" {
		t.Fatalf("summary = %q, want authenticated", got)
	}
	got := accountStatusSummary(map[string]any{
		"user": map[string]any{
			"user": map[string]any{
				"name": map[string]any{"first_name": "Deep", "last_name": "User"},
			},
		},
	})
	if got != "authenticated as Deep User" {
		t.Fatalf("nested summary = %q", got)
	}
}
