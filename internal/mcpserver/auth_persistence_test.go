package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	profileservice "github.com/mekedron/wolt-cli/internal/service/profile"
)

func TestMCPRefreshPersistsAccessForServerRestart(t *testing.T) {
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
		}},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var refreshCalls atomic.Int32
	wolt := &refreshingStubWolt{
		stubWolt: &stubWolt{},
		refreshFn: func(_ context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
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
	firstServer := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: profileservice.NewResolver(store),
		Config:   store,
	})
	_, auth, err := firstServer.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("first server requireAuth: %v", err)
	}
	if err := firstServer.refreshTokens(context.Background(), &auth); err != nil {
		t.Fatalf("first server refresh: %v", err)
	}
	if auth.WToken != "fresh-access" ||
		auth.RefreshToken != "rotated-process-local" {
		t.Fatalf("first server in-memory auth = %+v", auth)
	}

	onDisk, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load persisted credentials: %v", err)
	}
	persisted := configstore.CredentialsFromConfig(onDisk)
	if persisted.AccessToken != "fresh-access" ||
		persisted.RefreshToken != "bootstrap-refresh" ||
		len(persisted.Cookies) != 1 ||
		persisted.Cookies[0] != "session=stable" {
		t.Fatalf("persisted credentials = %+v", persisted)
	}

	restartedStore, err := configstore.NewStore()
	if err != nil {
		t.Fatalf("create restarted store: %v", err)
	}
	restarted := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: profileservice.NewResolver(restartedStore),
		Config:   restartedStore,
	})
	_, restartedAuth, err := restarted.requireAuth(context.Background())
	if err != nil {
		t.Fatalf("restarted server requireAuth: %v", err)
	}
	if restartedAuth.WToken != "fresh-access" ||
		restartedAuth.RefreshToken != "bootstrap-refresh" {
		t.Fatalf("restarted auth = %+v", restartedAuth)
	}
	got, err := invokeWithRefresh(
		context.Background(),
		restarted,
		&restartedAuth,
		func(current woltgateway.AuthContext) (string, error) {
			if current.WToken != "fresh-access" {
				return "", fmt.Errorf("restarted server used %q", current.WToken)
			}
			return "ok", nil
		},
	)
	if err != nil || got != "ok" {
		t.Fatalf("restarted invocation = %q, %v", got, err)
	}
	if calls := refreshCalls.Load(); calls != 1 {
		t.Fatalf("refresh calls = %d, want one before restart only", calls)
	}
}
