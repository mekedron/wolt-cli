package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

const (
	regressionVenueID   = "000000000000000000000201"
	regressionVenueSlug = "synthetic-market"
	regressionItemID    = "000000000000000000000202"
	regressionOtherID   = "000000000000000000000203"
)

type cartCheckoutRegressionAPI struct {
	*testWoltAPI
	basketCountFn func(context.Context, woltgateway.AuthContext) (map[string]any, error)
}

func (api *cartCheckoutRegressionAPI) BasketCount(
	ctx context.Context,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if api.basketCountFn != nil {
		return api.basketCountFn(ctx, auth)
	}
	return map[string]any{}, nil
}

// An embedded nil API makes any accidental upstream invocation panic. Guard
// tests use it to prove validation runs before all network-facing methods.
type panicCLIRegressionAPI struct {
	woltgateway.API
}

func regressionCLIDeps(api woltgateway.API) Dependencies {
	return Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 10.25, Lon: 20.5},
			WToken:    "synthetic-token",
		}},
	}
}

func runCLIRegressionCommand(
	t *testing.T,
	cmd *cobra.Command,
	args ...string,
) (*bytes.Buffer, error) {
	t.Helper()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs(args)
	return output, cmd.ExecuteContext(context.Background())
}

func syntheticVenuePayload() map[string]any {
	return map[string]any{
		"venue": map[string]any{
			"id":       regressionVenueID,
			"slug":     regressionVenueSlug,
			"name":     "Synthetic Market",
			"currency": "EUR",
		},
	}
}

func configuredRegressionLine(itemID, name string, count, price int) map[string]any {
	return map[string]any{
		"id":    itemID,
		"name":  name,
		"count": count,
		"price": map[string]any{"amount": price},
		"options": []any{
			map[string]any{
				"id": "addon",
				"values": []any{
					map[string]any{"id": "double", "count": 2, "price": map[string]any{"amount": 50}},
				},
			},
		},
	}
}

func unrelatedRegressionBasket(venueID string) map[string]any {
	basket := map[string]any{
		"id": "basket-other",
		"items": []any{
			map[string]any{"id": "other-line", "count": 3, "price": 100},
		},
	}
	if venueID != "" {
		basket["venue_id"] = venueID
	}
	return basket
}

func TestCheckoutNegativeTipFailsBeforeUpstream(t *testing.T) {
	output, err := runCLIRegressionCommand(
		t,
		newCheckoutPreviewCommand(regressionCLIDeps(&panicCLIRegressionAPI{})),
		"--tip", "-1",
		"--format", "json",
	)
	if err == nil {
		t.Fatalf("negative tip succeeded:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "WOLT_INVALID_ARGUMENT") {
		t.Fatalf("unexpected negative-tip output:\n%s", output.String())
	}
}

func TestCartAddRejectsUnidentifiableExistingBasket(t *testing.T) {
	addCalls := 0
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return syntheticVenuePayload(), nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"baskets": []any{
					map[string]any{"basket_id": "unknown-venue", "items": []any{}},
				},
			}, nil
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			addCalls++
			return map[string]any{}, nil
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartAddCommand(regressionCLIDeps(api)),
		regressionVenueSlug,
		regressionItemID,
		"--price", "500",
		"--currency", "EUR",
		"--format", "json",
	)
	if err == nil || !strings.Contains(output.String(), "no venue identity") {
		t.Fatalf("cart add error = %v\n%s", err, output.String())
	}
	if addCalls != 0 {
		t.Fatalf("AddToBasket called %d times, want 0", addCalls)
	}
}

func TestCartClearUsesBothContainersAndCompatibilityIDs(t *testing.T) {
	deletedIDs := []string{}
	api := &testWoltAPI{
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"baskets": []any{
					map[string]any{"id": "basket-a", "items": []any{}},
				},
				"results": []any{
					map[string]any{"basket_id": "basket-b", "items": []any{}},
				},
			}, nil
		},
		deleteBasketsFn: func(
			_ context.Context,
			ids []string,
			_ woltgateway.AuthContext,
		) (map[string]any, error) {
			deletedIDs = append([]string{}, ids...)
			return map[string]any{}, nil
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartClearCommand(regressionCLIDeps(api)),
		"--all",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart clear: %v\n%s", err, output.String())
	}
	if strings.Join(deletedIDs, ",") != "basket-a,basket-b" {
		t.Fatalf("deleted ids = %v, want both compatibility containers", deletedIDs)
	}
	data := decodeCLIData(t, output)
	if asInt(data["cleared_baskets"]) != 2 {
		t.Fatalf("clear output = %#v", data)
	}
}

