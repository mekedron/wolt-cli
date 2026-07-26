package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

func TestVenueItemAvailabilityDistinguishesOmissionAndTransportFailure(t *testing.T) {
	const (
		venueID   = "000000000000000000000411"
		venueSlug = "synthetic-market"
		itemID    = "000000000000000000000412"
	)
	tests := []struct {
		name            string
		currentPayload  map[string]any
		currentErr      error
		wantVerified    bool
		wantAvailable   any
		wantReason      any
		wantWarningText string
	}{
		{
			name:           "exact omission",
			currentPayload: map[string]any{"items": []any{}},
			wantVerified:   true,
			wantAvailable:  false,
			wantReason:     "item is missing from the current assortment",
		},
		{
			name:            "exact transport failure",
			currentErr:      errors.New("exact item endpoint unavailable"),
			wantAvailable:   nil,
			wantReason:      nil,
			wantWarningText: "current item endpoint unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &testWoltAPI{
				venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{
						"venue": map[string]any{"id": venueID, "slug": venueSlug, "currency": "EUR"},
					}, nil
				},
				assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{
						"currency": "EUR",
						"items": []any{
							map[string]any{"id": itemID, "name": "Older item", "price": 500},
						},
					}, nil
				},
				assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
					return test.currentPayload, test.currentErr
				},
				venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
					return map[string]any{
						"id":          itemID,
						"name":        "Older item",
						"description": "Preserved page metadata",
						"price":       500,
					}, nil
				},
			}
			resolvedID, payload, verified, warnings := resolveVenueItemPayloadBySlug(
				context.Background(),
				Dependencies{Wolt: api},
				venueID,
				venueSlug,
				itemID,
				woltgateway.AuthContext{},
			)
			if resolvedID != venueID || verified != test.wantVerified {
				t.Fatalf("resolved ID=%q verified=%v warnings=%v", resolvedID, verified, warnings)
			}
			if test.wantWarningText != "" {
				found := false
				for _, warning := range warnings {
					if strings.Contains(warning, test.wantWarningText) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("warnings = %v, want %q", warnings, test.wantWarningText)
				}
			}
			data, _ := observability.BuildItemDetail(
				itemID,
				resolvedID,
				payload,
				false,
				observability.ItemVenueContext{
					VenueID:              venueID,
					VenueSlug:            venueSlug,
					AvailabilityVerified: &verified,
				},
			)
			if data["availability_verified"] != test.wantVerified ||
				data["is_available"] != test.wantAvailable ||
				data["unavailable_reason"] != test.wantReason {
				t.Fatalf("availability = %#v", data)
			}
			if data["description"] != "Preserved page metadata" {
				t.Fatalf("page metadata was not preserved: %#v", data)
			}
		})
	}
}
