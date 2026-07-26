package cli

import (
	"context"
	"errors"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestNormalizeVenueInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"plain slug", "hesburger-helsinki-kamppi", "hesburger-helsinki-kamppi"},
		{"slug with whitespace", "  hesburger-kamppi  ", "hesburger-kamppi"},
		{"object id", "6123456789abcdef01234567", "6123456789abcdef01234567"},
		{"restaurant url", "https://wolt.com/fi/fin/helsinki/restaurant/hesburger-helsinki-kamppi", "hesburger-helsinki-kamppi"},
		{"restaurant url trailing slash", "https://wolt.com/en/fin/helsinki/restaurant/hesburger-helsinki-kamppi/", "hesburger-helsinki-kamppi"},
		{"venue url", "https://wolt.com/en/fin/helsinki/venue/wolt-market-niittari", "wolt-market-niittari"},
		{"nested category url", "https://wolt.com/en/xx/example-city/venue/example-market/categories/produce", "example-market"},
		{"nested item url", "https://wolt.com/en/xx/example-city/restaurant/example-restaurant/itemid-000000000000000000000001", "example-restaurant"},
		{"discovery url", "https://wolt.com/en/discovery/helsinki/", "helsinki"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeVenueInput(tc.raw)
			if got != tc.want {
				t.Fatalf("normalizeVenueInput(%q) = %q; want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResolveVenueReferenceSlugQueriesStaticPage(t *testing.T) {
	withIsolatedSlugCache(t)
	calls := 0
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			calls++
			if slug != "hesburger-kamppi" {
				t.Fatalf("expected slug to be passed through, got %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": "6123456789abcdef01234567"}}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueSlug != "hesburger-kamppi" {
		t.Fatalf("expected slug to be preserved, got %q", ref.VenueSlug)
	}
	if ref.VenueID != "6123456789abcdef01234567" {
		t.Fatalf("expected resolved venue id, got %q", ref.VenueID)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one static-page call, got %d", calls)
	}
}

func TestResolveVenueReferenceSlugFallsBackToDynamicWhenStaticFails(t *testing.T) {
	withIsolatedSlugCache(t)
	staticCalls := 0
	dynamicCalls := 0
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			staticCalls++
			// Wolt now 404s the static page for most venues.
			return nil, errors.New("status 404")
		},
		venuePageDynamicFn: func(_ context.Context, slug string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			dynamicCalls++
			if slug != "eat-poke-iso-omena" {
				t.Fatalf("expected slug passed to dynamic page, got %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": "637e383476c00f021e6bf084"}}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "eat-poke-iso-omena")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueID != "637e383476c00f021e6bf084" {
		t.Fatalf("expected id resolved via dynamic fallback, got %q", ref.VenueID)
	}
	if staticCalls != 1 || dynamicCalls != 1 {
		t.Fatalf("expected static=1 dynamic=1, got static=%d dynamic=%d", staticCalls, dynamicCalls)
	}
	// The resolved id must be cached so a second lookup skips both upstreams.
	if cached, ok := lookupCachedVenueID("eat-poke-iso-omena"); !ok || cached != "637e383476c00f021e6bf084" {
		t.Fatalf("expected resolved id to be cached, got %q ok=%v", cached, ok)
	}
}

func TestResolveVenueReferenceSlugSkipsDynamicWhenStaticResolves(t *testing.T) {
	withIsolatedSlugCache(t)
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, _ string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": "6123456789abcdef01234567"}}, nil
		},
		venuePageDynamicFn: func(_ context.Context, slug string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			t.Fatalf("dynamic page must not be called when static resolves, got slug %q", slug)
			return nil, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueID != "6123456789abcdef01234567" {
		t.Fatalf("expected id from static payload, got %q", ref.VenueID)
	}
}

func TestResolveVenueReferenceURLExtractsSlugThenResolves(t *testing.T) {
	withIsolatedSlugCache(t)
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			if slug != "wolt-market-niittari" {
				t.Fatalf("expected slug from URL path, got %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": "6111111111aaaaaaaa222222"}}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "https://wolt.com/en/fin/espoo/venue/wolt-market-niittari")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueSlug != "wolt-market-niittari" {
		t.Fatalf("expected slug from URL, got %q", ref.VenueSlug)
	}
	if ref.VenueID != "6111111111aaaaaaaa222222" {
		t.Fatalf("expected resolved id from static payload, got %q", ref.VenueID)
	}
}

