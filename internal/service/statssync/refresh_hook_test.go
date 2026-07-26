package statssync

import (
	"context"
	"errors"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestBuildRefreshHookRequiresRefreshCredential(t *testing.T) {
	calls := 0
	hook := buildRefreshHook(
		&woltgateway.AuthContext{WToken: "access-only"},
		func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			calls++
			return woltgateway.TokenRefreshResult{AccessToken: "unexpected"}, nil
		},
		nil,
		nil,
	)
	if hook != nil {
		t.Fatal("buildRefreshHook returned a refresh path without a refresh credential")
	}
	if calls != 0 {
		t.Fatalf("refresher calls = %d, want zero", calls)
	}
}

func TestBuildRefreshHookClassifiesMissingAccessToken(t *testing.T) {
	hook := buildRefreshHook(
		&woltgateway.AuthContext{RefreshToken: "bootstrap-refresh"},
		func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			return woltgateway.TokenRefreshResult{}, nil
		},
		nil,
		nil,
	)
	if hook == nil {
		t.Fatal("buildRefreshHook returned nil with a refresh credential")
	}
	if err := hook(context.Background()); !errors.Is(err, woltgateway.ErrInvalidResponse) {
		t.Fatalf("refresh error = %v, want ErrInvalidResponse", err)
	}
}

func TestBuildRefreshHookNotifiesWithFreshAccess(t *testing.T) {
	auth := woltgateway.AuthContext{
		WToken:       "stale-access",
		RefreshToken: "bootstrap-refresh",
		Cookies:      []string{"session=stable"},
	}
	callbackCalls := 0
	persistedAccess := ""
	hook := buildRefreshHook(
		&auth,
		func(_ context.Context, refreshToken string, current woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			if refreshToken != "bootstrap-refresh" || current.WToken != "stale-access" {
				t.Fatalf("unexpected refresh input: %q, %+v", refreshToken, current)
			}
			return woltgateway.TokenRefreshResult{
				AccessToken:  "fresh-access",
				RefreshToken: "rotated-process-local",
			}, nil
		},
		func(updated woltgateway.AuthContext) error {
			callbackCalls++
			persistedAccess = updated.WToken
			return nil
		},
		nil,
	)
	if hook == nil {
		t.Fatal("buildRefreshHook returned nil with a refresh credential")
	}
	if err := hook(context.Background()); err != nil {
		t.Fatalf("refresh hook: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("persistence callback calls = %d, want one", callbackCalls)
	}
	if auth.WToken != "fresh-access" ||
		auth.RefreshToken != "rotated-process-local" {
		t.Fatalf("process-local auth = %+v", auth)
	}
	if persistedAccess != "fresh-access" {
		t.Fatalf("persistence callback access = %q, want fresh-access", persistedAccess)
	}
}
