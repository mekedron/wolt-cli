package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestNewStoreUsesEnvConfigPath(t *testing.T) {
	t.Setenv(envConfigPath, "/tmp/custom-wolt-config.json")
	store, err := NewStore()
	if err != nil {
		t.Fatalf("unexpected error creating store: %v", err)
	}
	if store.Path() != "/tmp/custom-wolt-config.json" {
		t.Fatalf("expected env path, got %q", store.Path())
	}
}

func TestStoreSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := &Store{path: path}

	input := domain.Config{
		Profiles: []domain.Profile{
			{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1699, Lon: 24.9384}},
		},
	}
	if err := store.Save(context.Background(), input); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	output, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if len(output.Profiles) != 1 || output.Profiles[0].Name != "default" {
		t.Fatalf("unexpected roundtrip config: %+v", output)
	}
}

func TestStoreLoadMissingConfig(t *testing.T) {
	store := &Store{path: filepath.Join(t.TempDir(), "missing.json")}
	_, err := store.Load(context.Background())
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestStoreLoadInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	store := &Store{path: path}
	_, err := store.Load(context.Background())
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestStoreSaveRejectsEmptyProfiles(t *testing.T) {
	store := &Store{path: filepath.Join(t.TempDir(), "config.json")}
	err := store.Save(context.Background(), domain.Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestStoreLoadMigratesLegacyMultiProfileWithExplicitDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{
		"profiles": [
			{"name": "work", "is_default": false, "wtoken": "work-token", "location": {"lat": 60.1, "lon": 24.9}},
			{"name": "home", "is_default": true, "wtoken": "home-token", "wrefresh_token": "home-refresh", "wolt_address_id": "home-addr", "location": {"lat": 60.2, "lon": 25.0}}
		]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	store := &Store{path: path}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected single normalized profile, got %d: %+v", len(cfg.Profiles), cfg.Profiles)
	}
	if cfg.Profiles[0].Name != "default" || !cfg.Profiles[0].IsDefault {
		t.Fatalf("expected normalized default profile, got %+v", cfg.Profiles[0])
	}
	if cfg.Account.WToken != "home-token" || cfg.Account.WRefreshToken != "home-refresh" || cfg.Account.WoltAddressID != "home-addr" {
		t.Fatalf("expected account to mirror the legacy default profile, got %+v", cfg.Account)
	}
	if cfg.Account.Location.Lat != 60.2 || cfg.Account.Location.Lon != 25.0 {
		t.Fatalf("expected location from default profile, got %+v", cfg.Account.Location)
	}
}

func TestStoreLoadMigratesLegacyMultiProfileWithoutDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{
		"profiles": [
			{"name": "first", "wtoken": "first-token", "location": {"lat": 1, "lon": 2}},
			{"name": "second", "wtoken": "second-token", "location": {"lat": 3, "lon": 4}}
		]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	store := &Store{path: path}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.Account.WToken != "first-token" {
		t.Fatalf("expected first profile to win when no explicit default; got %+v", cfg.Account)
	}
	if cfg.Profiles[0].Name != "default" {
		t.Fatalf("expected profile renamed to default; got %q", cfg.Profiles[0].Name)
	}
}

func TestStoreLoadRoundTripsNewAccountOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	payload := `{
		"account": {
			"wtoken": "abc.def.ghi",
			"wrefresh_token": "ref",
			"cookies": ["__wtoken=abc.def.ghi"],
			"wolt_address_id": "addr-7",
			"location": {"lat": 60.5, "lon": 25.5}
		}
	}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write account config: %v", err)
	}
	store := &Store{path: path}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.Account.WToken != "abc.def.ghi" {
		t.Fatalf("expected token to survive load, got %+v", cfg.Account)
	}

	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatalf("save round-trip failed: %v", err)
	}
	reloaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("reload after save: %v", err)
	}
	if !reflect.DeepEqual(reloaded.Account, cfg.Account) {
		t.Fatalf("round-trip diverged:\nbefore=%+v\nafter=%+v", cfg.Account, reloaded.Account)
	}
}

func TestStoreSavePersistsProfileMutationAfterLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"account": {
			"wtoken": "old-access",
			"wrefresh_token": "old-refresh",
			"cookies": ["__wtoken=old-access"],
			"location": {"lat": 60.1, "lon": 24.9}
		}
	}`), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	store := &Store{path: path}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load seeded config: %v", err)
	}
	if cfg.Account.WToken != "old-access" || cfg.Profiles[0].WToken != "old-access" {
		t.Fatalf("expected loaded account+profile mirror, got %+v / %+v", cfg.Account, cfg.Profiles[0])
	}

	// Simulate an explicit profile credential update after Load.
	cfg.Profiles[0].WToken = "new-access"
	cfg.Profiles[0].WRefreshToken = "new-refresh"

	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Account.WToken != "new-access" {
		t.Fatalf("expected rotated access token to persist, got %q", reloaded.Account.WToken)
	}
	if reloaded.Account.WRefreshToken != "new-refresh" {
		t.Fatalf("expected rotated refresh token to persist, got %q", reloaded.Account.WRefreshToken)
	}
}

func TestStoreSaveUsesOwnerOnlyPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := &Store{path: path}
	cfg := domain.Config{
		Account: domain.Account{WToken: "abc.def.ghi"},
	}
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("expected owner-only permissions 0600, got %#o", mode)
	}
}

func TestStoreLoadNeverObservesPartialConcurrentSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writer := &Store{path: path}
	reader := &Store{path: path}
	configFor := func(token string) domain.Config {
		return domain.Config{Profiles: []domain.Profile{{
			Name:          "default",
			IsDefault:     true,
			WToken:        token,
			WRefreshToken: "refresh-token",
			Cookies:       []string{strings.Repeat(token, 4096)},
			Location:      domain.Location{Lat: 10, Lon: 20},
		}}}
	}
	if err := writer.Save(context.Background(), configFor("initial-token")); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		<-start
		for index := 0; index < 50; index++ {
			if err := writer.Save(context.Background(), configFor("rotated-token")); err != nil {
				errs <- err
				return
			}
		}
	}()
	for range 3 {
		go func() {
			defer wg.Done()
			<-start
			for index := 0; index < 500; index++ {
				cfg, err := reader.Load(context.Background())
				if err != nil {
					errs <- err
					return
				}
				if len(cfg.Profiles) != 1 ||
					(cfg.Profiles[0].WToken != "initial-token" &&
						cfg.Profiles[0].WToken != "rotated-token") {
					errs <- errors.New("reader observed an incomplete config")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent load/save: %v", err)
	}
}
