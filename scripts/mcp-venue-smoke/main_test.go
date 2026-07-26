package main

import "testing"

func TestVenueIdentityFromDetailExtractsCanonicalFields(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"venue_id":      "000000000000000000000001",
			"slug":          "example-venue",
			"canonical_url": "https://wolt.com/en/example/venue/example-venue",
		},
	}

	identity, err := venueIdentityFromDetail(payload)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ID != "000000000000000000000001" ||
		identity.Slug != "example-venue" ||
		identity.CanonicalURL != "https://wolt.com/en/example/venue/example-venue" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestValidateHoursAcceptsEmptySchedule(t *testing.T) {
	err := validateHours(map[string]any{
		"hours": map[string]any{
			"venue_id":        "000000000000000000000001",
			"opening_windows": []any{},
		},
	}, "000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateHoursRejectsMalformedSchedule(t *testing.T) {
	err := validateHours(map[string]any{
		"hours": map[string]any{
			"venue_id":        "000000000000000000000001",
			"opening_windows": "always",
		},
	}, "000000000000000000000001")
	if err == nil {
		t.Fatal("expected malformed opening_windows to fail")
	}
}

func TestUniqueVenueReferencesDeduplicatesCaseInsensitively(t *testing.T) {
	refs := uniqueVenueReferences("Example-Venue", "example-venue", "000000000000000000000001")
	if len(refs) != 2 || refs[0] != "Example-Venue" || refs[1] != "000000000000000000000001" {
		t.Fatalf("references = %#v", refs)
	}
}
