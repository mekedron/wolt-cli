package payloadutil

import (
	"reflect"
	"testing"
)

func TestMap(t *testing.T) {
	if got := Map(nil); got != nil {
		t.Errorf("Map(nil) = %v, want nil", got)
	}
	if got := Map("not a map"); got != nil {
		t.Errorf("Map(string) = %v, want nil", got)
	}
	in := map[string]any{"a": 1}
	if got := Map(in); !reflect.DeepEqual(got, in) {
		t.Errorf("Map(map[string]any) = %v, want %v", got, in)
	}
	// map[string]string must be widened to map[string]any.
	if got := Map(map[string]string{"k": "v"}); !reflect.DeepEqual(got, map[string]any{"k": "v"}) {
		t.Errorf("Map(map[string]string) = %v, want widened map", got)
	}
}

func TestSlice(t *testing.T) {
	if got := Slice(nil); got != nil {
		t.Errorf("Slice(nil) = %v, want nil", got)
	}
	if got := Slice([]any{1, "two"}); !reflect.DeepEqual(got, []any{1, "two"}) {
		t.Errorf("Slice([]any) = %v", got)
	}
	// Typed slices are reflected into []any.
	if got := Slice([]string{"a", "b"}); !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Errorf("Slice([]string) = %v, want []any{a,b}", got)
	}
	// Byte slices are deliberately NOT treated as element slices.
	if got := Slice([]byte("hi")); got != nil {
		t.Errorf("Slice([]byte) = %v, want nil", got)
	}
	if got := Slice(42); got != nil {
		t.Errorf("Slice(int) = %v, want nil", got)
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{42, "42"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBool(t *testing.T) {
	if !Bool(true) {
		t.Error("Bool(true) = false")
	}
	if Bool("true") {
		t.Error(`Bool("true") = true, want false (only real bools count)`)
	}
	if Bool(nil) {
		t.Error("Bool(nil) = true")
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{int(7), 7},
		{int64(8), 8},
		{float64(9.9), 9}, // JSON numbers decode to float64; truncation is intentional.
		{float32(10.5), 10},
		{"11", 0}, // strings are not coerced.
		{nil, 0},
	}
	for _, c := range cases {
		if got := Int(c.in); got != c.want {
			t.Errorf("Int(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCoalesceAny(t *testing.T) {
	if got := CoalesceAny(nil, "", "first", "second"); got != "first" {
		t.Errorf("CoalesceAny skipped nil/blank wrong: got %v, want first", got)
	}
	if got := CoalesceAny(nil, "   ", 0); got != 0 {
		t.Errorf("CoalesceAny should return first non-blank (0), got %v", got)
	}
	if got := CoalesceAny(nil, ""); got != nil {
		t.Errorf("CoalesceAny(all blank) = %v, want nil", got)
	}
	// A non-empty string wins over a later value.
	if got := CoalesceAny("x", "y"); got != "x" {
		t.Errorf("CoalesceAny = %v, want x", got)
	}
}

func TestExtractBasketVenueIdentitySupportsNestedAndTopLevelShapes(t *testing.T) {
	tests := []struct {
		name   string
		basket map[string]any
		want   BasketVenueIdentity
	}{
		{
			name: "nested",
			basket: map[string]any{
				"venue": map[string]any{"id": "venue-1", "slug": "venue-one"},
			},
			want: BasketVenueIdentity{ID: "venue-1", Slug: "venue-one"},
		},
		{
			name: "top-level",
			basket: map[string]any{
				"venue_id": "venue-2", "venue_slug": "venue-two",
			},
			want: BasketVenueIdentity{ID: "venue-2", Slug: "venue-two"},
		},
		{
			name: "canonical top-level id beats malformed nested id",
			basket: map[string]any{
				"venue":    map[string]any{"id": "synthetic-slug"},
				"venue_id": "000000000000000000000601",
			},
			want: BasketVenueIdentity{ID: "000000000000000000000601"},
		},
		{
			name: "conflicting canonical ids fail closed",
			basket: map[string]any{
				"venue":    map[string]any{"id": "000000000000000000000601"},
				"venue_id": "000000000000000000000602",
			},
			want: BasketVenueIdentity{Conflict: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ExtractBasketVenueIdentity(test.basket); got != test.want {
				t.Fatalf("ExtractBasketVenueIdentity() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestBasketHelpersSupportCompatibilityShapesAndSkipBlankIDs(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{"id": "basket-1"},
		},
		"results": []any{
			map[string]any{"basket_id": " BASKET-1 "},
			map[string]any{"id": "basket-2"},
			map[string]any{"id": " "},
			map[string]any{"basket_id": "basket-1"},
		},
	}
	rows := BasketRows(page)
	if len(rows) != 3 || BasketID(rows[0]) != "basket-1" || BasketID(rows[1]) != "basket-2" ||
		BasketID(rows[2]) != "" {
		t.Fatalf("BasketRows/BasketID = %#v", rows)
	}
	ids := BasketIDs(page)
	if len(ids) != 2 || ids[0] != "basket-1" || ids[1] != "basket-2" {
		t.Fatalf("BasketIDs = %#v, want unique usable compatibility ids", ids)
	}
	if BasketIDsComplete(page) {
		t.Fatal("BasketIDsComplete = true with a blank basket id")
	}
	delete(page, "baskets")
	page["results"] = []any{
		map[string]any{"basket_id": "basket-1"},
		map[string]any{"basket_id": "basket-1"},
	}
	if !BasketIDsComplete(page) || len(BasketIDs(page)) != 1 {
		t.Fatalf("duplicate usable basket IDs should remain safe: %#v", page)
	}
}

func TestBasketReplacementHelpersPreserveUnrelatedLines(t *testing.T) {
	basket := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-a",
				"count": 2,
				"name":  "A",
				"price": 500,
				"options": []any{
					map[string]any{
						"id": "size",
						"values": []any{
							map[string]any{"id": "large", "count": 1, "price": 100},
						},
					},
				},
				"substitution_settings": map[string]any{"is_allowed": true},
			},
			map[string]any{"id": "item-b", "count": 3, "name": "B", "price": 700},
		},
	}

	replacement, err := BuildBasketUpsertItem(
		map[string]any{
			"id":    "item-a",
			"name":  "A refreshed",
			"price": map[string]any{"amount": 550},
			"options": []any{
				map[string]any{
					"id": "size",
					"values": []any{
						map[string]any{"id": "large", "count": 1, "price": map[string]any{"amount": 125}},
					},
				},
			},
			"substitution_settings": map[string]any{"is_allowed": true},
		},
		1,
	)
	if err != nil {
		t.Fatalf("BuildBasketUpsertItem() error = %v", err)
	}
	merged, err := MergeBasketItems(
		basket,
		"item-a",
		1,
		replacement,
	)
	if err != nil {
		t.Fatalf("MergeBasketItems() error = %v", err)
	}
	if len(merged) != 2 || Int(Map(merged[0])["count"]) != 3 || String(Map(merged[1])["id"]) != "item-b" {
		t.Fatalf("MergeBasketItems() lost or miscounted lines: %#v", merged)
	}
	option := Map(Slice(Map(merged[0])["options"])[0])
	value := Map(Slice(option["values"])[0])
	if String(Map(merged[0])["name"]) != "A refreshed" ||
		Int(Map(merged[0])["price"]) != 550 ||
		String(value["id"]) != "large" ||
		Int(value["price"]) != 125 ||
		!Bool(Map(Map(merged[0])["substitution_settings"])["is_allowed"]) {
		t.Fatalf("MergeBasketItems() did not preserve refreshed line metadata: %#v", merged[0])
	}

	remaining, removed, err := RemoveBasketItems(basket, "item-a", 1)
	if err != nil {
		t.Fatalf("RemoveBasketItems(partial) error = %v", err)
	}
	if removed != 1 || len(remaining) != 2 || Int(Map(remaining[0])["count"]) != 1 ||
		String(Map(remaining[1])["id"]) != "item-b" {
		t.Fatalf("RemoveBasketItems(partial) = (%#v, %d)", remaining, removed)
	}

	remaining, removed, err = RemoveBasketItems(basket, "item-a", 0)
	if err != nil {
		t.Fatalf("RemoveBasketItems(all) error = %v", err)
	}
	if removed != 2 || len(remaining) != 1 || String(Map(remaining[0])["id"]) != "item-b" {
		t.Fatalf("RemoveBasketItems(all) = (%#v, %d)", remaining, removed)
	}
}

func TestMergeBasketItemsKeepsDifferentConfigurationsSeparate(t *testing.T) {
	option := func(valueID string) []any {
		return []any{
			map[string]any{
				"id": "size",
				"values": []any{
					map[string]any{"id": valueID, "count": 1, "price": 0},
				},
			},
		}
	}
	basket := map[string]any{
		"items": []any{
			map[string]any{"id": "item-a", "count": 1, "price": 500, "options": option("small")},
		},
	}

	same, err := MergeBasketItems(
		basket,
		"item-a",
		1,
		map[string]any{"id": "item-a", "count": 1, "price": 500, "options": option("small")},
	)
	if err != nil {
		t.Fatalf("MergeBasketItems(same) error = %v", err)
	}
	if len(same) != 1 || Int(Map(same[0])["count"]) != 2 {
		t.Fatalf("same configuration was not merged: %#v", same)
	}

	repricedBasket := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-a",
				"count": 1,
				"price": 500,
				"options": []any{
					map[string]any{
						"id": "size",
						"values": []any{
							map[string]any{"id": "small", "count": 1, "price": 100},
						},
					},
				},
			},
		},
	}
	repriced, err := MergeBasketItems(
		repricedBasket,
		"item-a",
		1,
		map[string]any{
			"id":    "item-a",
			"count": 1,
			"price": 500,
			"options": []any{
				map[string]any{
					"id": "size",
					"values": []any{
						map[string]any{"id": "small", "count": 1, "price": 150},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("MergeBasketItems(repriced) error = %v", err)
	}
	if len(repriced) != 1 || Int(Map(repriced[0])["count"]) != 2 {
		t.Fatalf("same option selection with a refreshed price was not merged: %#v", repriced)
	}
	repricedOption := Map(Slice(Map(repriced[0])["options"])[0])
	repricedValue := Map(Slice(repricedOption["values"])[0])
	if Int(repricedValue["price"]) != 150 {
		t.Fatalf("refreshed option price was not forwarded: %#v", repriced[0])
	}

	different, err := MergeBasketItems(
		basket,
		"item-a",
		1,
		map[string]any{"id": "item-a", "count": 1, "price": 500, "options": option("large")},
	)
	if err != nil {
		t.Fatalf("MergeBasketItems(different) error = %v", err)
	}
	if len(different) != 2 || Int(Map(different[0])["count"]) != 1 ||
		String(Map(Slice(Map(Slice(Map(different[1])["options"])[0])["values"])[0])["id"]) != "large" {
		t.Fatalf("different configuration was merged or lost: %#v", different)
	}

	maxInt := int(^uint(0) >> 1)
	if _, err := MergeBasketItems(
		map[string]any{"items": []any{map[string]any{"id": "item-a", "count": maxInt, "price": 500}}},
		"item-a",
		1,
		map[string]any{"id": "item-a", "count": 1, "price": 500},
	); err == nil {
		t.Fatal("MergeBasketItems() accepted an overflowing count")
	}
}

func TestRemoveBasketItemsAppliesCountAcrossMatchingConfigurations(t *testing.T) {
	basket := map[string]any{
		"items": []any{
			map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{map[string]any{"id": "small", "price": 0}}}}},
			map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{map[string]any{"id": "large", "price": 0}}}}},
		},
	}

	remaining, removed, err := RemoveBasketItems(basket, "item-a", 1)
	if err != nil {
		t.Fatalf("RemoveBasketItems() error = %v", err)
	}
	if removed != 1 || len(remaining) != 1 {
		t.Fatalf("RemoveBasketItems() = (%#v, %d), want one configuration remaining", remaining, removed)
	}
	value := Map(Slice(Map(Slice(Map(remaining[0])["options"])[0])["values"])[0])
	if String(value["id"]) != "large" {
		t.Fatalf("wrong configuration remained: %#v", remaining)
	}
}

func TestRemoveBasketItemsRejectsOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	basket := map[string]any{
		"items": []any{
			map[string]any{"id": "item-a", "count": maxInt, "price": 500},
			map[string]any{"id": "item-a", "count": 1, "price": 500},
		},
	}

	remaining, removed, err := RemoveBasketItems(basket, "item-a", 0)
	if err == nil {
		t.Fatal("RemoveBasketItems() accepted an overflowing removed count")
	}
	if remaining != nil || removed != 0 {
		t.Fatalf("RemoveBasketItems() returned partial state after overflow: (%#v, %d)", remaining, removed)
	}
}

func TestBasketRowsForMutationRejectsAmbiguousPageShapes(t *testing.T) {
	tests := []struct {
		name string
		page map[string]any
	}{
		{name: "missing container", page: map[string]any{}},
		{name: "nil container", page: map[string]any{"baskets": nil}},
		{name: "wrong container", page: map[string]any{"baskets": map[string]any{}}},
		{name: "non-object basket", page: map[string]any{"baskets": []any{"invalid"}}},
		{
			name: "conflicting venue ids",
			page: map[string]any{"baskets": []any{
				map[string]any{
					"venue_id": "000000000000000000000001",
					"venue":    map[string]any{"id": "000000000000000000000002"},
				},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if rows, err := BasketRowsForMutation(test.page); err == nil || rows != nil {
				t.Fatalf("BasketRowsForMutation() = (%#v, %v), want fail-closed error", rows, err)
			}
			if BasketIDsComplete(test.page) {
				t.Fatal("BasketIDsComplete() accepted an ambiguous page")
			}
		})
	}
}

func TestBasketReplacementHelpersRejectIncompleteState(t *testing.T) {
	validLine := func() map[string]any {
		return map[string]any{
			"id":    "item-a",
			"count": 1,
			"price": 500,
			"options": []any{
				map[string]any{
					"id": "size",
					"values": []any{
						map[string]any{"id": "large", "count": 1, "price": 0},
					},
				},
			},
			"substitution_settings": map[string]any{"is_allowed": false},
		}
	}
	withLine := func(line any) map[string]any {
		return map[string]any{"items": []any{line}}
	}
	tests := []struct {
		name   string
		basket map[string]any
	}{
		{name: "missing items", basket: map[string]any{}},
		{name: "wrong items container", basket: map[string]any{"items": "invalid"}},
		{name: "non-object line", basket: withLine("invalid")},
		{name: "blank line id", basket: withLine(map[string]any{"id": " ", "count": 1, "price": 500})},
		{name: "missing line price", basket: withLine(map[string]any{"id": "item-a", "count": 1})},
		{name: "nonpositive line count", basket: withLine(map[string]any{"id": "item-a", "count": 0, "price": 500})},
		{name: "wrong options container", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": "invalid"})},
		{name: "non-object option", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{"invalid"}})},
		{name: "blank option id", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": " ", "values": []any{}}}})},
		{name: "missing values", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size"}}})},
		{name: "wrong values container", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": "invalid"}}})},
		{name: "non-object value", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{"invalid"}}}})},
		{name: "blank value id", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{map[string]any{"id": " ", "price": 0}}}}})},
		{name: "missing value price", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{map[string]any{"id": "large"}}}}})},
		{name: "negative value price", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "options": []any{map[string]any{"id": "size", "values": []any{map[string]any{"id": "large", "price": -1}}}}})},
		{name: "wrong substitution settings", basket: withLine(map[string]any{"id": "item-a", "count": 1, "price": 500, "substitution_settings": "invalid"})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merged, mergeErr := MergeBasketItems(test.basket, "item-a", 1, validLine())
			if mergeErr == nil || merged != nil {
				t.Fatalf("MergeBasketItems() = (%#v, %v), want no partial replacement", merged, mergeErr)
			}
			remaining, removed, removeErr := RemoveBasketItems(test.basket, "item-a", 1)
			if removeErr == nil || remaining != nil || removed != 0 {
				t.Fatalf(
					"RemoveBasketItems() = (%#v, %d, %v), want no partial replacement",
					remaining,
					removed,
					removeErr,
				)
			}
		})
	}
}