func TestCartClearFailsClosedForIncompleteBasketPage(t *testing.T) {
	tests := []struct {
		name string
		page map[string]any
	}{
		{
			name: "blank id",
			page: map[string]any{
				"baskets": []any{map[string]any{"id": "basket-a", "items": []any{}}},
				"results": []any{map[string]any{"basket_id": " ", "items": []any{}}},
			},
		},
		{name: "missing container", page: map[string]any{}},
		{name: "wrong container", page: map[string]any{"baskets": "invalid"}},
		{name: "non-object basket", page: map[string]any{"baskets": []any{"invalid"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deleteCalls := 0
			api := &testWoltAPI{
				basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
					return test.page, nil
				},
				deleteBasketsFn: func(
					context.Context,
					[]string,
					woltgateway.AuthContext,
				) (map[string]any, error) {
					deleteCalls++
					return map[string]any{}, nil
				},
			}

			output, err := runCLIRegressionCommand(
				t,
				newCartClearCommand(regressionCLIDeps(api)),
				"--all",
				"--format", "json",
			)
			if err == nil {
				t.Fatalf("partial clear succeeded:\n%s", output.String())
			}
			if deleteCalls != 0 {
				t.Fatalf("DeleteBaskets called %d times, want 0", deleteCalls)
			}
			if !strings.Contains(output.String(), "WOLT_BASKET_UNRESOLVED") {
				t.Fatalf("unexpected partial-clear output:\n%s", output.String())
			}
		})
	}
}

func TestCartAddKeepsFallbackCountsConfiguredTotalAndNestedPrices(t *testing.T) {
	withIsolatedSlugCache(t)

	initialPage := map[string]any{
		"baskets": []any{
			map[string]any{
				"basket_id":  "basket-target",
				"venue_id":   regressionVenueID,
				"venue_slug": regressionVenueSlug,
				"currency":   "EUR",
				"items": []any{
					configuredRegressionLine(regressionOtherID, "Existing configured item", 2, 300),
				},
			},
			unrelatedRegressionBasket("000000000000000000000299"),
		},
	}

	pageCalls := 0
	var mutation map[string]any
	api := &cartCheckoutRegressionAPI{
		testWoltAPI: &testWoltAPI{
			venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
				return syntheticVenuePayload(), nil
			},
			assortmentItemsFn: func(
				context.Context,
				string,
				[]string,
				woltgateway.AuthContext,
			) (map[string]any, error) {
				return map[string]any{
					"items": []any{
						map[string]any{
							"id":                  regressionItemID,
							"title":               "Fresh configured item",
							"price":               map[string]any{"amount": 500},
							"purchasable_balance": 10,
							"options": []any{
								map[string]any{"option_id": "SIZE"},
							},
						},
					},
				}, nil
			},
			basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				pageCalls++
				if pageCalls == 1 {
					return initialPage, nil
				}
				return map[string]any{"baskets": []any{}}, nil
			},
			addToBasketFn: func(
				_ context.Context,
				payload map[string]any,
				_ woltgateway.AuthContext,
			) (map[string]any, error) {
				mutation = payload
				return map[string]any{
					"basket_id": "basket-target",
					"venue_id":  regressionVenueID,
				}, nil
			},
			venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"name":  "Stale configured item",
					"price": map[string]any{"amount": 450, "currency": "EUR"},
					"items": []any{
						map[string]any{
							"id": regressionItemID,
							"option_groups": []any{
								map[string]any{
									"id":   "size",
									"name": "Size",
									"values": []any{
										map[string]any{
											"id":    "large",
											"name":  "Large",
											"price": map[string]any{"amount": 100},
										},
									},
								},
							},
						},
					},
				}, nil
			},
		},
		basketCountFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return nil, errors.New("count refresh unavailable")
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartAddCommand(regressionCLIDeps(api)),
		regressionVenueID,
		regressionItemID,
		"--option", "size=large:2",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart add: %v\n%s", err, output.String())
	}
	if pageCalls != 2 {
		t.Fatalf("BasketsPage calls = %d, want pre/post snapshots", pageCalls)
	}

	items := asSlice(mutation["items"])
	if len(items) != 2 {
		t.Fatalf("mutation items = %#v", items)
	}
	added := asMap(items[1])
	if asString(added["name"]) != "Fresh configured item" ||
		asInt(added["price"]) != 500 {
		t.Fatalf("fresh line metadata = %#v", added)
	}
	option := asMap(asSlice(added["options"])[0])
	value := asMap(asSlice(option["values"])[0])
	if asInt(value["price"]) != 100 || asInt(value["count"]) != 2 {
		t.Fatalf("configured option = %#v", value)
	}

	data := decodeCLIData(t, output)
	if asString(data["basket_id"]) != "basket-target" {
		t.Fatalf("basket compatibility id = %#v", data["basket_id"])
	}
	if asInt(data["total_items"]) != 6 {
		t.Fatalf("fallback total_items = %v, want 6", data["total_items"])
	}
	if got := asInt(asMap(data["total"])["amount"]); got != 1500 {
		t.Fatalf("fallback total = %d, want configured target-basket total 1500", got)
	}
}

