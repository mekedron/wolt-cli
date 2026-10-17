package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// The fixtures below follow the shapes Wolt returns for a restaurant whose
// dishes use the shared option library:
//
//   - the venue assortment lists each dish's option groups by an item-level
//     "id" plus the library "option_id", and prices the values only on the
//     library group;
//   - a basket response lists the selected values by id and count, with no
//     price, under the item-level group id.
const (
	optionsVenueID   = "64ddbc0321fe4f0e00ad9d95"
	optionsVenueSlug = "options-venue"
)

func optionsVenueAssortment() map[string]any {
	return map[string]any{
		"items": []any{
			map[string]any{
				"id":                  "item-pizza",
				"name":                "Super Pizza",
				"price":               1490,
				"purchasable_balance": 10,
				"options": []any{
					map[string]any{"id": "item-grp-extras", "option_id": "lib-grp-extras", "name": "Extras"},
				},
			},
			map[string]any{
				"id":                  "item-kebab",
				"name":                "Kebab",
				"price":               1350,
				"purchasable_balance": 10,
				"options": []any{
					map[string]any{"id": "item-grp-dip", "option_id": "lib-grp-dip", "name": "Dip"},
				},
			},
		},
		"options": []any{
			map[string]any{
				"id":   "lib-grp-extras",
				"name": "Extras",
				"values": []any{
					map[string]any{"id": "val-cheese", "name": "Double cheese", "price": 400},
					map[string]any{"id": "val-feta", "name": "Feta", "price": 400},
				},
			},
			map[string]any{
				"id":   "lib-grp-dip",
				"name": "Dip",
				"values": []any{
					map[string]any{"id": "val-none", "name": "No dip", "price": 0},
					map[string]any{"id": "val-garlic", "name": "Garlic dip", "price": 120},
				},
			},
		},
	}
}

func unpricedOptionLine(itemID string, name string, count int, price int, groupID string, valueID string) map[string]any {
	return map[string]any{
		"id":    itemID,
		"name":  name,
		"count": count,
		"price": price,
		"options": []any{
			map[string]any{
				"id":     groupID,
				"values": []any{map[string]any{"id": valueID, "count": 1}},
			},
		},
		"substitution_settings": map[string]any{"is_allowed": false},
	}
}

func optionsVenueBasketsPage(lines ...any) map[string]any {
	return map[string]any{
		"baskets": []any{
			map[string]any{
				"id":    "basket-1",
				"total": "€14.70",
				"venue": map[string]any{
					"id":       optionsVenueID,
					"slug":     optionsVenueSlug,
					"country":  "FIN",
					"currency": "EUR",
				},
				"items": lines,
			},
		},
	}
}

