package mcpserver

import (
	"context"
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

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
