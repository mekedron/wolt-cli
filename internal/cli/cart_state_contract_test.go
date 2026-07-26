package cli

import (
	"testing"
)

func TestNeedsAssortmentFallbackRequiresHydratedOptionValues(t *testing.T) {
	incomplete := map[string]any{
		"price": map[string]any{"amount": 1000},
		"options": []any{
			map[string]any{"id": "size", "values": []any{}},
		},
	}
	if !needsAssortmentFallback(incomplete) {
		t.Fatal("ID-only option metadata must trigger assortment fallback")
	}

	complete := map[string]any{
		"price": map[string]any{"amount": 1000},
		"options": []any{
			map[string]any{
				"id": "size",
				"values": []any{
					map[string]any{"id": "large", "price": map[string]any{"amount": 200}},
				},
			},
		},
	}
	if needsAssortmentFallback(complete) {
		t.Fatal("hydrated option metadata unexpectedly triggered fallback")
	}
}

func TestBuildCartStateNormalizesMissingCountAndHonorsExplicitZeroTotal(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":       "basket-1",
				"total":    "€0.00",
				"currency": "EUR",
				"venue":    map[string]any{"id": "venue-1"},
				"items": []any{
					map[string]any{
						"id":    "item-1",
						"name":  "Example item",
						"price": 500,
						"options": []any{
							map[string]any{
								"id": "extra",
								"values": []any{
									map[string]any{"id": "addon", "price": map[string]any{"amount": 100}},
								},
							},
						},
					},
				},
				"telemetry": map[string]any{"basket_total": 0},
			},
		},
	}

	state, _ := buildCartState(page, "venue-1")
	if asInt(state["total_items"]) != 1 || asInt(asMap(asSlice(state["lines"])[0])["count"]) != 1 {
		t.Fatalf("missing basket count was not normalized: %#v", state)
	}
	if got := asInt(asMap(state["subtotal"])["amount"]); got != 600 {
		t.Fatalf("configured subtotal = %d, want 600", got)
	}
	total := asMap(state["total"])
	if asInt(total["amount"]) != 0 || asString(total["formatted_amount"]) != "€0.00" {
		t.Fatalf("explicit free basket total was not preserved: %#v", total)
	}
}

func TestFindBasketLineByIDAcceptsObjectIDCase(t *testing.T) {
	basket := map[string]any{
		"items": []any{
			map[string]any{"id": "abcdefabcdefabcdefabcdef", "count": 2},
		},
	}
	line, count := findBasketLineByID(basket, "ABCDEFABCDEFABCDEFABCDEF")
	if line == nil || count != 2 {
		t.Fatalf("case-insensitive lookup = (%#v, %d)", line, count)
	}
}
