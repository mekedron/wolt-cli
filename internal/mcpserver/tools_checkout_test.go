package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/deliveryselection"
)

// TestHandleCheckoutPreviewSendsPurchasePlan is the MCP-path regression lock for
// PR #23. The old handler POSTed a flat {venue_id, currency, items,
// delivery_mode, location} body, which Wolt now rejects with
// `('body', 'purchase_plan'): Field required`. The handler must instead send the
// shared checkoutpayload.Build shape: a single top-level purchase_plan object.
func TestHandleCheckoutPreviewSendsPurchasePlan(t *testing.T) {
	const venueID = "000000000000000000000001"

	var captured map[string]any
	checkoutCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": "test-venue", "currency": "EUR"},
			}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"baskets": []any{
					map[string]any{
						"venue": map[string]any{"id": venueID, "country": "ZZZ"},
						"total": "EUR 5.00",
						"items": []any{
							map[string]any{
								"id":          "000000000000000000000101",
								"count":       1,
								"price":       500,
								"category_id": "cat-x",
							},
						},
					},
				},
			}, nil
		},
		checkoutPreviewFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			checkoutCalled = true
			captured = payload
			return map[string]any{
				"payable_amount": 500,
				"checkout_rows":  []any{},
				"delivery_configs": []any{
					map[string]any{"type": "standard", "selected": true},
				},
			}, nil
		},
	}

	tc := newToolCtx(checkoutTestDeps(wolt))

	res, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
		Venue:         venueID,
		DeliveryMode:  "standard",
	})
	if err != nil {
		t.Fatalf("handleCheckoutPreview returned error: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("expected success result, got error: %v", textContent(res))
	}
	if !checkoutCalled {
		t.Fatal("CheckoutPreview was never called — Build likely failed before the upstream request")
	}
	if _, ok := out.Data["payable_amount"]; !ok {
		t.Errorf("output data missing payable_amount: %v", out.Data)
	}

	// The captured upstream body must be the new purchase_plan shape...
	if _, ok := captured["purchase_plan"].(map[string]any); !ok {
		t.Fatalf("upstream payload missing top-level purchase_plan map; got keys %v", keysOf(captured))
	}
	// ...and must NOT carry the old rejected flat fields at the top level.
	for _, banned := range []string{"venue_id", "currency", "items", "delivery_mode", "location"} {
		if _, exists := captured[banned]; exists {
			t.Errorf("upstream payload must not contain top-level %q (old rejected MCP shape)", banned)
		}
	}
}

func TestHandleCheckoutPreviewAcceptsVenueIDSlugAndURL(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "test-venue"
	)
	tests := []struct {
		name        string
		venue       string
		firstLookup string
	}{
		{name: "id", venue: venueID, firstLookup: venueID},
		{name: "slug", venue: venueSlug, firstLookup: venueSlug},
		{
			name:        "URL",
			venue:       "https://wolt.com/en/test/venue/" + venueSlug,
			firstLookup: venueSlug,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wolt := checkoutReadyStub(venueID, map[string]any{
				"payable_amount": 500,
				"delivery_configs": []any{
					map[string]any{"type": "standard", "selected": true},
				},
			})
			staticInputs := []string{}
			wolt.venueStaticFn = func(_ context.Context, input string) (map[string]any, error) {
				staticInputs = append(staticInputs, input)
				return map[string]any{
					"venue": map[string]any{"id": venueID, "slug": venueSlug, "currency": "EUR"},
				}, nil
			}
			tc := newToolCtx(checkoutTestDeps(wolt))

			result, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
				LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
				Venue:         test.venue,
				DeliveryMode:  "standard",
			})
			if err != nil {
				t.Fatalf("handleCheckoutPreview(%q): %v", test.venue, err)
			}
			if result != nil && result.IsError {
				t.Fatalf("handleCheckoutPreview(%q) returned tool error: %s", test.venue, textContent(result))
			}
			if out.Status != "ready" || out.AppliedDeliveryMode != "standard" {
				t.Fatalf("handleCheckoutPreview(%q) output = %#v", test.venue, out)
			}
			if len(staticInputs) == 0 {
				t.Fatal("VenuePageStatic was not called")
			}
			if staticInputs[0] != test.firstLookup {
				t.Fatalf("first static lookup = %q, want %q; all lookups = %v", staticInputs[0], test.firstLookup, staticInputs)
			}
		})
	}
}

