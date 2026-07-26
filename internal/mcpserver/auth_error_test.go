package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// mutableProfileResolver lets auth tests replace persisted account state
// while concurrent MCP requests are in flight.
type mutableProfileResolver struct {
	mu      sync.RWMutex
	profile domain.Profile
	err     error
}

func (r *mutableProfileResolver) Find(context.Context, string) (domain.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.profile, r.err
}

func (r *mutableProfileResolver) set(profile domain.Profile, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profile = profile
	r.err = err
}

func classifiedInfo(t *testing.T, err error) ToolErrorInfo {
	t.Helper()
	var classified *classifiedToolError
	if !errors.As(toolErr(err), &classified) {
		t.Fatalf("error %T was not classified", err)
	}
	return classified.info
}

func TestMCPRefreshOnlyProfileCoordinatesConcurrentRequests(t *testing.T) {
	const callers = 16

	profiles := &mutableProfileResolver{profile: domain.Profile{
		Name:          "default",
		WRefreshToken: "bootstrap-refresh",
	}}
	var refreshCalls atomic.Int32
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			refreshCalls.Add(1)
			if refreshToken != "bootstrap-refresh" || auth.WToken != "" {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf(
					"unexpected refresh input token=%q access=%q",
					refreshToken,
					auth.WToken,
				)
			}
			return woltgateway.TokenRefreshResult{AccessToken: "process-access"}, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: profiles})

	ready := sync.WaitGroup{}
	ready.Add(callers)
	start := make(chan struct{})
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			_, auth, err := tc.requireAuth(context.Background())
			if err != nil {
				errs <- err
				ready.Done()
				return
			}
			ready.Done()
			<-start
			result, err := invokeWithRefresh(
				context.Background(),
				tc,
				&auth,
				func(current woltgateway.AuthContext) (string, error) {
					if current.WToken == "" {
						return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
					}
					if current.WToken != "process-access" {
						return "", fmt.Errorf("unexpected access token %q", current.WToken)
					}
					return "ok", nil
				},
			)
			if err != nil {
				errs <- err
			} else if result != "ok" {
				errs <- fmt.Errorf("result = %q, want ok", result)
			}
		}()
	}
	readyDone := make(chan struct{})
	go func() {
		ready.Wait()
		close(readyDone)
	}()
	select {
	case <-readyDone:
	case <-time.After(5 * time.Second):
		close(start)
		t.Fatal("timed out waiting for concurrent callers to become ready")
	}
	close(start)
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent refresh callers to finish")
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("RefreshAccessToken calls = %d, want 1", got)
	}

	_, auth, err := tc.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("requireAuth after refresh: %v", err)
	}
	if auth.WToken != "process-access" || auth.RefreshToken != "bootstrap-refresh" {
		t.Fatalf("process-local auth = %#v", auth)
	}
}

func TestMCPProcessLocalRefreshHonorsExternalLogout(t *testing.T) {
	profiles := &mutableProfileResolver{profile: domain.Profile{
		Name:          "default",
		WToken:        "bootstrap-access",
		WRefreshToken: "bootstrap-refresh",
	}}
	tc := newToolCtx(Deps{
		Profiles: profiles,
		Wolt: &refreshingStubWolt{
			stubWolt: &stubWolt{},
			refreshFn: func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
				return woltgateway.TokenRefreshResult{AccessToken: "process-access"}, nil
			},
		},
	})
	auth := buildAuthContext(profiles.profile)
	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	profiles.set(domain.Profile{}, nil)
	if _, _, err := tc.requireAuth(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("requireAuth after logout = %v, want ErrNotLoggedIn", err)
	}

	tc.tokenRefreshMu.Lock()
	defer tc.tokenRefreshMu.Unlock()
	if tc.currentAccessToken != "" || tc.currentRefreshToken != "" || tc.hasProfileAuthHash ||
		len(tc.supersededTokenHashes) != 0 || len(tc.supersededTokenOrder) != 0 {
		t.Fatal("external logout did not clear process-local refresh state")
	}
}

