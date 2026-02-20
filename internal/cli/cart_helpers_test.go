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
