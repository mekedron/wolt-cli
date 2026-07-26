package cli

import (
	"strings"
	"testing"
)

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
	options, err := buildBasketOptions(payload, selected)
	if err != nil {
		t.Fatalf("buildBasketOptions: %v", err)
	}
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

func TestBuildBasketOptionsDistinguishesMissingAndFreePrices(t *testing.T) {
	tests := []struct {
		name      string
		value     map[string]any
		wantPrice int
		wantError bool
	}{
		{
			name:      "missing price fails closed",
			value:     map[string]any{"id": "selected", "name": "Selected"},
			wantError: true,
		},
		{
			name:      "explicit free price remains valid",
			value:     map[string]any{"id": "selected", "name": "Selected", "price": 0},
			wantPrice: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := map[string]any{
				"option_groups": []any{
					map[string]any{
						"id":     "group",
						"values": []any{test.value},
					},
				},
			}
			selected, err := parseOptionSelections([]string{"group=selected"})
			if err != nil {
				t.Fatal(err)
			}
			options, err := buildBasketOptions(payload, selected)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "missing current price metadata") {
					t.Fatalf("buildBasketOptions error = %v, want missing price metadata", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBasketOptions: %v", err)
			}
			value := asMap(asSlice(asMap(options[0])["values"])[0])
			if asInt(value["price"]) != test.wantPrice {
				t.Fatalf("price = %v, want %d", value["price"], test.wantPrice)
			}
		})
	}
}

func TestBuildBasketOptionsValidatesSelectionLimits(t *testing.T) {
	value := map[string]any{"id": "selected", "price": 0}
	tests := []struct {
		name           string
		group          map[string]any
		selections     []string
		wantError      string
		wantCount      int
		wantValueCount int
	}{
		{
			name:      "required group cannot be omitted",
			group:     map[string]any{"id": "group", "required": true, "values": []any{value}},
			wantError: "selection count must be at least 1",
			wantCount: -1,
		},
		{
			name:       "minimum counts repeated selections",
			group:      map[string]any{"id": "group", "min": 2, "values": []any{value}},
			selections: []string{"group=selected"},
			wantError:  "selection count must be at least 2",
			wantCount:  -1,
		},
		{
			name:       "maximum counts repeated selections",
			group:      map[string]any{"id": "group", "max": 1, "values": []any{value}},
			selections: []string{"group=selected:2"},
			wantError:  "selection count must be at most 1",
			wantCount:  -1,
		},
		{
			name:      "optional unselected group is omitted",
			group:     map[string]any{"id": "group", "values": []any{value}},
			wantCount: 0,
		},
		{
			name:           "selection within limits is emitted",
			group:          map[string]any{"id": "group", "min": 3, "max": 3, "values": []any{value}},
			selections:     []string{"group=selected", "group=SELECTED:2"},
			wantCount:      1,
			wantValueCount: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selected, err := parseOptionSelections(test.selections)
			if err != nil {
				t.Fatal(err)
			}
			options, err := buildBasketOptions(
				map[string]any{"option_groups": []any{test.group}},
				selected,
			)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("buildBasketOptions error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildBasketOptions: %v", err)
			}
			if len(options) != test.wantCount {
				t.Fatalf("option count = %d, want %d", len(options), test.wantCount)
			}
			if test.wantValueCount > 0 {
				values := asSlice(asMap(options[0])["values"])
				if len(values) != 1 || asInt(asMap(values[0])["count"]) != test.wantValueCount {
					t.Fatalf("values = %#v, want one canonical value with count %d", values, test.wantValueCount)
				}
			}
		})
	}
}

