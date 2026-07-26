package cli

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	profileservice "github.com/mekedron/wolt-cli/internal/service/profile"
)

func TestAuthRefreshPersistsAccessForNextCLIProcess(t *testing.T) {
	t.Setenv(envDisableChromeSync, "1")
	store := seedAuthPersistenceStore(t)
	var refreshCalls atomic.Int32
	wolt := &testWoltAPI{
		refreshAccessTokenFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
			refreshCalls.Add(1)
			if refreshToken != "bootstrap-refresh" || auth.WToken != "stale-access" {
				return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected refresh input")
			}
			return woltgateway.TokenRefreshResult{
				AccessToken:  "fresh-access",
				RefreshToken: "rotated-process-local",
			}, nil
		},
	}
	deps := Dependencies{
		Wolt:     wolt,
		Config:   store,
		Profiles: profileservice.NewResolver(store),
	}
	auth, err := loadAuthContextWithProfile(context.Background(), deps, globalFlags{})
	if err != nil {
		t.Fatalf("load initial auth: %v", err)
	}

	got, warnings, err := invokeWithAuthAutoRefresh(
		context.Background(),
		deps,
		globalFlags{},
		&auth,
		func(current woltgateway.AuthContext) (string, error) {
			switch current.WToken {
			case "stale-access":
				return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
			case "fresh-access":
				return "ok", nil
			default:
				return "", fmt.Errorf("unexpected request access token %q", current.WToken)
			}
		},
	)
	if err != nil || got != "ok" {
		t.Fatalf("first invocation = %q, %v", got, err)
	}
	if strings.Contains(strings.Join(warnings, "\n"), "failed to persist") {
		t.Fatalf("unexpected persistence warning: %v", warnings)
	}
	if auth.RefreshToken != "rotated-process-local" {
		t.Fatalf("in-process refresh token = %q", auth.RefreshToken)
	}

	onDisk, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load persisted auth: %v", err)
	}
	persisted := configstore.CredentialsFromConfig(onDisk)
	if persisted.AccessToken != "fresh-access" ||
		persisted.RefreshToken != "bootstrap-refresh" ||
		len(persisted.Cookies) != 1 ||
		persisted.Cookies[0] != "session=stable" {
		t.Fatalf("persisted credentials = %+v", persisted)
	}

	reloadedStore, err := configstore.NewStore()
	if err != nil {
		t.Fatalf("create second store: %v", err)
	}
	reloadedDeps := Dependencies{
		Wolt:     wolt,
		Config:   reloadedStore,
		Profiles: profileservice.NewResolver(reloadedStore),
	}
	reloadedAuth, err := loadAuthContextWithProfile(
		context.Background(),
		reloadedDeps,
		globalFlags{},
	)
	if err != nil {
		t.Fatalf("reload auth: %v", err)
	}
	if reloadedAuth.WToken != "fresh-access" ||
		reloadedAuth.RefreshToken != "bootstrap-refresh" {
		t.Fatalf("reloaded auth = %+v", reloadedAuth)
	}
	if _, _, err := invokeWithAuthAutoRefresh(
		context.Background(),
		reloadedDeps,
		globalFlags{},
		&reloadedAuth,
		func(current woltgateway.AuthContext) (string, error) {
			if current.WToken != "fresh-access" {
				return "", fmt.Errorf("second process used %q", current.WToken)
			}
			return "ok", nil
		},
	); err != nil {
		t.Fatalf("second process invocation: %v", err)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want one across sequential processes", got)
	}
}