func optionsCLIDeps(wolt *mockWolt) cli.Dependencies {
	return cli.Dependencies{
		Wolt:     wolt,
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
}

// selectedOptionValue returns the single selected value of the line's single
// option group, failing the test on any other shape.
func selectedOptionValue(t *testing.T, line map[string]any) (string, map[string]any) {
	t.Helper()
	options := asSlicePayload(t, line["options"])
	if len(options) != 1 {
		t.Fatalf("expected one option group on line %v, got %#v", line["id"], options)
	}
	group := asMapPayload(t, options[0])
	values := asSlicePayload(t, group["values"])
	if len(values) != 1 {
		t.Fatalf("expected one selected value on line %v, got %#v", line["id"], values)
	}
	return asStringPayload(group["id"]), asMapPayload(t, values[0])
}

// Covers issues #50 and #51 end to end through `wolt cart add`: the new line
// carries the item-level option group id the Wolt app recognises, its price
// includes the paid option value, and the basket's existing line with options
// (unpriced in the basket response) is sent back with its catalog price.
func TestCartAddWithLibraryOptionsSendsItemLevelGroupAndPricedLines(t *testing.T) {
	for _, option := range []string{
		"lib-grp-extras=val-feta",
		"item-grp-extras=val-feta",
		"Extras=Feta",
	} {
		t.Run(option, func(t *testing.T) {
			seenAddPayload := map[string]any{}
			deps := optionsCLIDeps(&mockWolt{
				venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
					return map[string]any{"venue": map[string]any{"id": optionsVenueID, "slug": optionsVenueSlug}}, nil
				},
				venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
					return nil, errors.New("item endpoint unavailable")
				},
				assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
					return optionsVenueAssortment(), nil
				},
				assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
					return optionsVenueAssortment(), nil
				},
				basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
					return optionsVenueBasketsPage(
						unpricedOptionLine("item-kebab", "Kebab", 1, 1470, "item-grp-dip", "val-garlic"),
					), nil
				},
				addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
					seenAddPayload = payload
					return map[string]any{"id": "basket-1", "venue_id": optionsVenueID}, nil
				},
				basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{"count": 1}, nil
				},
			})

			exitCode, out := runCLIWithDeps(t, deps,
				"cart", "add", optionsVenueSlug, "item-pizza",
				"--option", option,
				"--wtoken", "token", "--format", "json",
			)
			if exitCode != 0 {
				t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
			}
			items := asSlicePayload(t, seenAddPayload["items"])
			if len(items) != 2 {
				t.Fatalf("expected existing + new line, got %#v", items)
			}

			existing := asMapPayload(t, items[0])
			groupID, value := selectedOptionValue(t, existing)
			if existing["id"] != "item-kebab" || groupID != "item-grp-dip" ||
				value["id"] != "val-garlic" || asIntPayload(value["price"]) != 120 {
				t.Fatalf("existing line must keep its selection and gain the catalog price 120, got %#v", existing)
			}
			if asIntPayload(existing["price"]) != 1470 {
				t.Fatalf("existing line price must stay as Wolt stored it, got %v", existing["price"])
			}

			added := asMapPayload(t, items[1])
			groupID, value = selectedOptionValue(t, added)
			if groupID != "item-grp-extras" {
				t.Fatalf("new line option group = %q, want the item-level id item-grp-extras", groupID)
			}
			if value["id"] != "val-feta" || asIntPayload(value["price"]) != 400 || asIntPayload(value["count"]) != 1 {
				t.Fatalf("unexpected selected value %#v", value)
			}
			if asIntPayload(added["price"]) != 1890 {
				t.Fatalf("new line price = %v, want base 1490 + feta 400 = 1890", added["price"])
			}
		})
	}
}

// Covers issue #59: a line added with a count stores the whole-line price
// (count x (base + paid option values)), as the Wolt app does. Wolt sums line
// prices without multiplying them by count.
func TestCartAddWithCountStoresWholeLinePrice(t *testing.T) {
	seenAddPayload := map[string]any{}
	deps := optionsCLIDeps(&mockWolt{
		venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": optionsVenueID, "slug": optionsVenueSlug, "currency": "EUR"}}, nil
		},
		venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
			return nil, errors.New("item endpoint unavailable")
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return optionsVenueAssortment(), nil
		},
		assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
			return optionsVenueAssortment(), nil
		},
		basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{}}, nil
		},
		addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			seenAddPayload = payload
			return map[string]any{"id": "basket-1", "venue_id": optionsVenueID}, nil
		},
		basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"count": 1}, nil
		},
	})

	exitCode, out := runCLIWithDeps(t, deps,
		"cart", "add", optionsVenueSlug, "item-pizza",
		"--count", "2",
		"--option", "Extras=Feta",
		"--wtoken", "token", "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	items := asSlicePayload(t, seenAddPayload["items"])
	if len(items) != 1 {
		t.Fatalf("expected one line, got %#v", items)
	}
	added := asMapPayload(t, items[0])
	_, value := selectedOptionValue(t, added)
	if asIntPayload(added["count"]) != 2 || asIntPayload(added["price"]) != 3780 {
		t.Fatalf("line = %#v, want count 2 at 2 x (1490 + 400) = 3780", added)
	}
	if asIntPayload(value["price"]) != 400 || asIntPayload(value["count"]) != 1 {
		t.Fatalf("option value must stay priced per unit, got %#v", value)
	}
}