func TestResolveVenueReferenceObjectIDReverseLookup(t *testing.T) {
	withIsolatedSlugCache(t)
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, reference string) (map[string]any, error) {
			if reference != "6123456789abcdef01234567" {
				t.Fatalf("expected object id to be passed through, got %q", reference)
			}
			return map[string]any{
				"venue": map[string]any{
					"id":   reference,
					"slug": "example-restaurant",
				},
			}, nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "6123456789abcdef01234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueID != "6123456789abcdef01234567" {
		t.Fatalf("expected id preserved, got %q", ref.VenueID)
	}
	if ref.VenueSlug != "example-restaurant" {
		t.Fatalf("expected slug from supported venue payload, got %q", ref.VenueSlug)
	}
}

func TestResolveCartAddVenueReferenceAcceptsRequestedSlugAlias(t *testing.T) {
	withIsolatedSlugCache(t)
	const venueID = "6123456789abcdef01234567"
	wolt := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{
					"id":   venueID,
					"slug": "canonical-market",
				},
			}, nil
		},
	}

	reference, err := resolveCartAddVenueReference(
		context.Background(),
		Dependencies{Wolt: wolt},
		"legacy-market",
		"legacy-market",
	)
	if err != nil {
		t.Fatalf("requested alias was rejected: %v", err)
	}
	if reference.ID != venueID ||
		reference.Slug != "legacy-market" ||
		!reference.explicitSlugVerified {
		t.Fatalf("unexpected cart venue reference: %+v", reference)
	}
}

func TestResolveVenueReferenceEmptyInput(t *testing.T) {
	withIsolatedSlugCache(t)
	deps := Dependencies{}

	ref, err := resolveVenueReference(context.Background(), deps, "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueID != "" || ref.VenueSlug != "" {
		t.Fatalf("expected empty resolution for blank input, got %+v", ref)
	}
}

func TestResolveItemReference(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantID   string
		wantSlug string
	}{
		{"empty", "", "", ""},
		{"whitespace", "   ", "", ""},
		{"plain object id", "67dbda2656a6f0831337ecdb", "67dbda2656a6f0831337ecdb", ""},
		{"object id with whitespace", " 67dbda2656a6f0831337ecdb ", "67dbda2656a6f0831337ecdb", ""},
		{"bare slug is not an item", "the-bastard-classic-cheese", "", ""},
		{
			"itemid url",
			"https://wolt.com/en/fin/helsinki/venue/bastard-burgers-mikonkatu/itemid-67dbda2656a6f0831337ecdb",
			"67dbda2656a6f0831337ecdb",
			"bastard-burgers-mikonkatu",
		},
		{
			"menuitem url",
			"https://wolt.com/en/fin/helsinki/venue/some-place/menuitem-67dbda2656a6f0831337ecdb",
			"67dbda2656a6f0831337ecdb",
			"some-place",
		},
		{
			"restaurant url with itemid",
			"https://wolt.com/en/fin/helsinki/restaurant/burger-king-finnoo/itemid-676939cb70769df4cec6cc6f",
			"676939cb70769df4cec6cc6f",
			"burger-king-finnoo",
		},
		{
			"url with itemid query parameter",
			"https://wolt.com/en/fin/helsinki/venue/foo?itemid=67dbda2656a6f0831337ecdb",
			"67dbda2656a6f0831337ecdb",
			"foo",
		},
		{
			"url with trailing object id as last segment",
			"https://wolt.com/en/fin/helsinki/venue/foo/67dbda2656a6f0831337ecdb",
			"67dbda2656a6f0831337ecdb",
			"foo",
		},
		{
			"third-party item URL is rejected",
			"https://example.com/en/example/venue/foo/itemid-67dbda2656a6f0831337ecdb",
			"",
			"",
		},
		{
			"lookalike Wolt host is rejected",
			"https://wolt.com.example.org/en/example/venue/foo/itemid-67dbda2656a6f0831337ecdb",
			"",
			"",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := resolveItemReference(tc.raw)
			if got.ItemID != tc.wantID {
				t.Fatalf("ItemID: want %q, got %q", tc.wantID, got.ItemID)
			}
			if got.VenueSlugHint != tc.wantSlug {
				t.Fatalf("VenueSlugHint: want %q, got %q", tc.wantSlug, got.VenueSlugHint)
			}
		})
	}
}

func TestResolveVenueReferenceKeepsUnverifiedSlugOutOfVenueID(t *testing.T) {
	withIsolatedSlugCache(t)
	wolt := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, _ string) (map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
		venuePageDynamicFn: func(
			_ context.Context,
			_ string,
			_ woltgateway.VenuePageDynamicOptions,
		) (map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
	}
	deps := Dependencies{Wolt: wolt}

	ref, err := resolveVenueReference(context.Background(), deps, "hesburger-kamppi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.VenueSlug != "hesburger-kamppi" {
		t.Fatalf("expected slug echoed back when API is down, got %q", ref.VenueSlug)
	}
	if ref.VenueID != "" {
		t.Fatalf("unverified slug must not be emitted as venue id, got %q", ref.VenueID)
	}
}
