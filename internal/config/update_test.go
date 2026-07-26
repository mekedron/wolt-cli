package config

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestStoreUpdateSerializesIndependentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := &Store{path: path}
	second := &Store{path: path}
	if err := first.Save(context.Background(), domain.Config{
		Profiles: []domain.Profile{{
			Name:          "default",
			IsDefault:     true,
			WToken:        "initial-access",
			WRefreshToken: "bootstrap-refresh",
			Cookies:       []string{"session=initial"},
			Location:      domain.Location{Lat: 60.1, Lon: 24.9},
		}},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.Update(context.Background(), func(cfg *domain.Config) (bool, error) {
			if got := CredentialsFromConfig(*cfg).AccessToken; got != "initial-access" {
				return false, errors.New("first update did not load the seed")
			}
			cfg.Profiles[0].WToken = "first-access"
			cfg.Account.WToken = "first-access"
			close(firstEntered)
			<-releaseFirst
			return true, nil
		})
	}()
	select {
	case <-firstEntered:
	case <-time.After(5 * time.Second):
		close(releaseFirst)
		t.Fatal("timed out waiting for first update to acquire the file lock")
	}

	var secondMutatorCalled atomic.Bool
	waitCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	secondErr := second.Update(waitCtx, func(*domain.Config) (bool, error) {
		secondMutatorCalled.Store(true)
		return true, nil
	})
	close(releaseFirst)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first update: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first update to finish")
	}
	if !errors.Is(secondErr, context.DeadlineExceeded) {
		t.Fatalf("contending update error = %v, want context deadline", secondErr)
	}
	if secondMutatorCalled.Load() {
		t.Fatal("contending mutator ran before the first writer released the file lock")
	}

	if err := second.Update(context.Background(), func(cfg *domain.Config) (bool, error) {
		if got := CredentialsFromConfig(*cfg).AccessToken; got != "first-access" {
			return false, errors.New("second update did not observe the first transaction")
		}
		cfg.Profiles[0].WoltAddressID = "address-after-first"
		cfg.Account.WoltAddressID = "address-after-first"
		return true, nil
	}); err != nil {
		t.Fatalf("second update after release: %v", err)
	}

	got, err := first.Load(context.Background())
	if err != nil {
		t.Fatalf("load final config: %v", err)
	}
	if got.Account.WToken != "first-access" ||
		got.Account.WRefreshToken != "bootstrap-refresh" ||
		got.Account.WoltAddressID != "address-after-first" {
		t.Fatalf("unexpected final account: %+v", got.Account)
	}
}

func TestCompareAndSwapAccessPinsBootstrapCredentials(t *testing.T) {
	cfg := domain.Config{
		Account: domain.Account{
			WToken:        "old-access",
			WRefreshToken: "bootstrap-refresh",
			Cookies:       []string{"session=stable", "device=stable"},
			WoltAddressID: "address-1",
			Location:      domain.Location{Lat: 60.1, Lon: 24.9},
		},
		Profiles: []domain.Profile{{
			Name:          "default",
			IsDefault:     true,
			WToken:        "old-access",
			WRefreshToken: "bootstrap-refresh",
			Cookies:       []string{"session=stable", "device=stable"},
			WoltAddressID: "address-1",
			Location:      domain.Location{Lat: 60.1, Lon: 24.9},
		}},
	}
	expected := CredentialsFromConfig(cfg)

	if !CompareAndSwapAccess(&cfg, expected, "new-access") {
		t.Fatal("access-token compare-and-swap unexpectedly rejected its snapshot")
	}
	got := CredentialsFromConfig(cfg)
	if got.AccessToken != "new-access" || got.RefreshToken != "bootstrap-refresh" {
		t.Fatalf("credentials after access swap = %+v", got)
	}
	if !slices.Equal(got.Cookies, expected.Cookies) {
		t.Fatalf("cookies changed during access-only swap: %v", got.Cookies)
	}
	if cfg.Account.WoltAddressID != "address-1" ||
		cfg.Account.Location != (domain.Location{Lat: 60.1, Lon: 24.9}) {
		t.Fatalf("non-credential state changed during access swap: %+v", cfg.Account)
	}

	concurrentLogin := expected
	concurrentLogin.RefreshToken = "replacement-bootstrap"
	SetCredentials(&cfg, concurrentLogin)
	if CompareAndSwapAccess(&cfg, got, "must-not-win") {
		t.Fatal("stale access-token swap overwrote a concurrent login")
	}
	if final := CredentialsFromConfig(cfg); final.RefreshToken != "replacement-bootstrap" ||
		final.AccessToken == "must-not-win" {
		t.Fatalf("concurrent credentials were not preserved: %+v", final)
	}
}

func TestSetCredentialsPreservesAccountOnlyProfileState(t *testing.T) {
	cfg := domain.Config{Account: domain.Account{
		Location:      domain.Location{Lat: 60.1, Lon: 24.9},
		WoltAddressID: "address-1",
	}}

	SetCredentials(&cfg, Credentials{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Cookies:      []string{"session=stable"},
	})

	if len(cfg.Profiles) != 1 ||
		cfg.Profiles[0].Location != cfg.Account.Location ||
		cfg.Profiles[0].WoltAddressID != cfg.Account.WoltAddressID {
		t.Fatalf("profile state was not preserved: %+v", cfg)
	}
}