func TestInferCurrency(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"€12.50", "EUR"},
		{"$9.99", "USD"},
		{"PLN 20", "PLN"},
		{"  €5 ", "EUR"},
		{"GEL0.00", "GEL"},
		{"GEL 16.45", "GEL"},
		{"₾16.45", "GEL"},
		{"SEK 149.00", "SEK"},
		{"149,00 DKK", "DKK"},
		{"NOK129.00", "NOK"},
		{"the total", ""},
		{"12.50", ""}, // no recognizable symbol
		{"", ""},
	}
	for _, c := range cases {
		if got := InferCurrency(c.in); got != c.want {
			t.Errorf("InferCurrency(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeCurrencyKeepsUnlistedMarkets guards the regression where an
// allowlist gated explicitly stated currencies: callers fail closed on an empty
// result, so dropping a valid code takes a whole market offline. Verified
// against production venue payloads — Albania reports "ALL", North Macedonia
// reports "MKD".
func TestNormalizeCurrencyKeepsUnlistedMarkets(t *testing.T) {
	for _, code := range []string{"ALL", "MKD", "AMD", "KGS", "UZS", "GEL", "EUR"} {
		if got := NormalizeCurrency(code); got != code {
			t.Errorf("NormalizeCurrency(%q) = %q, want %q", code, got, code)
		}
	}
	if got := NormalizeCurrency("  gel "); got != "GEL" {
		t.Errorf("NormalizeCurrency trims and uppercases: got %q", got)
	}
	for _, invalid := range []string{"", "EU", "EUROS", "E1R", "12.50", "€"} {
		if got := NormalizeCurrency(invalid); got != "" {
			t.Errorf("NormalizeCurrency(%q) = %q, want empty", invalid, got)
		}
	}
}

func TestCurrencyFromPayloadsAcceptsUnlistedCodes(t *testing.T) {
	venue := map[string]any{"venue": map[string]any{"currency": "ALL"}}
	if got := CurrencyFromVenuePayload(venue); got != "ALL" {
		t.Errorf("CurrencyFromVenuePayload = %q, want ALL", got)
	}
	basket := map[string]any{"currency": "MKD"}
	if got := CurrencyFromBasket(basket); got != "MKD" {
		t.Errorf("CurrencyFromBasket = %q, want MKD", got)
	}
}

func TestExtractOptionSpecs(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			map[string]any{
				"id":       "group-1",
				"name":     "Size",
				"required": true,
				"min":      1,
				"max":      1,
				"values": []any{
					map[string]any{"id": "val-a", "name": "Small", "price": 0},
					map[string]any{"id": "val-b", "name": "Large", "price": 150},
				},
			},
		},
	}

	specs := ExtractOptionSpecs(payload)
	group, ok := specs["group-1"]
	if !ok {
		t.Fatalf("expected group-1 in specs, got keys %v", keys(specs))
	}
	if group.Name != "Size" || !group.Required || group.MinSelect != 1 || group.MaxSelect != 1 {
		t.Errorf("group metadata mismatch: %+v", group)
	}
	if len(group.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(group.Values))
	}
	if group.Values["val-b"].Price != 150 || group.Values["val-b"].Name != "Large" {
		t.Errorf("value val-b mismatch: %+v", group.Values["val-b"])
	}
}

func TestExtractOptionSpecsNestedPriceObject(t *testing.T) {
	for _, groupKey := range []string{"group_id", "option_id"} {
		t.Run(groupKey, func(t *testing.T) {
			// Wolt uses both group aliases and sometimes nests the price.
			payload := map[string]any{
				"options": []any{
					map[string]any{
						groupKey: "g",
						"items": []any{
							map[string]any{"value_id": "v", "title": "Cheese", "price": map[string]any{"amount": 250}},
						},
					},
				},
			}
			specs := ExtractOptionSpecs(payload)
			if got := specs["g"].Values["v"].Price; got != 250 {
				t.Errorf("nested price.amount not resolved: got %d, want 250", got)
			}
			if got := specs["g"].Values["v"].Name; got != "Cheese" {
				t.Errorf("title fallback not resolved: got %q, want Cheese", got)
			}
		})
	}
}

func keys(m map[string]OptionGroupSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
