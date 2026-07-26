package mcpserver

import (
	"context"
	"errors"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestMCPVenueItemSlugOnlyFallbackDoesNotCallIDEndpoint(t *testing.T) {
	const (
		venueSlug = "synthetic-market"
		itemID    = "000000000000000000000501"
	)
	itemPageCalls := 0
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"slug": venueSlug, "currency": "EUR"},
			}, nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"items": []any{
					map[string]any{
						"id":                  itemID,
						"name":                "Configurable item",
						"price":               500,
						"purchasable_balance": 5,
						"options": []any{
							map[string]any{
								"id":   "size",
								"name": "Size",
								"values": []any{
									map[string]any{"id": "large", "name": "Large", "price": 125},
								},
							},
						},
					},
				},
			}, nil
		},
		venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
			itemPageCalls++
			return nil, errors.New("must not call ID endpoint without a verified ID")
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueItem(context.Background(), nil, VenueItemInput{
		Venue:  venueSlug,
		ItemID: itemID,
	})
	if err != nil {
		t.Fatalf("handleVenueItem: %v", err)
	}
	if itemPageCalls != 0 || out.Item["venue_id"] != "" || out.Item["venue_slug"] != venueSlug {
		t.Fatalf("slug-only item output = %#v, calls=%d", out, itemPageCalls)
	}
	groups := asSlice(out.Item["option_groups"])
	if len(groups) != 1 || len(asSlice(asMap(groups[0])["values"])) != 1 {
		t.Fatalf("rich option metadata = %#v", groups)
	}
}

func TestMCPVenueItemAvailabilityDistinguishesOmissionAndTransportFailure(t *testing.T) {
	const (
		venueID   = "000000000000000000000511"
		venueSlug = "synthetic-market"
		itemID    = "000000000000000000000512"
	)
	tests := []struct {
		name           string
		currentPayload map[string]any
		currentErr     error
		wantVerified   bool
		wantAvailable  any
		wantReason     any
		wantWarning    string
	}{
		{
			name:           "exact omission",
			currentPayload: map[string]any{"items": []any{}},
			wantVerified:   true,
			wantAvailable:  false,
			wantReason:     "item is missing from the current assortment",
			wantWarning:    "missing from the current assortment",
		},
		{
			name:          "exact transport failure",
			currentErr:    errors.New("exact item endpoint unavailable"),
			wantAvailable: nil,
			wantReason:    nil,
			wantWarning:   "could not be verified",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{
						"venue": map[string]any{"id": venueID, "slug": venueSlug, "currency": "EUR"},
					}, nil
				},
				venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
					return map[string]any{
						"id":          itemID,
						"name":        "Older item",
						"description": "Preserved page metadata",
						"price":       500,
					}, nil
				},
				assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
					return test.currentPayload, test.currentErr
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})
			_, out, err := tc.handleVenueItem(context.Background(), nil, VenueItemInput{
				Venue:  venueID,
				ItemID: itemID,
			})
			if err != nil {
				t.Fatalf("handleVenueItem: %v", err)
			}
			if out.Item["availability_verified"] != test.wantVerified ||
				out.Item["is_available"] != test.wantAvailable ||
				out.Item["unavailable_reason"] != test.wantReason {
				t.Fatalf("availability output = %#v", out)
			}
			if out.Item["description"] != "Preserved page metadata" ||
				!containsWarning(out.Warnings, test.wantWarning) {
				t.Fatalf("metadata/warnings output = %#v", out)
			}
		})
	}
}