func TestHandleCheckoutPreviewNormalizesTopLevelBasketVenueIdentity(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "test-venue"
		itemID    = "000000000000000000000101"
	)
	var captured map[string]any
	staticInputs := []string{}
	wolt := checkoutReadyStub(venueID, nil)
	wolt.venueStaticFn = func(_ context.Context, input string) (map[string]any, error) {
		staticInputs = append(staticInputs, input)
		venue := map[string]any{"id": venueID, "currency": "EUR"}
		if input == venueSlug {
			venue["slug"] = venueSlug
		}
		return map[string]any{"venue": venue}, nil
	}
	wolt.basketsPageFn = func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
		return map[string]any{"baskets": []any{
			map[string]any{
				"venue_id":   venueID,
				"venue_slug": venueSlug,
				"currency":   "EUR",
				"items": []any{
					map[string]any{
						"id":          itemID,
						"name":        "Synthetic item",
						"count":       1,
						"price":       500,
						"category_id": "cat-x",
					},
				},
			},
		}}, nil
	}
	wolt.checkoutPreviewFn = func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
		captured = payload
		return map[string]any{
			"delivery_configs": []any{
				map[string]any{"type": "standard", "selected": true},
			},
		}, nil
	}
	tc := newToolCtx(checkoutTestDeps(wolt))

	result, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         venueID,
	})
	if err != nil {
		t.Fatalf("handleCheckoutPreview: %v", err)
	}
	if result != nil && result.IsError {
		t.Fatalf("unexpected tool error: %s", textContent(result))
	}
	if out.Status != "ready" {
		t.Fatalf("status = %q, want ready", out.Status)
	}
	plan := asMap(captured["purchase_plan"])
	venue := asMap(plan["venue"])
	if venue["id"] != venueID {
		t.Fatalf("purchase_plan.venue.id = %v, want %s", venue["id"], venueID)
	}
	item := asMap(asSlice(plan["menu_items"])[0])
	if item["venue_id"] != venueID {
		t.Fatalf("purchase_plan.menu_items[0].venue_id = %v, want %s", item["venue_id"], venueID)
	}
	if len(staticInputs) < 2 || staticInputs[len(staticInputs)-1] != venueSlug {
		t.Fatalf("static venue inputs = %v, want checkout enrichment by top-level slug %q", staticInputs, venueSlug)
	}
}

func TestHandleCheckoutPreviewUsesOnlyCanonicalBasketIDsDuringResolverOutage(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "test-venue"
		itemID    = "000000000000000000000101"
	)
	tests := []struct {
		name      string
		basket    map[string]any
		wantReady bool
	}{
		{
			name: "basket carries canonical id",
			basket: map[string]any{
				"venue_id":   venueID,
				"venue_slug": venueSlug,
				"currency":   "EUR",
				"items": []any{
					map[string]any{
						"id":          itemID,
						"name":        "Synthetic item",
						"count":       1,
						"price":       500,
						"category_id": "cat-x",
					},
				},
			},
			wantReady: true,
		},
		{
			name: "slug-only basket is blocked",
			basket: map[string]any{
				"venue_slug": venueSlug,
				"currency":   "EUR",
				"items": []any{
					map[string]any{
						"id":          itemID,
						"count":       1,
						"price":       500,
						"category_id": "cat-x",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkoutCalled := false
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return nil, errors.New("static venue page unavailable")
				},
				venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
					return nil, errors.New("dynamic venue page unavailable")
				},
				basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{"baskets": []any{test.basket}}, nil
				},
				checkoutPreviewFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
					checkoutCalled = true
					return map[string]any{
						"delivery_configs": []any{
							map[string]any{"type": "standard", "selected": true},
						},
					}, nil
				},
			}
			tc := newToolCtx(checkoutTestDeps(wolt))

			result, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
				LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
				Venue:         venueSlug,
			})
			if !test.wantReady {
				if err == nil || !strings.Contains(err.Error(), "canonical venue id") {
					t.Fatalf("error = %v, want canonical venue id failure", err)
				}
				if checkoutCalled {
					t.Fatal("CheckoutPreview must not be called with a slug-shaped venue id")
				}
				return
			}
			if err != nil {
				t.Fatalf("handleCheckoutPreview: %v", err)
			}
			if result != nil && result.IsError {
				t.Fatalf("unexpected tool error: %s", textContent(result))
			}
			if !checkoutCalled || out.Status != "ready" {
				t.Fatalf("checkout called = %v, status = %q; want true, ready", checkoutCalled, out.Status)
			}
			if want := "checkout preview for venue " + venueID; out.Summary != want {
				t.Fatalf("summary = %q, want %q", out.Summary, want)
			}
		})
	}
}