func TestCartAddHydratesIDOnlyOptionReferencesFromFullAssortment(t *testing.T) {
	withIsolatedSlugCache(t)

	var mutation map[string]any
	api := &cartCheckoutRegressionAPI{
		testWoltAPI: &testWoltAPI{
			venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
				return syntheticVenuePayload(), nil
			},
			assortmentItemsFn: func(
				context.Context,
				string,
				[]string,
				woltgateway.AuthContext,
			) (map[string]any, error) {
				return map[string]any{
					"items": []any{
						map[string]any{
							"id":                  regressionItemID,
							"name":                "Configurable item",
							"price":               500,
							"purchasable_balance": 10,
							"options": []any{
								map[string]any{"option_id": "size"},
							},
						},
					},
				}, nil
			},
			assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"items": []any{
						map[string]any{
							"id":      regressionItemID,
							"name":    "Configurable item",
							"price":   500,
							"options": []any{map[string]any{"option_id": "SIZE"}},
						},
					},
					"options": []any{
						map[string]any{"id": "size"},
					},
					"option_groups": []any{
						map[string]any{
							"id": "size",
							"values": []any{
								map[string]any{"id": "large", "price": 125},
							},
						},
					},
					"venue": map[string]any{"currency": "EUR"},
				}, nil
			},
			basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"baskets": []any{}}, nil
			},
			addToBasketFn: func(
				_ context.Context,
				payload map[string]any,
				_ woltgateway.AuthContext,
			) (map[string]any, error) {
				mutation = payload
				return map[string]any{"id": "basket-new", "venue_id": regressionVenueID}, nil
			},
			venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
				return nil, errors.New("item page unavailable")
			},
		},
		basketCountFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return nil, errors.New("count refresh unavailable")
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartAddCommand(regressionCLIDeps(api)),
		regressionVenueID,
		regressionItemID,
		"--option", "SIZE=large",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart add: %v\n%s", err, output.String())
	}
	added := asMap(asSlice(mutation["items"])[0])
	option := asMap(asSlice(added["options"])[0])
	value := asMap(asSlice(option["values"])[0])
	if asInt(value["price"]) != 125 {
		t.Fatalf("full-assortment option price = %v, want 125; mutation %#v", value["price"], mutation)
	}
}

func TestCartRemoveKeepsFallbackCountsAndConfiguredTotalOnEmptyRefresh(t *testing.T) {
	initialPage := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":         "basket-target",
				"venue_id":   regressionVenueID,
				"venue_slug": regressionVenueSlug,
				"currency":   "EUR",
				"items": []any{
					map[string]any{
						"id":      regressionItemID,
						"name":    "Removed item",
						"count":   2,
						"price":   map[string]any{"amount": 500},
						"options": []any{},
					},
					configuredRegressionLine(regressionOtherID, "Configured remainder", 2, 300),
				},
			},
			unrelatedRegressionBasket("000000000000000000000299"),
		},
	}
	pageCalls := 0
	var mutation map[string]any
	api := &cartCheckoutRegressionAPI{
		testWoltAPI: &testWoltAPI{
			basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				pageCalls++
				if pageCalls == 1 {
					return initialPage, nil
				}
				return map[string]any{"baskets": []any{}}, nil
			},
			addToBasketFn: func(
				_ context.Context,
				payload map[string]any,
				_ woltgateway.AuthContext,
			) (map[string]any, error) {
				mutation = payload
				return map[string]any{}, nil
			},
		},
		basketCountFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return nil, errors.New("count refresh unavailable")
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartRemoveCommand(regressionCLIDeps(api)),
		regressionItemID,
		"--count", "1",
		"--venue-id", regressionVenueID,
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart remove: %v\n%s", err, output.String())
	}
	if pageCalls != 2 {
		t.Fatalf("BasketsPage calls = %d, want pre/post snapshots", pageCalls)
	}
	if len(asSlice(mutation["items"])) != 2 {
		t.Fatalf("replacement mutation = %#v", mutation)
	}
	data := decodeCLIData(t, output)
	if asInt(data["total_items"]) != 6 {
		t.Fatalf("fallback total_items = %v, want 6", data["total_items"])
	}
	if got := asInt(asMap(data["total"])["amount"]); got != 1300 {
		t.Fatalf("fallback total = %d, want 1300", got)
	}
}

