package cli

import (
	"strings"
	"testing"
)

func TestCurrencyAndAmountHelpers(t *testing.T) {
	if got := inferCurrency("€12.34"); got != "EUR" {
		t.Fatalf("expected EUR, got %q", got)
	}
	if got := inferCurrency("$12.34"); got != "USD" {
		t.Fatalf("expected USD, got %q", got)
	}
	if got := formatMinorAmount(1595, "EUR"); got != "€15.95" {
		t.Fatalf("expected €15.95, got %q", got)
	}
	if got := formatMinorAmount(0, "EUR"); got != "€0.00" {
		t.Fatalf("expected €0.00, got %q", got)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"a", "b", "a", "", "c", "b"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("unexpected deduped values: %v", got)
	}
}

func TestSelectBasketWithMeta(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{"id": "basket-1", "venue": map[string]any{"id": "venue-1", "name": "A", "slug": "venue-a"}},
			map[string]any{"id": "basket-2", "venue": map[string]any{"id": "venue-2", "name": "B"}},
		},
	}

	selected, meta, warnings := selectBasketWithMeta(page, "")
	if asString(selected["id"]) != "basket-1" {
		t.Fatalf("expected first basket to be selected, got %v", selected["id"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning when multiple baskets exist")
	}
	if asString(meta["selection_mode"]) != "first-available" {
		t.Fatalf("expected first-available selection mode, got %v", meta["selection_mode"])
	}

	selectedBySlug, metaBySlug, warningsBySlug := selectBasketWithMeta(page, "venue-a")
	if asString(selectedBySlug["id"]) != "basket-1" {
		t.Fatalf("expected basket-1 selected by slug, got %v", selectedBySlug["id"])
	}
	if len(warningsBySlug) != 0 {
		t.Fatalf("expected no warnings for explicit slug selection, got %v", warningsBySlug)
	}
	if asString(metaBySlug["selection_mode"]) != "requested-venue-slug" {
		t.Fatalf("expected requested-venue-slug selection mode, got %v", metaBySlug["selection_mode"])
	}
}

func TestBuildCartStateAndLineDetails(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":    "basket-1",
				"total": "€18.00",
				"venue": map[string]any{"id": "venue-1", "name": "Venue 1", "slug": "venue-1"},
				"telemetry": map[string]any{
					"basket_total": 1800,
				},
				"items": []any{
					map[string]any{
						"id":    "item-1",
						"name":  "Combo",
						"count": 1,
						"price": 1700,
						"options": []any{
							map[string]any{
								"id":   "drink",
								"name": "Drink",
								"values": []any{
									map[string]any{"id": "cola", "name": "Cola", "count": 2, "price": 50},
								},
							},
						},
					},
				},
			},
		},
	}

	data, warnings := buildCartState(page, "")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if asString(data["basket_id"]) != "basket-1" {
		t.Fatalf("expected basket-1, got %v", data["basket_id"])
	}
	if asInt(asMap(data["total"])["amount"]) != 1800 {
		t.Fatalf("expected total amount 1800, got %v", asMap(data["total"])["amount"])
	}

	lines := asSlice(data["lines"])
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	details := cartLineDetails(asMap(lines[0]), "EUR")
	if len(details) != 1 || !strings.Contains(details[0], "Drink: Cola x2 (+€0.50)") {
		t.Fatalf("unexpected line details: %v", details)
	}
}

func TestBuildBasketMutationItem(t *testing.T) {
	line := map[string]any{
		"id":    "item-1",
		"name":  "Combo",
		"price": 1700,
		"total": "€17.00",
		"options": []any{
			map[string]any{
				"id": "drink",
				"values": []any{
					map[string]any{"id": "cola", "count": 1, "price": 100},
				},
			},
		},
	}
	item := buildBasketMutationItem(line, 2)
	if asInt(item["price"]) != 3400 {
		t.Fatalf("expected mutation price 3400, got %v", item["price"])
	}
	opts := asSlice(item["options"])
	if len(opts) != 1 {
		t.Fatalf("expected one option group in mutation item, got %d", len(opts))
	}
}

func TestBuildBasketUpsertItemKeepsUnitPrice(t *testing.T) {
	line := map[string]any{
		"id":    "item-1",
		"name":  "Combo",
		"price": 1700,
		"options": []any{
			map[string]any{
				"id": "drink",
				"values": []any{
					map[string]any{"id": "cola", "count": 1, "price": 100},
				},
			},
		},
	}
	item := buildBasketUpsertItem(line, 3)
	if asInt(item["price"]) != 1700 {
		t.Fatalf("expected unit price 1700, got %v", item["price"])
	}
	if asInt(item["count"]) != 3 {
		t.Fatalf("expected count 3, got %v", item["count"])
	}
}

func TestBuildItemOptionsDataAndTable(t *testing.T) {
	payload := map[string]any{
		"price": map[string]any{"currency": "EUR"},
		"option_groups": []any{
			map[string]any{
				"id":   "group-drink",
				"name": "Drink",
				"min":  1,
				"max":  1,
				"values": []any{
					map[string]any{"id": "value-cola", "name": "Cola", "price": map[string]any{"amount": 100}},
				},
			},
		},
	}

	data, warnings := buildItemOptionsData("venue-1", "item-1", payload, nil)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if asInt(data["group_count"]) != 1 {
		t.Fatalf("expected group_count=1, got %v", data["group_count"])
	}
	rendered := buildItemOptionsTable(data)
	if !strings.Contains(rendered, "--option group-drink=value-cola") {
		t.Fatalf("expected rendered example option, got:\n%s", rendered)
	}
}

func TestBuildItemDetailTableFormatsGroups(t *testing.T) {
	data := map[string]any{
		"name":        "Combo",
		"item_id":     "item-1",
		"venue_id":    "venue-1",
		"description": "Test item",
		"price": map[string]any{
			"formatted_amount": "€10.00",
		},
		"option_groups": []any{
			map[string]any{
				"group_id": "group-drink",
				"name":     "Drink",
				"required": true,
				"min":      1,
				"max":      1,
			},
		},
		"upsell_items": []any{},
	}

	rendered := buildItemDetailTable(data)
	for _, expected := range []string{
		"Option groups\t1",
		"Upsell items\t0",
		"Option groups\nGroup ID\tName\tRequired\tMin\tMax",
		"group-drink\tDrink\tyes\t1\t1",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestTokenPreviewAndExpiryFormatting(t *testing.T) {
	if got := tokenPreview("abcdefghijklmnop"); got != "abcdef...klmnop" {
		t.Fatalf("unexpected token preview: %q", got)
	}
	if got := tokenPreview("short"); got != "short" {
		t.Fatalf("unexpected short token preview: %q", got)
	}
	if got := tokenExpiryRFC3339("bad-token"); got != "" {
		t.Fatalf("expected empty expiry for invalid token, got %q", got)
	}
}
