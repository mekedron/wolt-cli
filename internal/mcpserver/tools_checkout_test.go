package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// TestHandleCheckoutPreviewSendsPurchasePlan is the MCP-path regression lock for
// PR #23. The old handler POSTed a flat {venue_id, currency, items,
// delivery_mode, location} body, which Wolt now rejects with
// `('body', 'purchase_plan'): Field required`. The handler must instead send the
// shared checkoutpayload.Build shape: a single top-level purchase_plan object.
func TestHandleCheckoutPreviewSendsPurchasePlan(t *testing.T) {
	const venueID = "5f9a1b2c3d4e5f6071829304"

	var captured map[string]any
	checkoutCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": slug, "slug": "test-venue"}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"baskets": []any{
					map[string]any{
						"venue": map[string]any{"id": venueID, "country": "FIN"},
						"total": "€5.00",
						"items": []any{
							map[string]any{
								"id":          "627cb2c7e2a6f0a1b2c3d4e5",
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
			return map[string]any{"payable_amount": 500, "checkout_rows": []any{}}, nil
		},
	}

	tc := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})

	res, out, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 60.17, Lon: 24.94},
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

// TestHandleCheckoutPreviewResolvesSlugWithoutRestaurantEndpoint locks the id→
// slug path that current item validation depends on. Wolt's
// restaurant-api/v3/venues/<id> document answers HTTP 410 for every client, so a
// venue passed as a bare ObjectID must still reach a slug through the venue
// page — otherwise the preview is refused for want of an availability check.
func TestHandleCheckoutPreviewResolvesSlugWithoutRestaurantEndpoint(t *testing.T) {
	const venueID = "5f9a1b2c3d4e5f6071829304"

	assortmentSlug := ""
	checkoutCalled := false
	wolt := &stubWolt{
		restaurantFn: func(context.Context, string) (*domain.Restaurant, error) {
			return nil, fmt.Errorf("status=410; retired upstream")
		},
		// Serves both the id→slug lookup and the currency lookup that
		// checkoutpayload.Build performs on the resolved slug.
		venueStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			if slug != venueID && slug != "test-venue" {
				t.Errorf("unexpected venue page lookup for %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": venueID, "slug": "test-venue", "currency": "EUR"}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					// No slug on the basket venue: the id is the only lead.
					"venue": map[string]any{"id": venueID, "country": "FIN"},
					"total": "€5.00",
					"items": []any{map[string]any{
						"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500,
					}},
				},
			}}, nil
		},
		assortmentItemsFn: func(_ context.Context, slug string, itemIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
			assortmentSlug = slug
			return availableStubItems(itemIDs), nil
		},
		checkoutPreviewFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			checkoutCalled = true
			return map[string]any{"payable_amount": 500}, nil
		},
	}

	tc := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})

	res, _, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 60.17, Lon: 24.94},
		Venue:         venueID,
	})
	if err != nil {
		t.Fatalf("handleCheckoutPreview returned error: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("expected success result, got error: %v", textContent(res))
	}
	if assortmentSlug != "test-venue" {
		t.Errorf("availability lookup must use the resolved slug, got %q", assortmentSlug)
	}
	if !checkoutCalled {
		t.Fatal("CheckoutPreview was never called")
	}
}

// TestHandleCheckoutPreviewPrefersBasketSlug pins the cheapest slug source: the
// basket Wolt just returned already names its venue, so no id→slug round-trip
// is warranted.
func TestHandleCheckoutPreviewPrefersBasketSlug(t *testing.T) {
	const venueID = "5f9a1b2c3d4e5f6071829304"

	assortmentSlug := ""
	wolt := &stubWolt{
		restaurantFn: func(context.Context, string) (*domain.Restaurant, error) {
			return nil, fmt.Errorf("status=410; retired upstream")
		},
		venueStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": venueID, "currency": "EUR"}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"venue": map[string]any{"id": venueID, "slug": "basket-venue", "country": "FIN"},
					"total": "€5.00",
					"items": []any{map[string]any{
						"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500,
					}},
				},
			}}, nil
		},
		assortmentItemsFn: func(_ context.Context, slug string, itemIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
			assortmentSlug = slug
			return availableStubItems(itemIDs), nil
		},
		checkoutPreviewFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"payable_amount": 500}, nil
		},
	}

	tc := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})

	if _, _, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 60.17, Lon: 24.94},
		Venue:         venueID,
	}); err != nil {
		t.Fatalf("handleCheckoutPreview returned error: %v", err)
	}
	if assortmentSlug != "basket-venue" {
		t.Errorf("availability lookup must use the basket's own slug, got %q", assortmentSlug)
	}
}

func TestHandleCheckoutPreviewBlocksUnavailableBasketItem(t *testing.T) {
	const venueID = "5f9a1b2c3d4e5f6071829304"
	const itemID = "627cb2c7e2a6f0a1b2c3d4e5"

	checkoutCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": venueID, "currency": "GEL"}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"venue": map[string]any{"id": venueID, "slug": "test-venue"},
					"total": "GEL5.00",
					"items": []any{map[string]any{
						"id": itemID, "name": "Unavailable chicken", "count": 1, "price": 500,
					}},
				},
			}}, nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"items": []any{
				map[string]any{
					"id":                  itemID,
					"name":                "Unavailable chicken",
					"disabled_info":       map[string]any{"disable_text": "Sold out"},
					"purchasable_balance": 0,
				},
			}}, nil
		},
		checkoutPreviewFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			checkoutCalled = true
			return map[string]any{}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})

	_, _, err := tc.handleCheckoutPreview(context.Background(), nil, CheckoutPreviewInput{
		LocationInput: LocationInput{Lat: 41.7, Lon: 44.8},
		Venue:         "test-venue",
	})
	if err == nil || !strings.Contains(err.Error(), "Sold out") {
		t.Fatalf("expected Sold out validation error, got %v", err)
	}
	if checkoutCalled {
		t.Fatal("CheckoutPreview must not be called when a basket item is unavailable")
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
