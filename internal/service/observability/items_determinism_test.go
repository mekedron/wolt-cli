package observability

import (
	"reflect"
	"testing"
)

func TestMenuExtractionIsDeterministicAndPreservesSliceOrder(t *testing.T) {
	payload := map[string]any{
		"z_duplicate_items": []any{
			map[string]any{"id": "item-b", "name": "Item B", "price": 999},
		},
		"a_primary_items": []any{
			map[string]any{"id": "item-b", "name": "Item B", "price": 100},
			map[string]any{"id": "item-a", "name": "Item A", "price": 200},
		},
		"m_more_items": []any{
			map[string]any{"id": "item-c", "name": "Item C", "price": 300},
		},
	}

	wantIDs := []string{"item-b", "item-a", "item-c"}
	baselineItems := ExtractMenuItems(payload, "venue-1", "example-venue")
	assertItemOrderAndFirstPrice(t, baselineItems, wantIDs, 100)

	baselineMenu, warnings := BuildVenueMenu(
		"venue-1",
		[]map[string]any{payload},
		"",
		false,
		nil,
		ItemVenueContext{VenueSlug: "example-venue"},
	)
	if len(warnings) != 0 {
		t.Fatalf("BuildVenueMenu warnings = %v", warnings)
	}
	assertMenuOrderAndFirstPrice(t, baselineMenu, wantIDs, 100)

	for attempt := 0; attempt < 100; attempt++ {
		items := ExtractMenuItems(payload, "venue-1", "example-venue")
		if !reflect.DeepEqual(items, baselineItems) {
			t.Fatalf("ExtractMenuItems attempt %d differs:\n got: %#v\nwant: %#v", attempt, items, baselineItems)
		}
		menu, gotWarnings := BuildVenueMenu(
			"venue-1",
			[]map[string]any{payload},
			"",
			false,
			nil,
			ItemVenueContext{VenueSlug: "example-venue"},
		)
		if !reflect.DeepEqual(gotWarnings, warnings) || !reflect.DeepEqual(menu, baselineMenu) {
			t.Fatalf(
				"BuildVenueMenu attempt %d differs:\n got: %#v warnings=%v\nwant: %#v warnings=%v",
				attempt,
				menu,
				gotWarnings,
				baselineMenu,
				warnings,
			)
		}
	}
}

func assertItemOrderAndFirstPrice(
	t *testing.T,
	items []map[string]any,
	wantIDs []string,
	wantFirstPrice int,
) {
	t.Helper()
	if len(items) != len(wantIDs) {
		t.Fatalf("ExtractMenuItems count = %d, want %d: %#v", len(items), len(wantIDs), items)
	}
	for idx, wantID := range wantIDs {
		if got := stringFromAny(items[idx]["item_id"]); got != wantID {
			t.Fatalf("ExtractMenuItems item %d id = %q, want %q", idx, got, wantID)
		}
	}
	if got := intValue(toMap(items[0]["base_price"])["amount"]); got != wantFirstPrice {
		t.Fatalf("ExtractMenuItems first price = %d, want %d", got, wantFirstPrice)
	}
}

func assertMenuOrderAndFirstPrice(
	t *testing.T,
	menu map[string]any,
	wantIDs []string,
	wantFirstPrice int,
) {
	t.Helper()
	rows, ok := menu["items"].([]map[string]any)
	if !ok {
		t.Fatalf("BuildVenueMenu items type = %T, want []map[string]any", menu["items"])
	}
	if len(rows) != len(wantIDs) {
		t.Fatalf("BuildVenueMenu count = %d, want %d: %#v", len(rows), len(wantIDs), rows)
	}
	for idx, wantID := range wantIDs {
		if got := stringFromAny(rows[idx]["item_id"]); got != wantID {
			t.Fatalf("BuildVenueMenu item %d id = %q, want %q", idx, got, wantID)
		}
	}
	if got := intValue(toMap(rows[0]["price"])["amount"]); got != wantFirstPrice {
		t.Fatalf("BuildVenueMenu first price = %d, want %d", got, wantFirstPrice)
	}
}
