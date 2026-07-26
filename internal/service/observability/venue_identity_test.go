package observability

import "testing"

func TestExtractVenueIdentityPrefersExplicitPayloadIdentityOverFallbackAlias(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "resolved-venue"
	)
	got := ExtractVenueIdentity(
		VenueIdentity{ID: "unresolved-alias", Slug: "unresolved-alias"},
		map[string]any{
			"venue_id":   venueID,
			"venue_slug": venueSlug,
		},
	)
	if got.ID != venueID || got.Slug != venueSlug {
		t.Fatalf("identity = %#v, want payload venue identity", got)
	}
}

func TestExtractVenueIdentityDoesNotTreatCategoryOrItemRootAsVenue(t *testing.T) {
	fallback := VenueIdentity{
		ID:           "fallback-venue-id",
		Slug:         "fallback-venue",
		CanonicalURL: "https://wolt.com/en/example/venue/fallback-venue",
	}
	tests := []struct {
		name    string
		payload map[string]any
		wantID  string
	}{
		{
			name: "category",
			payload: map[string]any{
				"id":       "category-id",
				"slug":     "category-slug",
				"name":     "Category",
				"item_ids": []any{"item-1"},
			},
			wantID: fallback.ID,
		},
		{
			name: "item with explicit venue id",
			payload: map[string]any{
				"id":         "item-id",
				"slug":       "item-slug",
				"name":       "Item",
				"price":      500,
				"venue_id":   "explicit-venue-id",
				"public_url": "https://wolt.com/en/example/venue/other-venue/items/item-id",
			},
			wantID: "explicit-venue-id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ExtractVenueIdentity(fallback, test.payload)
			if got.ID != test.wantID {
				t.Fatalf("id = %q, want %q", got.ID, test.wantID)
			}
			if got.Slug != fallback.Slug {
				t.Fatalf("slug = %q, want fallback venue slug %q", got.Slug, fallback.Slug)
			}
			if got.CanonicalURL != fallback.CanonicalURL {
				t.Fatalf("canonical URL = %q, want fallback %q", got.CanonicalURL, fallback.CanonicalURL)
			}
		})
	}
}

func TestExtractVenueIdentityNormalizesNestedVenueURL(t *testing.T) {
	got := ExtractVenueIdentity(
		VenueIdentity{Slug: "example-market"},
		map[string]any{
			"venue_slug": "example-market",
			"public_url": "http://wolt.com/en/example/venue/example-market/items/item-1?source=test#details",
		},
	)
	const want = "https://wolt.com/en/example/venue/example-market"
	if got.CanonicalURL != want {
		t.Fatalf("canonical URL = %q, want %q", got.CanonicalURL, want)
	}
}

func TestExtractVenueIdentityAcceptsBareFieldsOnlyForVenueShapedPayload(t *testing.T) {
	got := ExtractVenueIdentity(
		VenueIdentity{ID: "fallback-id", Slug: "fallback-slug"},
		map[string]any{
			"id":       "root-venue-id",
			"slug":     "root-venue-slug",
			"timezone": "UTC",
		},
	)
	if got.ID != "root-venue-id" || got.Slug != "root-venue-slug" {
		t.Fatalf("identity = %#v, want root venue identity", got)
	}
}

func TestFindVenueURLUsesStableExplicitPrecedence(t *testing.T) {
	const (
		canonicalURL = "https://wolt.com/en/example/venue/canonical-venue"
		publicURL    = "https://wolt.com/en/example/venue/public-venue"
		shareURL     = "https://wolt.com/en/example/venue/share-venue"
		nestedAURL   = "https://wolt.com/en/example/venue/nested-a"
	)
	explicit := map[string]any{
		"share_url":     shareURL,
		"public_url":    publicURL,
		"canonical_url": canonicalURL,
	}
	for attempt := 0; attempt < 100; attempt++ {
		if got := findVenueURL(explicit, ""); got != canonicalURL {
			t.Fatalf("attempt %d: explicit URL = %q, want %q", attempt, got, canonicalURL)
		}
	}

	nested := map[string]any{
		"z_wrapper": map[string]any{"public_url": publicURL},
		"a_wrapper": map[string]any{"public_url": nestedAURL},
	}
	for attempt := 0; attempt < 100; attempt++ {
		if got := findVenueURL(nested, ""); got != nestedAURL {
			t.Fatalf("attempt %d: nested URL = %q, want %q", attempt, got, nestedAURL)
		}
	}
}
