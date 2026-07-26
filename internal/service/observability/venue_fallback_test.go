package observability

import (
	"testing"
)

// buildVenueDetailFallback is the degraded path taken when the rich
// /v3/venues/<id> restaurant endpoint is unavailable (it now returns 410
// "update your app"). It must reconstruct rating, delivery methods and order
// minimum from the static venue page payload, which is the only live source.
func TestBuildVenueDetailFallbackEnrichesFromStatic(t *testing.T) {
	// Shapes mirror the live static payload: numbers decode as float64 and the
	// order minimum lives at the top level in minor units.
	staticPayload := map[string]any{
		"order_minimum": float64(1000),
		"venue": map[string]any{
			"slug":     "burger-king-finnoo",
			"name":     "Burger King Finnoo",
			"address":  "Finnoonristi 1",
			"currency": "EUR",
			"rating": map[string]any{
				"rating":    float64(3),
				"score":     "8.6",
				"score_raw": float64(8.6),
				"volume":    float64(200),
			},
			"delivery_methods": []any{"homedelivery"},
		},
	}

	data, warnings := BuildVenueDetailFallback("burger-king-finnoo", "venue-id", nil, staticPayload, nil)

	// rating must be the numeric score_raw, matching the rich path's
	// float64 restaurant.Rating.Score so table and JSON output stay identical.
	if got, want := data["rating"], float64(8.6); got != want {
		t.Fatalf("rating = %#v, want %#v", got, want)
	}

	methods, ok := data["delivery_methods"].([]any)
	if !ok || len(methods) != 1 || methods[0] != "homedelivery" {
		t.Fatalf("delivery_methods = %#v, want [homedelivery]", data["delivery_methods"])
	}

	orderMinimum, ok := data["order_minimum"].(map[string]any)
	if !ok {
		t.Fatalf("order_minimum = %#v, want map", data["order_minimum"])
	}
	if got, want := orderMinimum["amount"], 1000; got != want {
		t.Fatalf("order_minimum.amount = %#v, want %#v", got, want)
	}
	if got, want := orderMinimum["formatted_amount"], "€10.00"; got != want {
		t.Fatalf("order_minimum.formatted_amount = %#v, want %#v", got, want)
	}

	// The "order minimum unavailable" warning would contradict a populated
	// order minimum, so it must be dropped once we resolve it.
	for _, w := range warnings {
		if w == "order minimum is unavailable in basic mode and returned as null" {
			t.Fatalf("order-minimum-unavailable warning present despite resolved order minimum: %#v", warnings)
		}
	}
}

// When the static payload carries no order minimum we must fall back to null
// and re-surface the warning, rather than emitting a misleading zero.
func TestBuildVenueDetailFallbackMissingOrderMinimum(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"slug":     "burger-king-finnoo",
			"currency": "EUR",
		},
	}

	data, warnings := BuildVenueDetailFallback("burger-king-finnoo", "venue-id", nil, staticPayload, nil)

	orderMinimum, ok := data["order_minimum"].(map[string]any)
	if !ok {
		t.Fatalf("order_minimum = %#v, want map", data["order_minimum"])
	}
	if orderMinimum["amount"] != nil || orderMinimum["formatted_amount"] != nil {
		t.Fatalf("order_minimum = %#v, want nil amount/formatted", orderMinimum)
	}

	found := false
	for _, w := range warnings {
		if w == "order minimum is unavailable in basic mode and returned as null" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing order-minimum-unavailable warning: %#v", warnings)
	}
}

// The static venue page is the only live source of opening hours, so the hours
// fallback must decode its seconds-since-midnight entries into the same
// weekday-ordered windows the rich builder produces.
func TestBuildVenueHoursFallbackDecodesStaticOpeningTimes(t *testing.T) {
	staticPayload := map[string]any{
		"venue_raw": map[string]any{
			"opening_times": map[string]any{
				"monday": []any{
					map[string]any{"type": "open", "value": float64(21600)},  // 06:00
					map[string]any{"type": "close", "value": float64(79200)}, // 22:00
				},
			},
		},
	}

	data, warnings := BuildVenueHoursFallback("venue-id", "Europe/Helsinki", staticPayload)

	if got, want := data["timezone"], "Europe/Helsinki"; got != want {
		t.Fatalf("timezone = %#v, want %#v", got, want)
	}
	windows, ok := data["opening_windows"].([]any)
	if !ok || len(windows) != 7 {
		t.Fatalf("opening_windows = %#v, want 7 weekday entries", data["opening_windows"])
	}
	monday, ok := windows[0].(map[string]any)
	if !ok {
		t.Fatalf("windows[0] = %#v, want map", windows[0])
	}
	if monday["day"] != "monday" || monday["open"] != "06:00" || monday["close"] != "22:00" {
		t.Fatalf("monday = %#v, want 06:00-22:00", monday)
	}
	if len(warnings) == 0 {
		t.Fatal("fallback mode must warn that hours came from the static payload")
	}
}

// No opening_times at all is "hours unknown", not "closed all week" — an empty
// slice keeps consumers from rendering seven bogus closed days.
func TestBuildVenueHoursFallbackWithoutOpeningTimes(t *testing.T) {
	data, warnings := BuildVenueHoursFallback("venue-id", "", map[string]any{})

	windows, ok := data["opening_windows"].([]any)
	if !ok || len(windows) != 0 {
		t.Fatalf("opening_windows = %#v, want empty", data["opening_windows"])
	}
	if got, want := data["timezone"], "UTC"; got != want {
		t.Fatalf("timezone = %#v, want %#v", got, want)
	}
	found := false
	for _, w := range warnings {
		if w == "restaurant detail endpoint unavailable; opening hours are unavailable in fallback mode" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing hours-unavailable warning: %#v", warnings)
	}
}
