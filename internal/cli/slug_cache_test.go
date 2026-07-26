package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// withIsolatedSlugCache points the venue slug cache at a per-test temp
// file so concurrent tests can't observe each other's resolutions through
// the on-disk cache. Helper because resolveVenueReference is otherwise a
// global-state singleton.
func withIsolatedSlugCache(t *testing.T) {
	t.Helper()
	t.Setenv(envSlugCachePath, filepath.Join(t.TempDir(), "slug-cache.json"))
}

func TestSlugCacheRoundTrip(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	if id, ok := lookupCachedVenueID("hesburger-kamppi"); ok || id != "" {
		t.Fatalf("expected miss on empty cache, got %q (ok=%v)", id, ok)
	}

	rememberVenueID("hesburger-kamppi", "6348098a9157ab2b10bdaf65")

	id, ok := lookupCachedVenueID("hesburger-kamppi")
	if !ok {
		t.Fatal("expected cache hit after remember")
	}
	if id != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("expected cached id, got %q", id)
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("expected cache file on disk, got error: %v", err)
	}
	if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("expected 0600 perms, got %#o", mode)
	}
}

func TestSlugCacheTTLExpiresEntries(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	stale := venueSlugCache{
		Venues: map[string]cachedVenueID{
			"hesburger-kamppi": {ID: "6348098a9157ab2b10bdaf65", FetchedAt: time.Now().Add(-2 * slugCacheTTL)},
		},
	}
	payload, _ := json.Marshal(stale)
	if err := os.WriteFile(cachePath, payload, 0o600); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	if _, ok := lookupCachedVenueID("hesburger-kamppi"); ok {
		t.Fatal("expected stale entry to be ignored")
	}
}

func TestSlugCacheCorruptFileIsTreatedAsEmpty(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	if err := os.WriteFile(cachePath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt cache: %v", err)
	}

	if _, ok := lookupCachedVenueID("hesburger-kamppi"); ok {
		t.Fatal("expected corrupt file to produce a miss")
	}
	// Writing over a corrupt cache must succeed.
	rememberVenueID("hesburger-kamppi", "6348098a9157ab2b10bdaf65")
	if id, ok := lookupCachedVenueID("hesburger-kamppi"); !ok || id != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("expected recovery write to land, got %q (ok=%v)", id, ok)
	}
}

func TestClearVenueSlugCacheRemovesFile(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)
	rememberVenueID("hesburger-kamppi", "6348098a9157ab2b10bdaf65")

	clearVenueSlugCache()

	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected cache file removed, stat err=%v", err)
	}
	// Calling again on a missing file must be a no-op (no panic, no error).
	clearVenueSlugCache()
}

func TestResolveVenueReferenceUsesCacheToSkipStaticLookup(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	calls := 0
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			calls++
			if slug != "hesburger-kamppi" {
				t.Fatalf("unexpected slug %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": "6348098a9157ab2b10bdaf65"}}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	// Cold call hits upstream and persists.
	first, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("cold resolve: %v", err)
	}
	if first.VenueID != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("cold resolve returned id %q", first.VenueID)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one upstream call on cold path, got %d", calls)
	}

	// Warm call must NOT hit upstream.
	second, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("warm resolve: %v", err)
	}
	if second.VenueID != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("warm resolve returned id %q", second.VenueID)
	}
	if calls != 1 {
		t.Fatalf("expected cached lookup to skip upstream, got %d total calls", calls)
	}
}

func TestResolveVenueReferenceCachedEntryIsScopedToSlug(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	rememberVenueID("hesburger-kamppi", "6348098a9157ab2b10bdaf65")

	calls := 0
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			calls++
			return map[string]any{"venue": map[string]any{"id": "ff0000000000000000000001"}}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	cached, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("resolve cached slug: %v", err)
	}
	if cached.VenueID != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("expected cached id to win, got %q", cached.VenueID)
	}
	if calls != 0 {
		t.Fatalf("expected zero upstream calls for cached slug, got %d", calls)
	}

	uncached, err := resolveVenueReference(context.Background(), deps, "other-venue")
	if err != nil {
		t.Fatalf("resolve uncached slug: %v", err)
	}
	if uncached.VenueID != "ff0000000000000000000001" {
		t.Fatalf("expected fresh id for uncached slug, got %q", uncached.VenueID)
	}
	if calls != 1 {
		t.Fatalf("expected one upstream call for the uncached slug, got %d", calls)
	}
}

func TestCachedVenuePageStaticPersistsFullPayload(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	calls := 0
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			calls++
			return map[string]any{
				"venue":         map[string]any{"id": "6348098a9157ab2b10bdaf65"},
				"order_minimum": map[string]any{"amount": 0},
			}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	first, err := cachedVenuePageStatic(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("cold call: %v", err)
	}
	if asMap(first["venue"])["id"] != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("cold call returned unexpected payload: %+v", first)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one upstream call on cold path, got %d", calls)
	}

	second, err := cachedVenuePageStatic(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("warm call: %v", err)
	}
	if asMap(second["venue"])["id"] != "6348098a9157ab2b10bdaf65" {
		t.Fatalf("warm call returned unexpected payload: %+v", second)
	}
	if calls != 1 {
		t.Fatalf("expected cached lookup to skip upstream, got %d total calls", calls)
	}
	// Sanity-check that the non-id field round-tripped via JSON in the cache file.
	if om := asMap(second["order_minimum"])["amount"]; om == nil {
		t.Fatalf("expected full payload to round-trip through cache, got %+v", second)
	}
}

func TestCachedVenuePageStaticDoesNotPersistUpstreamErrors(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	wolt := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("simulated upstream failure")
		},
	}
	deps := Dependencies{Wolt: wolt}

	if _, err := cachedVenuePageStatic(context.Background(), deps, "hesburger-kamppi"); err == nil {
		t.Fatal("expected upstream error to propagate")
	}
	if _, statErr := os.Stat(cachePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no cache file when upstream errored, stat err=%v", statErr)
	}
}

func TestLogoutClearsSlugCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "slug-cache.json")
	t.Setenv(envSlugCachePath, cachePath)

	rememberVenueID("hesburger-kamppi", "6348098a9157ab2b10bdaf65")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected seeded cache file, got: %v", err)
	}

	cfg := &normalizingConfigStub{
		cfg: domain.Config{
			Account: domain.Account{WToken: "abc.def.ghi"},
		},
	}
	deps := Dependencies{Config: cfg}

	cmd := newLogoutCommand(deps)
	cmd.SetArgs([]string{"--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected slug cache wiped by logout, stat err=%v", err)
	}
}