// `wolt cart remove` rebuilds every remaining line, so it has to restore the
// option value prices the basket response leaves out. Prices come from the
// venue assortment; the item endpoint is asked only for lines the assortment
// cannot price.
func TestCartRemoveRestoresOptionValuePricesFromCatalog(t *testing.T) {
	assortment := optionsVenueAssortment()
	// Drop the dip group from the assortment so only the item endpoint knows
	// the garlic dip price.
	assortment["options"] = asSlicePayload(t, assortment["options"])[:1]
	itemPageCalls := []string{}
	seenPayload := map[string]any{}
	deps := optionsCLIDeps(&mockWolt{
		assortmentBySlugFunc: func(_ context.Context, slug string) (map[string]any, error) {
			if slug != optionsVenueSlug {
				t.Fatalf("assortment requested for slug %q, want %q", slug, optionsVenueSlug)
			}
			return assortment, nil
		},
		venueItemPageFunc: func(_ context.Context, venueID string, itemID string) (map[string]any, error) {
			itemPageCalls = append(itemPageCalls, itemID)
			if venueID != optionsVenueID || itemID != "item-kebab" {
				return nil, errors.New("unexpected item page request")
			}
			return map[string]any{
				"id": "item-kebab",
				"options": []any{
					map[string]any{
						"id":     "item-grp-dip",
						"values": []any{map[string]any{"id": "val-garlic", "price": map[string]any{"amount": 120}}},
					},
				},
			}, nil
		},
		basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return optionsVenueBasketsPage(
				unpricedOptionLine("item-pizza", "Super Pizza", 2, 3780, "item-grp-extras", "val-feta"),
				unpricedOptionLine("item-kebab", "Kebab", 1, 1470, "item-grp-dip", "val-garlic"),
			), nil
		},
		addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			seenPayload = payload
			return map[string]any{"id": "basket-1", "venue_id": optionsVenueID}, nil
		},
		basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"count": 1}, nil
		},
	})

	exitCode, out := runCLIWithDeps(t, deps, "cart", "remove", "item-pizza", "--count", "1", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if strings.Join(itemPageCalls, ",") != "item-kebab" {
		t.Fatalf("item endpoint calls = %v, want only item-kebab", itemPageCalls)
	}
	items := asSlicePayload(t, seenPayload["items"])
	if len(items) != 2 {
		t.Fatalf("expected both lines to remain, got %#v", items)
	}
	pizza := asMapPayload(t, items[0])
	groupID, value := selectedOptionValue(t, pizza)
	if asIntPayload(pizza["count"]) != 1 || asIntPayload(pizza["price"]) != 1890 ||
		groupID != "item-grp-extras" || asIntPayload(value["price"]) != 400 {
		t.Fatalf("pizza line must drop to 1 at line price 1890 and keep feta at 400, got %#v", pizza)
	}
	kebab := asMapPayload(t, items[1])
	groupID, value = selectedOptionValue(t, kebab)
	if asIntPayload(kebab["count"]) != 1 || groupID != "item-grp-dip" || asIntPayload(value["price"]) != 120 {
		t.Fatalf("kebab line must stay with garlic dip at 120, got %#v", kebab)
	}
}