func TestCartRemoveIdentitylessFinalLineDoesNotSelectRemainingBasket(t *testing.T) {
	const removedBasketID = "identityless-basket"
	pageCalls := 0
	deletedIDs := []string{}
	api := &cartCheckoutRegressionAPI{
		testWoltAPI: &testWoltAPI{
			basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				pageCalls++
				if pageCalls == 1 {
					return map[string]any{
						"baskets": []any{
							map[string]any{
								"basket_id": removedBasketID,
								"items": []any{
									map[string]any{
										"id":    regressionItemID,
										"count": 1,
										"price": map[string]any{"amount": 500},
									},
								},
							},
							map[string]any{
								"id":         "remaining-basket",
								"venue_id":   "000000000000000000000298",
								"venue_slug": "remaining-market",
								"currency":   "EUR",
								"telemetry":  map[string]any{"basket_total": 900},
								"items": []any{
									map[string]any{"id": "remaining-line", "count": 3, "price": 300},
								},
							},
						},
					}, nil
				}
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":         "remaining-basket",
							"venue_id":   "000000000000000000000298",
							"venue_slug": "remaining-market",
							"currency":   "EUR",
							"telemetry":  map[string]any{"basket_total": 900},
							"items": []any{
								map[string]any{"id": "remaining-line", "count": 3, "price": 300},
							},
						},
					},
				}, nil
			},
			deleteBasketsFn: func(
				_ context.Context,
				ids []string,
				_ woltgateway.AuthContext,
			) (map[string]any, error) {
				deletedIDs = append([]string{}, ids...)
				return map[string]any{}, nil
			},
		},
		basketCountFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
			return nil, errors.New("count refresh unavailable")
		},
	}

	output, err := runCLIRegressionCommand(
		t,
		newCartRemoveCommand(regressionCLIDeps(api)),
		regressionItemID,
		"--all",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart remove final line: %v\n%s", err, output.String())
	}
	if strings.Join(deletedIDs, ",") != removedBasketID {
		t.Fatalf("deleted ids = %v", deletedIDs)
	}
	data := decodeCLIData(t, output)
	if asInt(data["total_items"]) != 3 {
		t.Fatalf("fallback total_items = %v, want remaining basket count 3", data["total_items"])
	}
	if got := asInt(asMap(data["total"])["amount"]); got != 0 {
		t.Fatalf("cleared basket total = %d, must not copy remaining basket total", got)
	}
}

func TestCheckoutFailsClosedWhenWoltDoesNotConfirmRequestedMode(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		preview   map[string]any
	}{
		{
			name:      "priority selection is ambiguous",
			requested: "priority",
			preview: map[string]any{
				"payable_amount":       500,
				"is_priority_delivery": true,
				"delivery_configs": []any{
					map[string]any{"type": "standard", "selected": true},
					map[string]any{"type": "priority", "selected": true},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withIsolatedSlugCache(t)

			checkoutCalls := 0
			api := &cartCheckoutRegressionAPI{
				testWoltAPI: &testWoltAPI{
					venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
						return syntheticVenuePayload(), nil
					},
					basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
						return map[string]any{
							"baskets": []any{
								map[string]any{
									"basket_id": "basket-target",
									"venue": map[string]any{
										"id":       regressionVenueID,
										"slug":     regressionVenueSlug,
										"currency": "EUR",
									},
									"total": "EUR 5.00",
									"items": []any{
										map[string]any{
											"id":          regressionItemID,
											"name":        "Checkout item",
											"count":       1,
											"price":       map[string]any{"amount": 500},
											"category_id": "category-a",
										},
									},
								},
							},
						}, nil
					},
					checkoutPreviewFn: func(
						context.Context,
						map[string]any,
						woltgateway.AuthContext,
					) (map[string]any, error) {
						checkoutCalls++
						return test.preview, nil
					},
				},
			}

			output, err := runCLIRegressionCommand(
				t,
				newCheckoutPreviewCommand(regressionCLIDeps(api)),
				"--venue-id", regressionVenueID,
				"--delivery-mode", test.requested,
				"--format", "json",
			)
			if err == nil {
				t.Fatalf("unconfirmed %s succeeded:\n%s", test.requested, output.String())
			}
			if checkoutCalls != 1 {
				t.Fatalf("CheckoutPreview calls = %d, want 1", checkoutCalls)
			}
			if !strings.Contains(output.String(), "WOLT_DELIVERY_MODE_UNAVAILABLE") {
				t.Fatalf("unexpected %s output:\n%s", test.requested, output.String())
			}
		})
	}
}
