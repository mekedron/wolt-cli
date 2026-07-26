package mcpserver

import (
	"context"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestVenueItemToolsCarryResolvedVenueCurrencyAndCategoryContext(t *testing.T) {
	const (
		venueID      = "000000000000000000000001"
		venueSlug    = "unicode-market"
		itemID       = "000000000000000000000101"
		canonicalURL = "https://wolt.com/en/test/venue/unicode-market"
	)
	itemPayload := map[string]any{
		"categories": []any{
			map[string]any{
				"id":       "cat-fish",
				"name":     "Fish",
				"item_ids": []any{itemID},
			},
		},
		"items": []any{
			map[string]any{
				"id":                  itemID,
				"name":                "Frozen fish",
				"name_ja":             "冷凍魚",
				"description":         "Frozen fish fillet",
				"price":               1890,
				"purchasable_balance": 5,
				"images": []any{
					map[string]any{"url": "https://images.example/items/frozen-fish"},
				},
				"translations": map[string]any{
					"ja": map[string]any{"name": "冷凍魚"},
					"fr": map[string]any{"name": "Poisson surgelé"},
				},
			},
		},
	}
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{
				"id":        venueID,
				"slug":      venueSlug,
				"currency":  "EUR",
				"share_url": canonicalURL,
			}}, nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return itemPayload, nil
		},
		assortmentFn: func(context.Context, string) (map[string]any, error) {
			return itemPayload, nil
		},
		assortmentSearchFn: func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error) {
			return itemPayload, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, menu, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue: venueID,
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if menu.Data["venue_id"] != venueID || menu.Data["venue_slug"] != venueSlug ||
		menu.Data["canonical_url"] != canonicalURL || menu.Data["currency"] != "EUR" {
		t.Fatalf("menu venue context = %#v", menu.Data)
	}
	menuRows := asSlice(menu.Data["items"])
	if len(menuRows) != 1 {
		t.Fatalf("menu items = %#v", menuRows)
	}
	menuItem := asMap(menuRows[0])
	assertItemContext(t, menuItem, venueID, venueSlug, canonicalURL)
	if menuItem["name_ja"] != "冷凍魚" || asMap(menuItem["translations"])["fr"] == nil {
		t.Fatalf("menu language variants = %#v", menuItem)
	}
	for _, absent := range []string{"name_fr", "original_name", "localized_name"} {
		if _, exists := menuItem[absent]; exists {
			t.Fatalf("menu invented %s: %#v", absent, menuItem)
		}
	}

	_, detail, err := tc.handleVenueItem(context.Background(), nil, VenueItemInput{
		Venue:  venueID,
		ItemID: itemID,
	})
	if err != nil {
		t.Fatalf("handleVenueItem: %v", err)
	}
	assertItemContext(t, detail.Item, venueID, venueSlug, canonicalURL)

	_, search, err := tc.handleVenueSearchItems(context.Background(), nil, VenueSearchItemsInput{
		Venue: venueID,
		Query: "frozen fish",
	})
	if err != nil {
		t.Fatalf("handleVenueSearchItems: %v", err)
	}
	if search.Data["venue_id"] != venueID || search.Data["venue_slug"] != venueSlug ||
		search.Data["canonical_url"] != canonicalURL || search.Data["currency"] != "EUR" {
		t.Fatalf("search venue context = %#v", search.Data)
	}
	rows := asSlice(search.Data["items"])
	if len(rows) != 1 {
		t.Fatalf("search items = %#v", rows)
	}
	assertItemContext(t, asMap(rows[0]), venueID, venueSlug, canonicalURL)
}

func assertItemContext(t *testing.T, item map[string]any, venueID string, venueSlug string, canonicalURL string) {
	t.Helper()
	if item["venue_id"] != venueID || item["venue_slug"] != venueSlug || item["canonical_url"] != canonicalURL {
		t.Fatalf("venue context = %#v", item)
	}
	price := asMap(item["price"])
	if price["amount"] != 1890 || price["currency"] != "EUR" || price["formatted_amount"] != "EUR 18.90" {
		t.Fatalf("price = %#v", price)
	}
	if item["category_id"] != "cat-fish" || item["category_name"] != "Fish" {
		t.Fatalf("category = %#v", item)
	}
	if item["description"] != "Frozen fish fillet" ||
		item["image_url"] != "https://images.example/items/frozen-fish" {
		t.Fatalf("description/image = %#v", item)
	}
}
