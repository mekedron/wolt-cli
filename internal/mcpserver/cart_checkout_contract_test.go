package mcpserver

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

type panicMCPRegressionAPI struct {
	woltgateway.API
}

func regressionMCPToolCtx(api woltgateway.API) *ToolCtx {
	return newToolCtx(Deps{
		Wolt: api,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:     "default",
			WToken:   "synthetic-token",
			Location: domain.Location{Lat: 10.25, Lon: 20.5},
		}},
		Location: &stubLocation{},
	})
}

func TestCartMutationRejectsNegativeCountBeforeUpstream(t *testing.T) {
	tc := regressionMCPToolCtx(&panicMCPRegressionAPI{})

	t.Run("add", func(t *testing.T) {
		_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
			Venue:  "synthetic-market",
			ItemID: "000000000000000000000301",
			Count:  -1,
		})
		if err == nil || !strings.Contains(err.Error(), "zero or greater") {
			t.Fatalf("negative add count error = %v", err)
		}
	})

	t.Run("remove", func(t *testing.T) {
		_, _, err := tc.handleCartRemove(context.Background(), nil, CartRemoveInput{
			Venue:  "synthetic-market",
			ItemID: "000000000000000000000301",
			Count:  -1,
		})
		if err == nil || !strings.Contains(err.Error(), "zero or greater") {
			t.Fatalf("negative remove count error = %v", err)
		}
	})
}

func TestCartAddRejectsUnidentifiableExistingBasket(t *testing.T) {
	addCalls := 0
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{
					"id":       "000000000000000000000341",
					"slug":     "synthetic-market",
					"currency": "EUR",
				},
			}, nil
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
	tc := regressionMCPToolCtx(wolt)

	_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		Venue:    "synthetic-market",
		ItemID:   "000000000000000000000342",
		Price:    500,
		Currency: "EUR",
		Name:     "Synthetic item",
	})
	if err == nil || !strings.Contains(err.Error(), "no venue identity") {
		t.Fatalf("cart add error = %v, want unidentifiable basket", err)
	}
	if addCalls != 0 {
		t.Fatalf("AddToBasket called %d times, want 0", addCalls)
	}
}

func TestCartAddRejectsRequiredOptionsItCannotRepresent(t *testing.T) {
	const (
		venueID = "000000000000000000000343"
		itemID  = "000000000000000000000344"
	)
	addCalls := 0
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": "synthetic-market", "currency": "EUR"},
			}, nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"items": []any{
					map[string]any{
						"id":                  itemID,
						"name":                "Configured item",
						"price":               500,
						"purchasable_balance": 10,
						"option_groups": []any{
							map[string]any{
								"id":       "size",
								"required": true,
								"values":   []any{map[string]any{"id": "large", "price": 0}},
							},
						},
					},
				},
			}, nil
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			addCalls++
			return map[string]any{}, nil
		},
	}
	tc := regressionMCPToolCtx(wolt)

	_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		Venue:  "synthetic-market",
		ItemID: itemID,
	})
	if err == nil || !strings.Contains(err.Error(), "requires option selections") {
		t.Fatalf("cart add error = %v, want required-option rejection", err)
	}
	if addCalls != 0 {
		t.Fatalf("AddToBasket called %d times, want 0", addCalls)
	}
}

func TestCartMutationsSerializeReadModifyWrite(t *testing.T) {
	const (
		venueID = "000000000000000000000345"
		itemA   = "000000000000000000000346"
		itemB   = "000000000000000000000347"
	)
	var (
		stateMu      sync.Mutex
		pageCalls    int
		state        = map[string]any{"baskets": []any{}}
		releaseOnce  sync.Once
		firstRead    = make(chan struct{})
		secondRead   = make(chan struct{})
		releaseFirst = make(chan struct{})
	)
	release := func() {
		releaseOnce.Do(func() { close(releaseFirst) })
	}
	defer release()

	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": "synthetic-market", "currency": "EUR"},
			}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			stateMu.Lock()
			pageCalls++
			call := pageCalls
			stateMu.Unlock()
			switch call {
			case 1:
				close(firstRead)
				<-releaseFirst
			case 2:
				close(secondRead)
			}
			stateMu.Lock()
			defer stateMu.Unlock()
			return state, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			stateMu.Lock()
			defer stateMu.Unlock()
			state = map[string]any{
				"baskets": []any{
					map[string]any{
						"basket_id":  "basket-serialized",
						"venue_id":   venueID,
						"venue_slug": "synthetic-market",
						"currency":   "EUR",
						"items":      payload["items"],
					},
				},
			}
			return map[string]any{"id": "basket-serialized"}, nil
		},
	}
	tc := regressionMCPToolCtx(wolt)
	results := make(chan error, 2)
	add := func(itemID string) {
		_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
			Venue:    "synthetic-market",
			ItemID:   itemID,
			Price:    500,
			Currency: "EUR",
			Name:     itemID,
		})
		results <- err
	}

	go add(itemA)
	<-firstRead
	go add(itemB)
	select {
	case <-secondRead:
		release()
		t.Fatal("second cart mutation reached its basket read before the first mutation completed")
	case <-time.After(100 * time.Millisecond):
		release()
	}
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("cart mutation error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for serialized cart mutation")
		}
	}

	stateMu.Lock()
	baskets := payloadutil.BasketRows(state)
	items := payloadutil.Slice(baskets[0]["items"])
	stateMu.Unlock()
	if len(items) != 2 {
		t.Fatalf("final basket items = %#v, want both concurrent additions", items)
	}
}