func TestBuildBasketOptionsRejectsUnknownAndAmbiguousNames(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			map[string]any{
				"id":   "primary-drink",
				"name": "Drink",
				"values": []any{
					map[string]any{"id": "still-water", "name": "Water", "price": 0},
					map[string]any{"id": "sparkling-water", "name": "Water", "price": 50},
				},
			},
			map[string]any{
				"id":   "secondary-drink",
				"name": "Drink",
				"values": []any{
					map[string]any{"id": "juice", "name": "Juice", "price": 100},
				},
			},
		},
	}

	tests := []struct {
		name      string
		selection string
		want      string
	}{
		{name: "unknown group", selection: "Missing=Water", want: "unknown option group"},
		{name: "ambiguous group name", selection: "Drink=Juice", want: "ambiguous option group"},
		{name: "unknown value", selection: "primary-drink=Missing", want: "unknown option value"},
		{name: "ambiguous value name", selection: "primary-drink=Water", want: "ambiguous option value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selected, err := parseOptionSelections([]string{test.selection})
			if err != nil {
				t.Fatalf("parseOptionSelections: %v", err)
			}
			for range 50 {
				if _, err := buildBasketOptions(payload, selected); err == nil ||
					!strings.Contains(err.Error(), test.want) {
					t.Fatalf("buildBasketOptions error = %v, want %q", err, test.want)
				}
			}
		})
	}

	selected, err := parseOptionSelections([]string{"primary-drink=still-water"})
	if err != nil {
		t.Fatal(err)
	}
	options, err := buildBasketOptions(payload, selected)
	if err != nil || asString(asMap(options[0])["id"]) != "primary-drink" {
		t.Fatalf("exact IDs must remain unambiguous: options=%#v err=%v", options, err)
	}
}

func TestBuildBasketOptionsValidatesMixedHydratedAndIDOnlyGroups(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			map[string]any{
				"id":   "size",
				"name": "Size",
				"values": []any{
					map[string]any{"id": "large", "name": "Large", "price": 100},
				},
			},
			map[string]any{"id": "sauce"},
		},
	}

	unknownGroup, err := parseOptionSelections([]string{"typo=large"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildBasketOptions(payload, unknownGroup); err == nil ||
		!strings.Contains(err.Error(), "unknown option group") {
		t.Fatalf("unknown mixed group error = %v", err)
	}

	unknownValue, err := parseOptionSelections([]string{"size=typo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildBasketOptions(payload, unknownValue); err == nil ||
		!strings.Contains(err.Error(), "unknown option value") {
		t.Fatalf("unknown hydrated value error = %v", err)
	}

	idOnlyValue, err := parseOptionSelections([]string{"sauce=custom"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildBasketOptions(payload, idOnlyValue); err != nil {
		t.Fatalf("ID-only group must retain value-ID pass-through: %v", err)
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

func TestBuildItemPayloadFromAssortmentMatchesUppercaseID(t *testing.T) {
	const itemID = "abcdefabcdefabcdefabcdef"
	payload := buildItemPayloadFromAssortment(
		map[string]any{
			"items": []any{
				map[string]any{
					"id":    strings.ToUpper(itemID),
					"name":  "Uppercase item",
					"price": 250,
				},
			},
		},
		itemID,
	)
	if payload == nil || payload["name"] != "Uppercase item" {
		t.Fatalf("uppercase item payload = %#v", payload)
	}
}

func TestBuildItemPayloadFromMenuPayload(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{"currency": "GEL"},
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
	if asString(asMap(itemPayload["price"])["currency"]) != "GEL" {
		t.Fatalf("expected venue currency GEL, got %v", asMap(itemPayload["price"])["currency"])
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

	implicitPartialAssortment := map[string]any{
		"categories": []any{
			map[string]any{
				"id":       "cat-fresh",
				"slug":     "fresh-produce",
				"item_ids": []any{"item-1", "item-2"},
			},
		},
		"items": []any{
			map[string]any{"id": "item-1", "name": "Preview item", "price": 500},
		},
	}
	if !needsVenueContentFallback(implicitPartialAssortment, "venue-1") {
		t.Fatalf("expected an unlabelled root with missing referenced items to require fallback")
	}

	regularAssortment := map[string]any{
		"categories": []any{
			map[string]any{
				"id":       "cat-fresh",
				"slug":     "fresh-produce",
				"item_ids": []any{"item-1"},
			},
		},
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
