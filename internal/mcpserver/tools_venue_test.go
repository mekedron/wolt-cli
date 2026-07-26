package mcpserver

import (
	"context"
	"errors"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// staticVenuePayloadStub mirrors the live static venue page: identity under
// `venue`, opening hours under `venue_raw.opening_times` as seconds since
// midnight.
func staticVenuePayloadStub() map[string]any {
	return map[string]any{
		"order_minimum": float64(1000),
		"venue": map[string]any{
			"id":               "5ae6013cf78b5a000bb64022",
			"slug":             "mcdonalds-kamppi-1",
			"name":             "McDonald's Helsinki Kamppi",
			"address":          "Fredrikinkatu 46",
			"currency":         "EUR",
			"timezone":         "Europe/Helsinki",
			"delivery_methods": []any{"homedelivery"},
			"rating":           map[string]any{"score_raw": float64(8.4)},
		},
		"venue_raw": map[string]any{
			"opening_times": map[string]any{
				"monday": []any{
					map[string]any{"type": "open", "value": float64(21600)},
					map[string]any{"type": "close", "value": float64(79200)},
				},
			},
		},
	}
}

// retiredRestaurantEndpoint reproduces the live upstream: the rich
// restaurant-api/v3/venues/<id> document answers HTTP 410 for every client.
func retiredRestaurantEndpoint() func(context.Context, string) (*domain.Restaurant, error) {
	return func(context.Context, string) (*domain.Restaurant, error) {
		return nil, errors.New("status=410; retired upstream")
	}
}

// TestHandleVenueHoursFallsBackToStaticPayload locks wolt_venue_hours against
// the retired restaurant endpoint: hours must still come back, decoded from the
// static venue page, with a warning naming the degraded source.
func TestHandleVenueHoursFallsBackToStaticPayload(t *testing.T) {
	wolt := &stubWolt{
		restaurantFn: retiredRestaurantEndpoint(),
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return staticVenuePayloadStub(), nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})

	_, out, err := tc.handleVenueHours(context.Background(), nil, VenueHoursInput{Venue: "mcdonalds-kamppi-1"})
	if err != nil {
		t.Fatalf("handleVenueHours returned error: %v", err)
	}
	windows, ok := out.Hours["opening_windows"].([]any)
	if !ok || len(windows) != 7 {
		t.Fatalf("opening_windows = %#v, want 7 weekday entries", out.Hours["opening_windows"])
	}
	monday, ok := windows[0].(map[string]any)
	if !ok || monday["open"] != "06:00" || monday["close"] != "22:00" {
		t.Fatalf("monday = %#v, want 06:00-22:00", windows[0])
	}
	if out.Hours["venue_id"] != "5ae6013cf78b5a000bb64022" {
		t.Errorf("venue_id = %#v, want the id from the static payload", out.Hours["venue_id"])
	}
	if len(out.Warnings) == 0 {
		t.Error("degraded output must carry a warning naming the fallback source")
	}
}

// TestHandleVenueDetailFallsBackToStaticPayload locks wolt_venue_detail the same
// way. The discovery-feed item is also absent here — a venue outside the
// resolved location's catalog — so this covers both lookups failing at once.
func TestHandleVenueDetailFallsBackToStaticPayload(t *testing.T) {
	wolt := &stubWolt{
		restaurantFn: retiredRestaurantEndpoint(),
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return staticVenuePayloadStub(), nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})

	_, out, err := tc.handleVenueDetail(context.Background(), nil, VenueDetailInput{
		LocationInput: LocationInput{Lat: 60.17, Lon: 24.94},
		Venue:         "mcdonalds-kamppi-1",
	})
	if err != nil {
		t.Fatalf("handleVenueDetail returned error: %v", err)
	}
	if got, want := asString(out.Venue["name"]), "McDonald's Helsinki Kamppi"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := asString(out.Venue["address"]), "Fredrikinkatu 46"; got != want {
		t.Errorf("address = %q, want %q", got, want)
	}
	if got, want := out.Venue["rating"], float64(8.4); got != want {
		t.Errorf("rating = %#v, want %#v", got, want)
	}
	if len(out.Warnings) == 0 {
		t.Error("degraded output must carry a warning naming the fallback source")
	}
}

// A bare ObjectID must reach the same result: the static venue page serves
// either identifier from its `slug` path segment.
func TestHandleVenueHoursAcceptsObjectID(t *testing.T) {
	const venueID = "5ae6013cf78b5a000bb64022"

	asked := []string{}
	wolt := &stubWolt{
		restaurantFn: retiredRestaurantEndpoint(),
		venueStaticFn: func(_ context.Context, slug string) (map[string]any, error) {
			asked = append(asked, slug)
			return staticVenuePayloadStub(), nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})

	_, out, err := tc.handleVenueHours(context.Background(), nil, VenueHoursInput{Venue: venueID})
	if err != nil {
		t.Fatalf("handleVenueHours returned error: %v", err)
	}
	if windows, ok := out.Hours["opening_windows"].([]any); !ok || len(windows) != 7 {
		t.Fatalf("opening_windows = %#v, want 7 weekday entries", out.Hours["opening_windows"])
	}
	if len(asked) == 0 || asked[0] != venueID {
		t.Errorf("expected the object id passed to the venue page, got %v", asked)
	}
}

// When the venue page is unreachable too there is nothing left to serve, so the
// tool must report a plain not-found rather than an empty success.
func TestHandleVenueHoursErrorsWhenStaticPageUnavailable(t *testing.T) {
	wolt := &stubWolt{
		restaurantFn: retiredRestaurantEndpoint(),
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("status 404")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("status 404")
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})

	if _, _, err := tc.handleVenueHours(context.Background(), nil, VenueHoursInput{Venue: "nope"}); err == nil {
		t.Fatal("expected an error when neither the restaurant document nor the venue page is available")
	}
}
