package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