func TestMCPProcessLocalRefreshKeepsRotatedRefreshChainAcrossToolCalls(t *testing.T) {
	profiles := &mutableProfileResolver{profile: domain.Profile{
		Name:          "default",
		WToken:        "bootstrap-access",
		WRefreshToken: "bootstrap-refresh",
	}}
	refreshInputs := []string{}
	tc := newToolCtx(Deps{
		Profiles: profiles,
		Wolt: &refreshingStubWolt{
			stubWolt: &stubWolt{},
			refreshFn: func(
				_ context.Context,
				refreshToken string,
				auth woltgateway.AuthContext,
			) (woltgateway.TokenRefreshResult, error) {
				refreshInputs = append(refreshInputs, refreshToken)
				switch refreshToken {
				case "bootstrap-refresh":
					if auth.WToken != "bootstrap-access" {
						return woltgateway.TokenRefreshResult{}, fmt.Errorf("first refresh auth = %#v", auth)
					}
					return woltgateway.TokenRefreshResult{
						AccessToken:  "process-access-1",
						RefreshToken: "process-refresh-1",
					}, nil
				case "process-refresh-1":
					if auth.WToken != "process-access-1" {
						return woltgateway.TokenRefreshResult{}, fmt.Errorf("second refresh auth = %#v", auth)
					}
					return woltgateway.TokenRefreshResult{
						AccessToken:  "process-access-2",
						RefreshToken: "process-refresh-2",
					}, nil
				default:
					return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh token %q", refreshToken)
				}
			},
		},
	})

	for request := 0; request < 2; request++ {
		_, auth, err := tc.requireAuth(context.Background())
		if err != nil {
			t.Fatalf("requireAuth %d: %v", request+1, err)
		}
		wantRejectedAccess := "bootstrap-access"
		wantAcceptedAccess := "process-access-1"
		if request == 1 {
			wantRejectedAccess = "process-access-1"
			wantAcceptedAccess = "process-access-2"
			if auth.RefreshToken != "process-refresh-1" {
				t.Fatalf("second tool auth lost rotated refresh token: %#v", auth)
			}
		}
		result, err := invokeWithRefresh(
			context.Background(),
			tc,
			&auth,
			func(current woltgateway.AuthContext) (string, error) {
				if current.WToken == wantRejectedAccess {
					return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
				}
				if current.WToken != wantAcceptedAccess {
					return "", fmt.Errorf("request access token = %q, want %q", current.WToken, wantAcceptedAccess)
				}
				return "ok", nil
			},
		)
		if err != nil || result != "ok" {
			t.Fatalf("tool call %d = %q, %v", request+1, result, err)
		}
	}

	_, auth, err := tc.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("final requireAuth: %v", err)
	}
	if auth.WToken != "process-access-2" || auth.RefreshToken != "process-refresh-2" {
		t.Fatalf("final process-local chain = %#v", auth)
	}
	if strings.Join(refreshInputs, ",") != "bootstrap-refresh,process-refresh-1" {
		t.Fatalf("refresh inputs = %v", refreshInputs)
	}
}

func TestMCPProcessLocalRefreshHonorsSameAccessRebootstrap(t *testing.T) {
	const persistedAccess = "shared-access"
	profiles := &mutableProfileResolver{profile: domain.Profile{
		Name:          "default",
		WToken:        persistedAccess,
		WRefreshToken: "old-bootstrap",
		Cookies:       []string{"session=old"},
	}}
	var refreshCalls atomic.Int32
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			refreshCalls.Add(1)
			switch refreshToken {
			case "old-bootstrap":
				return woltgateway.TokenRefreshResult{AccessToken: "old-process-access"}, nil
			case "new-bootstrap":
				if auth.WToken != persistedAccess ||
					len(auth.Cookies) != 1 || auth.Cookies[0] != "session=new" {
					return woltgateway.TokenRefreshResult{}, fmt.Errorf("new bootstrap auth = %#v", auth)
				}
				return woltgateway.TokenRefreshResult{AccessToken: "new-process-access"}, nil
			default:
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh token %q", refreshToken)
			}
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: profiles})
	auth := buildAuthContext(profiles.profile)
	if err := tc.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	profiles.set(domain.Profile{
		Name:          "default",
		WToken:        persistedAccess,
		WRefreshToken: "new-bootstrap",
		Cookies:       []string{"session=new"},
	}, nil)
	_, auth, err := tc.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("requireAuth after external login: %v", err)
	}
	if auth.WToken != persistedAccess || auth.RefreshToken != "new-bootstrap" {
		t.Fatalf("external bootstrap was not adopted: %#v", auth)
	}

	result, err := invokeWithRefresh(
		context.Background(),
		tc,
		&auth,
		func(current woltgateway.AuthContext) (string, error) {
			if current.WToken == persistedAccess {
				return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			if current.WToken != "new-process-access" {
				return "", fmt.Errorf("unexpected access token %q", current.WToken)
			}
			return "ok", nil
		},
	)
	if err != nil || result != "ok" {
		t.Fatalf("invoke after rebootstrap = %q, %v", result, err)
	}
	if got := refreshCalls.Load(); got != 2 {
		t.Fatalf("RefreshAccessToken calls = %d, want 2", got)
	}
}

func TestMCPProactiveRefreshFailureIsAttemptedOnceAndCarriesMetadata(t *testing.T) {
	const expiredJWT = "e30.eyJleHAiOjF9.eA"

	var refreshCalls atomic.Int32
	tc := newToolCtx(Deps{
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:          "default",
			WToken:        expiredJWT,
			WRefreshToken: "bootstrap-refresh",
		}},
		Wolt: &refreshingStubWolt{
			stubWolt: &stubWolt{},
			refreshFn: func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
				refreshCalls.Add(1)
				return woltgateway.TokenRefreshResult{}, &woltgateway.UpstreamRequestError{
					StatusCode: 503,
					RetryAfter: 1750 * time.Millisecond,
					Body:       "must not leak",
				}
			},
		},
	})
	auth := woltgateway.AuthContext{WToken: expiredJWT, RefreshToken: "bootstrap-refresh"}
	_, err := invokeWithRefresh(
		context.Background(),
		tc,
		&auth,
		func(woltgateway.AuthContext) (string, error) {
			return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
		},
	)
	if err == nil {
		t.Fatal("invokeWithRefresh succeeded after refresh failure")
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("RefreshAccessToken calls = %d, want 1", got)
	}

	result := &mcp.CallToolResult{IsError: true}
	result.SetError(toolErr(err))
	normalizeToolErrorResult(result)
	info, ok := result.Meta["wolt_error"].(ToolErrorInfo)
	if !ok {
		t.Fatalf("_meta.wolt_error type = %T", result.Meta["wolt_error"])
	}
	if info.Code != "SESSION_REFRESH_FAILED" || !info.Retryable ||
		info.RetryAfterMS != 1750 || strings.Contains(info.Message, "must not leak") {
		t.Fatalf("refresh failure metadata = %+v", info)
	}
}

