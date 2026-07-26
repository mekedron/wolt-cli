package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestCheckoutPreviewHydratesTopLevelBasketVenueIdentity(t *testing.T) {
	const (
		venueID   = "000000000000000000000071"
		venueSlug = "example-market"
		itemID    = "000000000000000000000072"
	)
	tests := []struct {
		name       string
		selection  string
		basketData map[string]any
	}{
		{
			name:      "id only",
			selection: venueID,
			basketData: map[string]any{
				"venue_id": venueID,
			},
		},
		{
			name:      "slug only",
			selection: venueSlug,
			basketData: map[string]any{
				"venue_slug": venueSlug,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			basket := map[string]any{
				"id":       "basket-1",
				"currency": "EUR",
				"items": []any{
					map[string]any{
						"id":          itemID,
						"count":       1,
						"price":       500,
						"category_id": "drinks",
					},
				},
			}
			for key, value := range test.basketData {
				basket[key] = value
			}
			var captured map[string]any
			api := &testWoltAPI{
				venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{
						"venue": map[string]any{
							"id":       venueID,
							"slug":     venueSlug,
							"currency": "EUR",
							"country":  "ZZZ",
						},
					}, nil
				},
				basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{"baskets": []any{basket}}, nil
				},
				checkoutPreviewFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
					captured = payload
					return map[string]any{
						"payable_amount": 500,
						"delivery_configs": []any{
							map[string]any{"type": "standard", "selected": true},
						},
					}, nil
				},
			}
			cmd := newCheckoutPreviewCommand(Dependencies{
				Wolt: api,
				Profiles: &testProfiles{profile: domain.Profile{
					Name:      "default",
					IsDefault: true,
					Location:  domain.Location{Lat: 10, Lon: 20},
					WToken:    "test-token",
				}},
			})
			output := &bytes.Buffer{}
			cmd.SetOut(output)
			cmd.SetErr(output)
			cmd.SetArgs([]string{"--venue-id", test.selection, "--format", "json"})

			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("checkout preview: %v\n%s", err, output.String())
			}
			plan := asMap(captured["purchase_plan"])
			venue := asMap(plan["venue"])
			if asString(venue["id"]) != venueID {
				t.Fatalf("purchase_plan venue = %#v", venue)
			}
			hydratedVenue := asMap(basket["venue"])
			if asString(hydratedVenue["id"]) != venueID || asString(hydratedVenue["slug"]) != venueSlug {
				t.Fatalf("hydrated basket venue = %#v", hydratedVenue)
			}
			item := asMap(asSlice(plan["menu_items"])[0])
			if asString(item["venue_id"]) != venueID {
				t.Fatalf("menu item venue_id = %#v", item["venue_id"])
			}
		})
	}
}