func TestHandleCheckoutPreviewRejectsUnconfirmedOrMismatchedDeliveryMode(t *testing.T) {
	const venueID = "000000000000000000000001"
	tests := []struct {
		name      string
		requested string
		preview   map[string]any
	}{
		{
			name:      "standard mode is not confirmed",
			requested: "standard",
			preview: map[string]any{
				"payable_amount": 500,
			},
		},
		{
			name:      "priority request is only echoed",
			requested: "priority",
			preview: map[string]any{
				"payable_amount":       500,
				"is_priority_delivery": true,
			},
		},
		{
			name:      "priority mode lacks selected config",
			requested: "priority",
			preview: map[string]any{
				"payable_amount":         500,
				"selected_delivery_mode": "priority",
			},
		},
		{
			name:      "standard request selects priority",
			requested: "standard",
			preview: map[string]any{
				"payable_amount": 650,
				"delivery_configs": []any{
					map[string]any{"type": "priority", "selected": true, "price": 250},
				},
			},
		},
		{
			name:      "nested response selects conflicting modes",
			requested: "standard",
			preview: map[string]any{
				"first": map[string]any{
					"delivery_configs": []any{
						map[string]any{"type": "standard", "selected": true},
					},
				},
				"second": map[string]any{
					"delivery_configs": []any{
						map[string]any{"type": "priority", "selected": true},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newToolCtx(checkoutTestDeps(checkoutReadyStub(venueID, test.preview)))

			result, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
				LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
				Venue:         venueID,
				DeliveryMode:  test.requested,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want tool error", result)
			}
			if out.Status != "delivery_mode_unavailable" ||
				out.Error == nil ||
				out.Error.Code != "DELIVERY_MODE_UNAVAILABLE" {
				t.Fatalf("output = %#v", out)
			}
			if out.AppliedDeliveryMode != "" {
				t.Fatalf("applied mode = %q, want empty", out.AppliedDeliveryMode)
			}
		})
	}
}

func TestHandleCheckoutPreviewReportsConfirmedPriority(t *testing.T) {
	const venueID = "000000000000000000000001"
	wolt := checkoutReadyStub(venueID, map[string]any{
		"payable_amount":       650,
		"is_priority_delivery": true,
		"checkout": map[string]any{
			"applied_delivery_mode": "priority",
			"delivery_configs": []any{
				map[string]any{"type": "standard", "price": 100},
				map[string]any{"type": "priority", "price": 250},
			},
		},
	})
	tc := newToolCtx(checkoutTestDeps(wolt))

	result, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
		Venue:         venueID,
		DeliveryMode:  "priority",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && result.IsError {
		t.Fatalf("unexpected tool error: %#v", result)
	}
	if out.Status != "ready" || out.AppliedDeliveryMode != "priority" {
		t.Fatalf("output = %#v", out)
	}
	if len(out.AvailableDeliveryModes) != 2 ||
		out.AvailableDeliveryModes[0] != "standard" ||
		out.AvailableDeliveryModes[1] != "priority" {
		t.Fatalf("available modes = %#v", out.AvailableDeliveryModes)
	}
	if out.SelectedDeliveryConfig["type"] != "priority" ||
		out.SelectedDeliveryConfig["price"] != 250 {
		t.Fatalf("selected config = %#v", out.SelectedDeliveryConfig)
	}
	if _, duplicated := out.SelectedDeliveryConfig["payable_amount"]; duplicated {
		t.Fatalf("selected config duplicated full preview: %#v", out.SelectedDeliveryConfig)
	}
	if _, duplicated := out.SelectedDeliveryConfig["delivery_configs"]; duplicated {
		t.Fatalf("selected config duplicated full preview: %#v", out.SelectedDeliveryConfig)
	}
}

func TestCheckoutDeliverySelectionChecksEveryConfigLabel(t *testing.T) {
	config := map[string]any{
		"type":     "home_delivery",
		"title":    "Priority delivery",
		"selected": true,
	}
	state := deliveryselection.Parse(map[string]any{
		"delivery_configs": []any{config},
	})
	if state.SelectedMode != "priority" {
		t.Fatalf("selected mode = %q, want priority", state.SelectedMode)
	}
	if len(state.AvailableModes) != 2 || state.AvailableModes[1] != "priority" {
		t.Fatalf("available modes = %#v", state.AvailableModes)
	}
	if state.SelectedConfig["title"] != "Priority delivery" {
		t.Fatalf("selected config = %#v", state.SelectedConfig)
	}
}

func TestCheckoutDeliverySelectionIsDeterministicWithConflictingNestedConfigs(t *testing.T) {
	payload := map[string]any{
		"z_priority": map[string]any{
			"delivery_configs": []any{
				map[string]any{"id": "priority-z", "type": "priority", "selected": true},
			},
		},
		"a_standard": map[string]any{
			"delivery_configs": []any{
				map[string]any{"id": "standard-a", "type": "standard", "selected": true},
			},
		},
	}

	for range 100 {
		state := deliveryselection.Parse(payload)
		if !state.SelectionAmbiguous ||
			state.SelectedMode != "" ||
			state.SelectedConfig != nil {
			t.Fatalf("conflicting selection was not reported as ambiguous: %#v", state)
		}
	}
}

func TestCheckoutUnavailableItemsSurviveTypedMCPResponse(t *testing.T) {
	const (
		venueID = "000000000000000000000001"
		itemID  = "000000000000000000000101"
	)
	checkoutCalled := false
	wolt := checkoutReadyStub(venueID, map[string]any{})
	wolt.assortmentItemsFn = func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
		return map[string]any{"items": []any{
			map[string]any{
				"id":                  itemID,
				"name":                "Unavailable item",
				"disabled_info":       map[string]any{"disable_text": "Sold out"},
				"purchasable_balance": 0,
			},
		}}, nil
	}
	wolt.checkoutPreviewFn = func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
		checkoutCalled = true
		return map[string]any{}, nil
	}
	_, client := connectInMemory(t, checkoutTestDeps(wolt))
	defer func() { _ = client.Close() }()

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "wolt_checkout_preview",
		Arguments: map[string]any{
			"venue": venueID,
			"lat":   10.25,
			"lon":   20.5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(textContent(result), "Sold out") {
		t.Fatalf("result = %#v", result)
	}
	structured := asMap(result.StructuredContent)
	if structured == nil {
		t.Fatalf("structuredContent = %#v", result.StructuredContent)
	}
	if structured["status"] != "blocked" {
		t.Fatalf("status = %v, want blocked", structured["status"])
	}
	errorInfo := asMap(structured["error"])
	if errorInfo["code"] != "UNAVAILABLE_ITEMS" || errorInfo["retryable"] != false {
		t.Fatalf("error = %#v", errorInfo)
	}
	items := asSlice(structured["unavailable_items"])
	if len(items) != 1 {
		t.Fatalf("unavailable_items = %#v", items)
	}
	item := asMap(items[0])
	if item["item_id"] != itemID || item["name"] != "Unavailable item" || item["reason"] != "Sold out" {
		t.Fatalf("unavailable item = %#v", item)
	}
	basket := asMap(structured["basket"])
	if len(asSlice(basket["items"])) != 1 {
		t.Fatalf("basket = %#v", basket)
	}
	if checkoutCalled {
		t.Fatal("CheckoutPreview must not be called when a basket item is unavailable")
	}
}

func checkoutReadyStub(venueID string, preview map[string]any) *stubWolt {
	return &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": "test-venue", "currency": "EUR"},
			}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"venue": map[string]any{"id": venueID, "slug": "test-venue"},
					"total": "EUR5.00",
					"items": []any{
						map[string]any{
							"id":          "000000000000000000000101",
							"name":        "Available item",
							"count":       1,
							"price":       500,
							"category_id": "cat-x",
						},
					},
				},
			}}, nil
		},
		checkoutPreviewFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			return preview, nil
		},
	}
}

func checkoutTestDeps(wolt *stubWolt) Deps {
	return Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