func TestConcurrentLoginOrLogoutWinsOverCLIRefreshPersistence(t *testing.T) {
	tests := []struct {
		name    string
		replace func(*testing.T, *configstore.Store)
		want    configstore.Credentials
	}{
		{
			name: "login",
			replace: func(t *testing.T, store *configstore.Store) {
				t.Helper()
				if err := saveAccountCredentials(
					context.Background(),
					Dependencies{Config: store},
					woltgateway.AuthContext{
						WToken:       "login-access",
						RefreshToken: "login-bootstrap",
						Cookies:      []string{"session=login"},
					},
				); err != nil {
					t.Fatalf("concurrent login: %v", err)
				}
			},
			want: configstore.Credentials{
				AccessToken:  "login-access",
				RefreshToken: "login-bootstrap",
				Cookies:      []string{"session=login"},
			},
		},
		{
			name: "logout",
			replace: func(t *testing.T, store *configstore.Store) {
				t.Helper()
				cmd := newLogoutCommand(Dependencies{Config: store})
				output := &bytes.Buffer{}
				cmd.SetOut(output)
				cmd.SetErr(output)
				cmd.SetArgs([]string{"--format", "json"})
				if err := cmd.ExecuteContext(context.Background()); err != nil {
					t.Fatalf("concurrent logout: %v", err)
				}
			},
			want: configstore.Credentials{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envDisableChromeSync, "1")
			store := seedAuthPersistenceStore(t)
			refreshStarted := make(chan struct{})
			releaseRefresh := make(chan struct{})
			var refreshCalls atomic.Int32
			wolt := &testWoltAPI{
				refreshAccessTokenFn: func(_ context.Context, _ string, _ woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
					if refreshCalls.Add(1) != 1 {
						return woltgateway.TokenRefreshResult{}, fmt.Errorf("unexpected second refresh")
					}
					close(refreshStarted)
					<-releaseRefresh
					return woltgateway.TokenRefreshResult{
						AccessToken:  "refresh-access",
						RefreshToken: "rotated-process-local",
					}, nil
				},
			}
			deps := Dependencies{
				Wolt:     wolt,
				Config:   store,
				Profiles: profileservice.NewResolver(store),
			}
			auth, err := loadAuthContextWithProfile(
				context.Background(),
				deps,
				globalFlags{},
			)
			if err != nil {
				t.Fatalf("load auth: %v", err)
			}

			type invocationResult struct {
				warnings []string
				err      error
			}
			done := make(chan invocationResult, 1)
			go func() {
				_, warnings, invokeErr := invokeWithAuthAutoRefresh(
					context.Background(),
					deps,
					globalFlags{},
					&auth,
					func(current woltgateway.AuthContext) (string, error) {
						if current.WToken == "stale-access" {
							return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
						}
						if current.WToken != "refresh-access" {
							return "", fmt.Errorf("unexpected request access token %q", current.WToken)
						}
						return "ok", nil
					},
				)
				done <- invocationResult{warnings: warnings, err: invokeErr}
			}()

			select {
			case <-refreshStarted:
			case <-time.After(5 * time.Second):
				close(releaseRefresh)
				t.Fatal("timed out waiting for token refresh to start")
			}
			test.replace(t, store)
			close(releaseRefresh)
			var result invocationResult
			select {
			case result = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for refreshing invocation to finish")
			}
			if result.err != nil {
				t.Fatalf("refreshing invocation: %v", result.err)
			}
			if !strings.Contains(strings.Join(result.warnings, "\n"), "changed concurrently") {
				t.Fatalf("warnings = %v, want concurrent-change notice", result.warnings)
			}

			cfg, err := store.Load(context.Background())
			if err != nil {
				t.Fatalf("load final config: %v", err)
			}
			got := configstore.CredentialsFromConfig(cfg)
			if !got.Equal(test.want) {
				t.Fatalf("final credentials = %+v, want %+v", got, test.want)
			}
			if cfg.Account.Location != (domain.Location{Lat: 60.1, Lon: 24.9}) {
				t.Fatalf("concurrent credential update changed location: %+v", cfg.Account.Location)
			}
		})
	}
}