// A value no catalog prices must still block the mutation: the basket is left
// untouched rather than rebuilt with a guessed price.
func TestCartRemoveRefusesOptionValueWithoutAnyPrice(t *testing.T) {
	mutated := false
	deps := optionsCLIDeps(&mockWolt{
		assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
			return optionsVenueAssortment(), nil
		},
		venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
			return nil, errors.New("item endpoint unavailable")
		},
		basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return optionsVenueBasketsPage(
				unpricedOptionLine("item-pizza", "Super Pizza", 2, 3780, "item-grp-extras", "val-feta"),
				unpricedOptionLine("item-kebab", "Kebab", 1, 1350, "item-grp-dip", "val-retired"),
			), nil
		},
		addToBasketFunc: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			mutated = true
			return map[string]any{}, nil
		},
	})

	exitCode, out := runCLIWithDeps(t, deps, "cart", "remove", "item-pizza", "--count", "1", "--wtoken", "token", "--format", "json")
	if exitCode == 0 {
		t.Fatalf("expected a failure, got exit 0\noutput:\n%s", out)
	}
	if mutated {
		t.Fatalf("basket must not be mutated when an option value has no price")
	}
	if !strings.Contains(out, `value \"val-retired\" has no current price`) && !strings.Contains(out, `value "val-retired" has no current price`) {
		t.Fatalf("expected the unpriced value in the error, got:\n%s", out)
	}
}

// Covers issue #51 through `wolt checkout`: a basket line whose option values
// come back without prices previews with the catalog prices instead of failing.
func TestCheckoutPreviewPricesBasketOptionValuesFromCatalog(t *testing.T) {
	seenPayload := map[string]any{}
	deps := optionsCLIDeps(&mockWolt{
		basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return optionsVenueBasketsPage(
				// Whole-line price: 2 x (1490 base + 400 feta).
				unpricedOptionLine("item-pizza", "Super Pizza", 2, 3780, "item-grp-extras", "val-feta"),
			), nil
		},
		venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
			return map[string]any{
				"sections": []any{
					map[string]any{"categories": []any{map[string]any{"id": "cat-pizza", "item_ids": []any{"item-pizza"}}}},
				},
			}, nil
		},
		assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
			return optionsVenueAssortment(), nil
		},
		checkoutPreviewFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			seenPayload = payload
			return map[string]any{
				"payable_amount":   3780,
				"delivery_configs": selectedStandardDeliveryConfigs(),
				"offers":           map[string]any{"selectable": []any{}, "applied": []any{}},
				"tip_config":       map[string]any{"min_amount": 50},
			}, nil
		},
	})

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	menuItems := asSlicePayload(t, asMapPayload(t, seenPayload["purchase_plan"])["menu_items"])
	if len(menuItems) != 1 {
		t.Fatalf("expected one menu item, got %#v", menuItems)
	}
	item := asMapPayload(t, menuItems[0])
	if asIntPayload(item["count"]) != 2 || asIntPayload(item["end_amount"]) != 3780 ||
		asIntPayload(item["base_price"]) != 1490 {
		t.Fatalf("menu item = %#v, want count 2, end_amount 3780 (the line price), base_price 1490", item)
	}
	groupID, value := selectedOptionValue(t, item)
	if groupID != "item-grp-extras" || value["id"] != "val-feta" || asIntPayload(value["price"]) != 400 {
		t.Fatalf("checkout option must carry the catalog feta price 400, got group %q value %#v", groupID, value)
	}
}

// Without a price anywhere the checkout preview still fails closed.
func TestCheckoutPreviewRejectsOptionValueWithoutAnyPrice(t *testing.T) {
	previewed := false
	deps := optionsCLIDeps(&mockWolt{
		basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return optionsVenueBasketsPage(
				unpricedOptionLine("item-pizza", "Super Pizza", 1, 1890, "item-grp-extras", "val-retired"),
			), nil
		},
		venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
			return nil, errors.New("item endpoint unavailable")
		},
		assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
			return optionsVenueAssortment(), nil
		},
		checkoutPreviewFunc: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			previewed = true
			return map[string]any{}, nil
		},
	})

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "--wtoken", "token", "--format", "json")
	if exitCode == 0 || previewed {
		t.Fatalf("expected checkout to fail before previewing, exit=%d previewed=%v\noutput:\n%s", exitCode, previewed, out)
	}
	if !strings.Contains(out, "has no current price") {
		t.Fatalf("expected a missing price error, got:\n%s", out)
	}
}
