package observability_test

import (
	"reflect"
	"testing"

	"github.com/mekedron/wolt-cli/internal/service/observability"
)

func TestBuildItemSearchResultCarriesScopedVenueAndItemContext(t *testing.T) {
	const (
		itemID       = "000000000000000000000101"
		venueID      = "000000000000000000000001"
		venueSlug    = "unicode-market"
		canonicalURL = "https://wolt.com/en/test/venue/unicode-market"
	)
	translations := map[string]any{
		"ja": map[string]any{"name": "冷凍魚"},
		"fr": map[string]any{"name": "Poisson surgelé"},
	}

	data, warnings := observability.BuildItemSearchResult(
		"frozen fish",
		[]map[string]any{itemSearchContextFixture(itemID, translations)},
		"",
		nil,
		0,
		nil,
		observability.ItemVenueContext{
			VenueID:      venueID,
			VenueSlug:    venueSlug,
			CanonicalURL: canonicalURL,
			Currency:     "EUR",
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if data["venue_id"] != venueID || data["venue_slug"] != venueSlug {
		t.Fatalf("top-level venue context = %#v", data)
	}
	if data["canonical_url"] != canonicalURL || data["currency"] != "EUR" {
		t.Fatalf("top-level canonical/currency context = %#v", data)
	}

	items := asSlice(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := asMap(t, items[0])
	if item["venue_id"] != venueID || item["venue_slug"] != venueSlug || item["canonical_url"] != canonicalURL {
		t.Fatalf("item venue context = %#v", item)
	}
	if item["description"] != "Flash-frozen fish" {
		t.Fatalf("description = %v", item["description"])
	}
	if item["category_id"] != "category-seafood" || item["category_name"] != "Seafood" {
		t.Fatalf("category context = %#v", item)
	}
	price := asMap(t, item["price"])
	if price["amount"] != 1890 || price["currency"] != "EUR" || price["formatted_amount"] != "EUR 18.90" {
		t.Fatalf("price = %#v", price)
	}
	if !reflect.DeepEqual(item["price"], item["base_price"]) {
		t.Fatalf("price and compatibility base_price diverged: price=%#v base_price=%#v", item["price"], item["base_price"])
	}
	if item["image_url"] != "https://images.example/items/frozen-fish-primary" {
		t.Fatalf("image_url = %v", item["image_url"])
	}
	if !reflect.DeepEqual(item["translations"], translations) {
		t.Fatalf("translations = %#v", item["translations"])
	}
	if _, exists := item["original_name"]; exists {
		t.Fatalf("original_name was invented: %#v", item["original_name"])
	}
}

func TestBuildItemSearchResultPreservesAuthoritativeCrossLanguageMatches(t *testing.T) {
	const itemID = "000000000000000000000101"
	payload := itemSearchContextFixture(itemID, map[string]any{
		"ja": map[string]any{"name": "冷凍魚"},
	})
	payload["items"].([]any)[0].(map[string]any)["name"] = "冷凍魚"

	data, warnings := observability.BuildItemSearchResult(
		"profile-language-query-not-in-display-name",
		[]map[string]any{payload},
		"",
		nil,
		0,
		nil,
		observability.ItemVenueContext{VenueID: "000000000000000000000001"},
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	items := asSlice(t, data["items"])
	if len(items) != 1 || asMap(t, items[0])["name"] != "冷凍魚" {
		t.Fatalf("authoritative upstream match was filtered out: %#v", items)
	}
}

func TestBuildItemDetailCarriesCategoryDescriptionAndOnlyUpstreamLanguageVariants(t *testing.T) {
	const (
		itemID       = "item-1"
		venueID      = "venue-1"
		venueSlug    = "unicode-market"
		canonicalURL = "https://wolt.com/en/test/venue/unicode-market"
	)
	payload := map[string]any{
		"categories": []any{
			map[string]any{
				"id":       "category-fish",
				"name":     "Fish",
				"item_ids": []any{itemID},
			},
		},
		"items": []any{
			map[string]any{
				"id":          itemID,
				"name":        "冷凍魚",
				"name_ja":     "冷凍魚",
				"description": "急速冷凍",
				"price":       1890,
				"images": []any{
					map[string]any{"url": "https://images.example/items/frozen-fish-primary"},
				},
			},
		},
	}

	data, warnings := observability.BuildItemDetail(
		itemID,
		venueID,
		payload,
		false,
		observability.ItemVenueContext{
			VenueID:      venueID,
			VenueSlug:    venueSlug,
			CanonicalURL: canonicalURL,
			Currency:     "EUR",
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if data["venue_id"] != venueID || data["venue_slug"] != venueSlug || data["canonical_url"] != canonicalURL {
		t.Fatalf("venue context = %#v", data)
	}
	if data["description"] != "急速冷凍" {
		t.Fatalf("description = %v", data["description"])
	}
	if data["category_id"] != "category-fish" || data["category_name"] != "Fish" {
		t.Fatalf("category context = %#v", data)
	}
	price := asMap(t, data["price"])
	if price["amount"] != 1890 || price["currency"] != "EUR" || price["formatted_amount"] != "EUR 18.90" {
		t.Fatalf("price = %#v", price)
	}
	if data["currency"] != "EUR" {
		t.Fatalf("top-level currency = %v", data["currency"])
	}
	if data["image_url"] != "https://images.example/items/frozen-fish-primary" {
		t.Fatalf("image_url = %v", data["image_url"])
	}
	if data["name_ja"] != "冷凍魚" {
		t.Fatalf("name_ja = %v", data["name_ja"])
	}
	for _, absent := range []string{"name_fr", "translations", "localized_name", "original_name"} {
		if _, exists := data[absent]; exists {
			t.Fatalf("%s was invented: %#v", absent, data[absent])
		}
	}
}

func TestBuildItemDetailDoesNotFormatUnknownPriceAsZero(t *testing.T) {
	data, _ := observability.BuildItemDetail(
		"item-unknown-price",
		"venue-1",
		map[string]any{
			"items": []any{
				map[string]any{
					"item_id": "item-unknown-price",
					"name":    "Unknown price",
					"base_price": map[string]any{
						"amount": nil,
					},
				},
			},
		},
		false,
		observability.ItemVenueContext{Currency: "EUR"},
	)

	price := asMap(t, data["price"])
	if price["amount"] != nil {
		t.Fatalf("amount = %#v, want nil", price["amount"])
	}
	if price["formatted_amount"] != nil {
		t.Fatalf("formatted_amount = %#v, want nil", price["formatted_amount"])
	}
	if price["currency"] != "EUR" {
		t.Fatalf("currency = %#v, want EUR", price["currency"])
	}
}

func itemSearchContextFixture(itemID string, translations map[string]any) map[string]any {
	return map[string]any{
		"categories": []any{
			map[string]any{
				"id":       "category-seafood",
				"name":     "Seafood",
				"item_ids": []any{itemID},
			},
		},
		"items": []any{
			map[string]any{
				"id":           itemID,
				"name":         "Frozen fish",
				"description":  "Flash-frozen fish",
				"price":        1890,
				"translations": translations,
				"images": []any{
					map[string]any{"url": "https://images.example/items/frozen-fish-primary"},
					map[string]any{"url": "https://images.example/items/frozen-fish-secondary"},
				},
			},
		},
	}
}