func TestOpportunisticChromeAuthRemainsProcessLocal(t *testing.T) {
	store := seedAuthPersistenceStore(t)
	expiredAccess := buildExpiringJWT(time.Now().Add(-2 * time.Hour).Unix())
	chromeAccess := buildExpiringJWT(time.Now().Add(-time.Hour).Unix())
	refreshedAccess := buildExpiringJWT(time.Now().Add(time.Hour).Unix())
	if err := configstore.ApplyUpdate(
		context.Background(),
		store,
		func(cfg *domain.Config) (bool, error) {
			configstore.SetCredentials(cfg, configstore.Credentials{
				AccessToken:  expiredAccess,
				RefreshToken: "bootstrap-refresh",
				Cookies:      []string{"session=stable"},
			})
			return true, nil
		},
	); err != nil {
		t.Fatalf("seed expired access: %v", err)
	}

	originalPull := pullOpportunisticChromeAuth
	pullOpportunisticChromeAuth = func(context.Context, string) (woltgateway.AuthContext, bool, error) {
		return woltgateway.AuthContext{
			WToken:       chromeAccess,
			RefreshToken: "chrome-refresh",
			Cookies:      []string{"session=chrome"},
		}, true, nil
	}
	t.Cleanup(func() { pullOpportunisticChromeAuth = originalPull })

	deps := Dependencies{
		Wolt: &testWoltAPI{
			refreshAccessTokenFn: func(
				_ context.Context,
				refreshToken string,
				current woltgateway.AuthContext,
			) (woltgateway.TokenRefreshResult, error) {
				if refreshToken != "chrome-refresh" ||
					current.WToken != chromeAccess ||
					len(current.Cookies) != 1 ||
					current.Cookies[0] != "session=chrome" {
					return woltgateway.TokenRefreshResult{},
						fmt.Errorf("refresh auth = %+v", current)
				}
				return woltgateway.TokenRefreshResult{
					AccessToken:  refreshedAccess,
					RefreshToken: "chrome-rotated",
				}, nil
			},
		},
		Config:   store,
		Profiles: profileservice.NewResolver(store),
	}
	auth, err := loadAuthContextWithProfile(
		context.Background(),
		deps,
		globalFlags{},
	)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	got, _, err := invokeWithAuthAutoRefresh(
		context.Background(),
		deps,
		globalFlags{},
		&auth,
		func(current woltgateway.AuthContext) (string, error) {
			if current.WToken != refreshedAccess ||
				current.RefreshToken != "chrome-rotated" ||
				len(current.Cookies) != 1 ||
				current.Cookies[0] != "session=chrome" {
				return "", fmt.Errorf("request auth = %+v", current)
			}
			return "ok", nil
		},
	)
	if err != nil || got != "ok" {
		t.Fatalf("invocation = %q, %v", got, err)
	}

	onDisk, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load persisted auth: %v", err)
	}
	persisted := configstore.CredentialsFromConfig(onDisk)
	if persisted.AccessToken != expiredAccess ||
		persisted.RefreshToken != "bootstrap-refresh" ||
		len(persisted.Cookies) != 1 ||
		persisted.Cookies[0] != "session=stable" {
		t.Fatalf("opportunistic Chrome auth reached disk: %+v", persisted)
	}
}

func TestCredentialPersistenceRejectsRefreshTokenMismatch(t *testing.T) {
	store := seedAuthPersistenceStore(t)
	deps := Dependencies{Config: store}
	auth := woltgateway.AuthContext{
		WToken:       "stale-access",
		RefreshToken: "bootstrap-refresh",
		Cookies:      []string{"session=stable"},
	}
	if err := saveAccountCredentials(
		context.Background(),
		deps,
		woltgateway.AuthContext{
			WToken:       auth.WToken,
			RefreshToken: "replacement-refresh",
			Cookies:      auth.Cookies,
		},
	); err != nil {
		t.Fatalf("replace saved refresh token: %v", err)
	}

	persistence := newCredentialPersistence(
		context.Background(),
		deps,
		auth,
		true,
	)
	attempted, persisted, err := persistence.persistAccess(
		context.Background(),
		"must-not-persist",
	)
	if err != nil || !attempted || persisted {
		t.Fatalf(
			"persistAccess = attempted %v, persisted %v, err %v",
			attempted,
			persisted,
			err,
		)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	got := configstore.CredentialsFromConfig(cfg)
	if got.AccessToken != auth.WToken ||
		got.RefreshToken != "replacement-refresh" {
		t.Fatalf("mismatched credentials were overwritten: %+v", got)
	}
}

func seedAuthPersistenceStore(t *testing.T) *configstore.Store {
	t.Helper()
	t.Setenv(
		"WOLT_CONFIG_PATH",
		filepath.Join(t.TempDir(), "wolt-config.json"),
	)
	store, err := configstore.NewStore()
	if err != nil {
		t.Fatalf("create config store: %v", err)
	}
	if err := store.Save(context.Background(), domain.Config{
		Profiles: []domain.Profile{{
			Name:          "default",
			IsDefault:     true,
			WToken:        "stale-access",
			WRefreshToken: "bootstrap-refresh",
			Cookies:       []string{"session=stable"},
			Location:      domain.Location{Lat: 60.1, Lon: 24.9},
		}},
	}); err != nil {
		t.Fatalf("seed auth config: %v", err)
	}
	return store
}