func TestCheckoutRejectsNegativeTipBeforeUpstream(t *testing.T) {
	tc := regressionMCPToolCtx(&panicMCPRegressionAPI{})
	_, _, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		Venue: "synthetic-market",
		Tip:   -1,
	})
	if err == nil || !strings.Contains(err.Error(), "zero or greater") {
		t.Fatalf("negative checkout tip error = %v", err)
	}
}

func TestCartClearUsesBothContainersAndCompatibilityIDs(t *testing.T) {
	deletedIDs := []string{}
	wolt := &stubWolt{
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
	tc := regressionMCPToolCtx(wolt)

	_, out, err := tc.handleCartClear(context.Background(), nil, CartClearInput{})
	if err != nil {
		t.Fatalf("handleCartClear: %v", err)
	}
	if strings.Join(deletedIDs, ",") != "basket-a,basket-b" {
		t.Fatalf("deleted ids = %v, want both compatibility containers", deletedIDs)
	}
	if out.Deleted != 2 {
		t.Fatalf("clear output = %#v", out)
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
			wolt := &stubWolt{
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
			tc := regressionMCPToolCtx(wolt)

			_, _, err := tc.handleCartClear(context.Background(), nil, CartClearInput{})
			if err == nil || !strings.Contains(err.Error(), "not all basket ids") {
				t.Fatalf("partial clear error = %v", err)
			}
			if deleteCalls != 0 {
				t.Fatalf("DeleteBaskets called %d times, want 0", deleteCalls)
			}
		})
	}
}

func TestFilterBasketPageFiltersBothPopulatedContainers(t *testing.T) {
	ref := venueRef{
		Input: "synthetic-market",
		ID:    "000000000000000000000321",
		Slug:  "synthetic-market",
	}
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":         "basket-a",
				"venue_id":   ref.ID,
				"venue_slug": ref.Slug,
			},
			map[string]any{
				"id":         "basket-unrelated-a",
				"venue_id":   "000000000000000000000322",
				"venue_slug": "unrelated-a",
			},
		},
		"results": []any{
			map[string]any{
				"basket_id":  "basket-b",
				"venue_id":   ref.ID,
				"venue_slug": ref.Slug,
			},
			map[string]any{
				"basket_id":  "basket-unrelated-b",
				"venue_id":   "000000000000000000000323",
				"venue_slug": "unrelated-b",
			},
		},
	}

	filtered, err := filterBasketPage(page, ref, ref.Input)
	if err != nil {
		t.Fatalf("filterBasketPage: %v", err)
	}
	baskets := asSlice(filtered["baskets"])
	results := asSlice(filtered["results"])
	if len(baskets) != 1 || asString(asMap(baskets[0])["id"]) != "basket-a" {
		t.Fatalf("filtered baskets = %#v", baskets)
	}
	if len(results) != 1 || asString(asMap(results[0])["basket_id"]) != "basket-b" {
		t.Fatalf("filtered results = %#v", results)
	}
}

func TestCheckoutPayloadPreservesNestedBasketPricesThroughHandler(t *testing.T) {
	const (
		venueID = "000000000000000000000311"
		itemID  = "000000000000000000000312"
	)
	var captured map[string]any
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{
					"id":       venueID,
					"slug":     "synthetic-market",
					"currency": "EUR",
				},
			}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"results": []any{
					map[string]any{
						"basket_id": "basket-nested-prices",
						"venue": map[string]any{
							"id":       venueID,
							"slug":     "synthetic-market",
							"currency": "EUR",
						},
						"total": "EUR 7.50",
						"items": []any{
							map[string]any{
								"id":          itemID,
								"name":        "Configured item",
								"count":       1,
								"price":       map[string]any{"amount": 650},
								"category_id": "category-a",
								"options": []any{
									map[string]any{
										"id": "addon",
										"values": []any{
											map[string]any{
												"id":    "extra",
												"count": 1,
												"price": map[string]any{"amount": 100},
											},
										},
									},
								},
							},
						},
					},
				},
			}, nil
		},
		checkoutPreviewFn: func(
			_ context.Context,
			payload map[string]any,
			_ woltgateway.AuthContext,
		) (map[string]any, error) {
			captured = payload
			return map[string]any{
				"payable_amount": 750,
				"delivery_configs": []any{
					map[string]any{"type": "standard", "selected": true},
				},
			}, nil
		},
	}
	tc := regressionMCPToolCtx(wolt)

	_, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
		Venue:         venueID,
	})
	if err != nil {
		t.Fatalf("handleCheckoutPreview: %v", err)
	}
	if out.Status != "ready" {
		t.Fatalf("checkout output = %#v", out)
	}
	plan := asMap(captured["purchase_plan"])
	item := asMap(asSlice(plan["menu_items"])[0])
	if asInt(item["base_price"]) != 650 {
		t.Fatalf("base_price = %v, want 650", item["base_price"])
	}
	option := asMap(asSlice(item["options"])[0])
	value := asMap(asSlice(option["values"])[0])
	if asInt(value["price"]) != 100 {
		t.Fatalf("option price = %v, want 100", value["price"])
	}
}
