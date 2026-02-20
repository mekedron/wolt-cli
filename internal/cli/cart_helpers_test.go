package cli

import "testing"

func TestParseOptionSelectionsWithCounts(t *testing.T) {
	parsed, err := parseOptionSelections([]string{"group-1=value-1", "group-1=value-2:3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values := parsed["group-1"]
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0].ValueID != "value-1" || values[0].Count != 1 {
		t.Fatalf("unexpected first selection: %+v", values[0])
	}
	if values[1].ValueID != "value-2" || values[1].Count != 3 {
		t.Fatalf("unexpected second selection: %+v", values[1])
	}
}

func TestBuildBasketOptionsResolvesGroupAndValueNames(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			map[string]any{
				"id":   "grp-drink",
				"name": "Drink",
				"values": []any{
					map[string]any{"id": "val-cola", "name": "Cola", "price": map[string]any{"amount": 100}},
					map[string]any{"id": "val-water", "name": "Water", "price": map[string]any{"amount": 0}},
				},
			},
		},
	}

	selected, err := parseOptionSelections([]string{"Drink=Cola:2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	options := buildBasketOptions(payload, selected)
	if len(options) != 1 {
		t.Fatalf("expected one option group, got %d", len(options))
	}
	group := asMap(options[0])
	if asString(group["id"]) != "grp-drink" {
		t.Fatalf("expected group id grp-drink, got %v", group["id"])
	}
	values := asSlice(group["values"])
	if len(values) != 1 {
		t.Fatalf("expected one selected value, got %d", len(values))
	}
	value := asMap(values[0])
	if asString(value["id"]) != "val-cola" {
		t.Fatalf("expected value id val-cola, got %v", value["id"])
	}
	if asInt(value["count"]) != 2 {
		t.Fatalf("expected count 2, got %v", value["count"])
	}
	if asInt(value["price"]) != 100 {
		t.Fatalf("expected price 100, got %v", value["price"])
	}
}

func TestBuildItemPayloadFromAssortment(t *testing.T) {
	assortment := map[string]any{
		"items": []any{
			map[string]any{
				"id":      "item-1",
				"name":    "Combo",
				"price":   1590,
				"options": []any{map[string]any{"option_id": "grp-drink"}},
			},
		},
		"options": []any{
			map[string]any{
				"id":   "grp-drink",
				"name": "Drink",
				"values": []any{
					map[string]any{"id": "val-cola", "name": "Cola", "price": 100},
				},
			},
		},
	}

	itemPayload := buildItemPayloadFromAssortment(assortment, "item-1")
	if itemPayload == nil {
		t.Fatalf("expected assortment payload for item")
	}
	if asString(itemPayload["name"]) != "Combo" {
		t.Fatalf("expected item name Combo, got %v", itemPayload["name"])
	}
	if asInt(asMap(itemPayload["price"])["amount"]) != 1590 {
		t.Fatalf("expected item price 1590, got %v", asMap(itemPayload["price"])["amount"])
	}
	groups := asSlice(itemPayload["option_groups"])
	if len(groups) != 1 {
		t.Fatalf("expected one option group, got %d", len(groups))
	}
	group := asMap(groups[0])
	if asString(group["id"]) != "grp-drink" {
		t.Fatalf("expected option group grp-drink, got %v", group["id"])
	}
}

func TestBuildItemPayloadFromMenuPayload(t *testing.T) {
	payload := map[string]any{
		"sections": []any{
			map[string]any{
				"name": "Deals",
				"items": []any{
					map[string]any{
						"id":          "item-1",
						"name":        "Iced Tea",
						"description": "Cold drink",
						"price":       299,
						"options": []any{
							map[string]any{
								"id":   "grp-size",
								"name": "Size",
								"values": []any{
									map[string]any{"id": "val-small", "name": "Small", "price": 0},
									map[string]any{"id": "val-large", "name": "Large", "price": 100},
								},
							},
						},
					},
				},
			},
		},
	}

	itemPayload := buildItemPayloadFromMenuPayload(payload, "venue-1", "item-1")
	if itemPayload == nil {
		t.Fatalf("expected menu payload fallback for item")
	}
	if asString(itemPayload["name"]) != "Iced Tea" {
		t.Fatalf("expected item name Iced Tea, got %v", itemPayload["name"])
	}
	if asInt(asMap(itemPayload["price"])["amount"]) != 299 {
		t.Fatalf("expected item price 299, got %v", asMap(itemPayload["price"])["amount"])
	}
	if asString(asMap(itemPayload["price"])["currency"]) != "EUR" {
		t.Fatalf("expected fallback currency EUR, got %v", asMap(itemPayload["price"])["currency"])
	}
	groups := asSlice(itemPayload["option_groups"])
	if len(groups) != 1 {
		t.Fatalf("expected one option group, got %d", len(groups))
	}
	group := asMap(groups[0])
	if asString(group["id"]) != "grp-size" {
		t.Fatalf("expected option group grp-size, got %v", group["id"])
	}
	values := asSlice(group["values"])
	if len(values) != 2 {
		t.Fatalf("expected two option values, got %d", len(values))
	}
}

func TestNeedsVenueContentFallback(t *testing.T) {
	partialAssortment := map[string]any{
		"loading_strategy": "partial",
	}
	if !needsVenueContentFallback(partialAssortment, "venue-1") {
		t.Fatalf("expected partial assortment to require venue-content fallback")
	}

	regularAssortment := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Combo",
				"price": 1290,
			},
		},
	}
	if needsVenueContentFallback(regularAssortment, "venue-1") {
		t.Fatalf("did not expect full assortment to require venue-content fallback")
	}
}
