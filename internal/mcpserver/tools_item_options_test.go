package mcpserver

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

const (
	optionsVenueID = "000000000000000000000651"
	optionsItemID  = "000000000000000000000652"
)

// optionsAssortmentItems mirrors POST /assortment/items for a pizza: the item
// lists item-level groups with multi_choice_config, and the root "options"
// library holds the values, their prices and default_value.
func optionsAssortmentItems() map[string]any {
	return map[string]any{
		"items": []any{
			map[string]any{
				"id":                  optionsItemID,
				"name":                "Pizza",
				"price":               1200,
				"purchasable_balance": nil,
				"options": []any{
					map[string]any{
						"id":                  "grp-base",
						"option_id":           "lib-base",
						"name":                "Base",
						"prerequisite_values": []any{},
						"multi_choice_config": map[string]any{
							"total_range":           map[string]any{"min": 1, "max": 1},
							"max_single_selections": 1,
						},
					},
					map[string]any{
						"id":                  "grp-toppings",
						"option_id":           "lib-toppings",
						"name":                "Toppings",
						"prerequisite_values": []any{},
						"multi_choice_config": map[string]any{
							"total_range":           map[string]any{"min": 0, "max": 3},
							"max_single_selections": 1,
						},
					},
				},
			},
		},
		"options": []any{
			map[string]any{
				"id":            "lib-base",
				"name":          "Base (+2,00€)",
				"type":          "choice",
				"default_value": "val-wheat",
				"values": []any{
					map[string]any{"id": "val-wheat", "name": "Wheat", "price": 200},
					map[string]any{"id": "val-gluten-free", "name": "Gluten free", "price": 400},
				},
			},
			map[string]any{
				"id":            "lib-toppings",
				"name":          "Toppings",
				"type":          "multi_choice",
				"default_value": "val-olives",
				"values": []any{
					map[string]any{"id": "val-olives", "name": "Olives", "price": 100},
					map[string]any{"id": "val-ham", "name": "Ham", "price": 150},
				},
			},
		},
	}
}

func optionsStubWolt(captured *map[string]any) *stubWolt {
	return &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": optionsVenueID, "slug": "pizza-place", "currency": "EUR"},
			}, nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return optionsAssortmentItems(), nil
		},
		venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
			return map[string]any{"sections": []any{}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{}}, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			if captured != nil {
				*captured = payload
			}
			return map[string]any{"id": "basket-options"}, nil
		},
	}
}

// Issue #65: wolt_venue_item returned every group with no values, min 0,
// max 0 and required false.
func TestVenueItemJoinsTheOptionLibrary(t *testing.T) {
	tc := regressionMCPToolCtx(optionsStubWolt(nil))
	_, out, err := tc.handleVenueItem(context.Background(), nil, VenueItemInput{
		Venue:  "pizza-place",
		ItemID: optionsItemID,
	})
	if err != nil {
		t.Fatalf("handleVenueItem: %v", err)
	}
	groups := map[string]map[string]any{}
	for _, raw := range asSlice(out.Item["option_groups"]) {
		group := asMap(raw)
		groups[asString(group["group_id"])] = group
	}
	base := groups["grp-base"]
	if base == nil || base["name"] != "Base" || base["required"] != true ||
		base["min"] != 1 || base["max"] != 1 || base["library_group_id"] != "lib-base" {
		t.Fatalf("base group = %#v", base)
	}
	values := asSlice(base["values"])
	if len(values) != 2 {
		t.Fatalf("base values = %#v, want the two library values", values)
	}
	wheat := asMap(values[1])
	if wheat["value_id"] != "val-wheat" || wheat["price"] != 200 || wheat["default"] != true {
		t.Fatalf("wheat value = %#v, want price 200 and default", wheat)
	}
	toppings := groups["grp-toppings"]
	if toppings == nil || toppings["required"] != false || toppings["max"] != 3 || len(asSlice(toppings["values"])) != 2 {
		t.Fatalf("toppings group = %#v", toppings)
	}
	defaultPrice := asMap(out.Item["default_price"])
	if payloadutil.Int(defaultPrice["amount"]) != 1400 {
		t.Fatalf("default_price = %#v, want 1200 base + 200 required default", defaultPrice)
	}
}

// Issue #66: wolt_cart_add takes option selections, validates them and
// prices the line with them.
func TestCartAddSendsChosenAndDefaultOptions(t *testing.T) {
	var captured map[string]any
	tc := regressionMCPToolCtx(optionsStubWolt(&captured))
	_, out, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		Venue:  "pizza-place",
		ItemID: optionsItemID,
		Count:  2,
		Options: []CartOptionInput{
			{GroupID: "grp-toppings", ValueID: "Ham"},
			{GroupID: "Toppings", ValueID: "val-olives", Count: 1},
		},
	})
	if err != nil {
		t.Fatalf("handleCartAdd: %v", err)
	}
	line := asMap(asSlice(captured["items"])[0])
	// (1200 base + 200 default wheat + 150 ham + 100 olives) x 2
	if line["count"] != 2 || payloadutil.Int(line["price"]) != 3300 {
		t.Fatalf("line = %#v, want count 2 and whole-line price 3300", line)
	}
	got := map[string]map[string]int{}
	for _, raw := range asSlice(line["options"]) {
		group := asMap(raw)
		values := map[string]int{}
		for _, rawValue := range asSlice(group["values"]) {
			value := asMap(rawValue)
			values[asString(value["id"])] = payloadutil.Int(value["count"])
		}
		got[asString(group["id"])] = values
	}
	want := map[string]map[string]int{
		"grp-base":     {"val-wheat": 1},
		"grp-toppings": {"val-ham": 1, "val-olives": 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("basket options = %v, want %v", got, want)
	}
	if len(out.DefaultOptions) != 1 || asMap(out.DefaultOptions[0])["value_id"] != "val-wheat" {
		t.Fatalf("default options = %#v, want the wheat base", out.DefaultOptions)
	}
	if !strings.Contains(out.Summary, "default options used for Base: Wheat") {
		t.Fatalf("summary = %q, want the applied default named", out.Summary)
	}
}

func TestCartAddRejectsInvalidOptionChoices(t *testing.T) {
	tests := []struct {
		name    string
		options []CartOptionInput
		want    string
	}{
		{
			name:    "unknown value",
			options: []CartOptionInput{{GroupID: "grp-base", ValueID: "Rye"}},
			want:    `unknown option value "Rye"`,
		},
		{
			name: "two values in a single-choice group",
			options: []CartOptionInput{
				{GroupID: "grp-base", ValueID: "val-wheat"},
				{GroupID: "grp-base", ValueID: "val-gluten-free"},
			},
			want: "selection count must be at most 1; got 2",
		},
		{
			name:    "missing value id",
			options: []CartOptionInput{{GroupID: "grp-base"}},
			want:    "every option needs group_id and value_id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured map[string]any
			tc := regressionMCPToolCtx(optionsStubWolt(&captured))
			_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
				Venue:   "pizza-place",
				ItemID:  optionsItemID,
				Options: test.options,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "NOT added") {
				t.Fatalf("handleCartAdd error = %v, want %q", err, test.want)
			}
			if captured != nil {
				t.Fatalf("AddToBasket was called with %#v", captured)
			}
		})
	}
}