func TestMCPExpiredAccessWithoutRefreshTokenPreservesAuthExpired(t *testing.T) {
	const expiredJWT = "e30.eyJleHAiOjF9.eA"

	tc := newToolCtx(Deps{
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: expiredJWT}},
		Wolt: &refreshingStubWolt{
			stubWolt: &stubWolt{},
			refreshFn: func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
				t.Fatal("RefreshAccessToken must not be called without a refresh token")
				return woltgateway.TokenRefreshResult{}, nil
			},
		},
	})
	auth := woltgateway.AuthContext{WToken: expiredJWT}
	_, err := invokeWithRefresh(
		context.Background(),
		tc,
		&auth,
		func(woltgateway.AuthContext) (string, error) {
			return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
		},
	)
	if info := classifiedInfo(t, err); info.Code != "AUTH_EXPIRED" {
		t.Fatalf("classification = %+v, want AUTH_EXPIRED", info)
	}
}

func TestMCPProfileConfigErrorsHaveStableClassifications(t *testing.T) {
	tests := []struct {
		name      string
		findErr   error
		wantCode  string
		retryable bool
	}{
		{
			name:     "invalid",
			findErr:  fmt.Errorf("decode config: %w", configstore.ErrInvalidConfig),
			wantCode: "CONFIG_INVALID",
		},
		{
			name:      "unavailable",
			findErr:   errors.New("config read denied"),
			wantCode:  "CONFIG_UNAVAILABLE",
			retryable: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newToolCtx(Deps{Profiles: &stubProfiles{findErr: test.findErr}})
			_, _, err := tc.requireAuth(context.Background())
			info := classifiedInfo(t, err)
			if info.Code != test.wantCode || info.Retryable != test.retryable {
				t.Fatalf("classification = %+v", info)
			}
		})
	}
}

func TestMCPUpstreamClassificationsPreserveStatusPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  string
		retryable bool
	}{
		{
			name: "invalid success response",
			err: &woltgateway.UpstreamRequestError{
				StatusCode: 200,
				Cause:      woltgateway.ErrInvalidResponse,
			},
			wantCode:  "UPSTREAM_INVALID_RESPONSE",
			retryable: true,
		},
		{
			name: "invalid refresh success response",
			err: &tokenRefreshError{cause: &woltgateway.UpstreamRequestError{
				StatusCode: 200,
				Cause:      woltgateway.ErrInvalidResponse,
			}},
			wantCode:  "UPSTREAM_INVALID_RESPONSE",
			retryable: true,
		},
		{
			name: "ordinary unsupported method",
			err: &woltgateway.UpstreamRequestError{
				StatusCode: 405,
			},
			wantCode: "UNSUPPORTED_ENDPOINT",
		},
		{
			name: "missing refresh endpoint",
			err: &tokenRefreshError{cause: &woltgateway.UpstreamRequestError{
				StatusCode: 404,
			}},
			wantCode: "UNSUPPORTED_ENDPOINT",
		},
		{
			name: "generic gone endpoint",
			err: &woltgateway.UpstreamRequestError{
				StatusCode: 410,
				Body:       "resource removed",
			},
			wantCode: "UNSUPPORTED_ENDPOINT",
		},
		{
			name: "outdated client",
			err: &woltgateway.UpstreamRequestError{
				StatusCode: 410,
				Body:       "Please update your app",
			},
			wantCode: "CLIENT_OUTDATED",
		},
		{
			name: "non-success status wins over invalid cause",
			err: &woltgateway.UpstreamRequestError{
				StatusCode: 503,
				Cause:      woltgateway.ErrInvalidResponse,
			},
			wantCode:  "UPSTREAM_TEMPORARY",
			retryable: true,
		},
		{
			name: "refresh non-success status wins over invalid cause",
			err: &tokenRefreshError{cause: &woltgateway.UpstreamRequestError{
				StatusCode: 503,
				Cause:      woltgateway.ErrInvalidResponse,
			}},
			wantCode:  "SESSION_REFRESH_FAILED",
			retryable: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := classifiedInfo(t, test.err)
			if info.Code != test.wantCode || info.Retryable != test.retryable {
				t.Fatalf("classification = %+v", info)
			}
		})
	}
}
