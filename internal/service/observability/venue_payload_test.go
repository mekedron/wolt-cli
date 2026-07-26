package observability

import (
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

const (
	venuePayloadTestID       = "000000000000000000000001"
	venuePayloadTestSlug     = "scheduled-market"
	venuePayloadTestName     = "Scheduled Market"
	venuePayloadTestURL      = "https://wolt.com/en/test/venue/scheduled-market"
	venuePayloadTestCurrency = "EUR"
	venuePayloadTestTimezone = "Etc/UTC"
	venuePayloadTestLat      = 10.25
	venuePayloadTestLon      = 20.5
	venuePayloadOpeningAt    = "2030-01-15T10:00:00Z"
)

func TestExtractVenueSearchCandidatesIncludesScheduledClosedVenue(t *testing.T) {
	payload := map[string]any{
		"sections": []any{
			map[string]any{
				"name": "venues",
				"items": []any{
					map[string]any{
						"title": venuePayloadTestName,
						"link": map[string]any{
							"target": venuePayloadTestURL,
						},
						"overlay_v2": map[string]any{
							"primary_text":     "Schedule order",
							"telemetry_status": "scheduled_order__without_time",
						},
						"venue": map[string]any{
							"id":       venuePayloadTestID,
							"slug":     venuePayloadTestSlug,
							"name":     venuePayloadTestName,
							"address":  "100 Example Street",
							"currency": venuePayloadTestCurrency,
							"online":   false,
							"delivers": true,
							"status": map[string]any{
								"primary_text":     "Schedule order",
								"telemetry_status": "scheduled_order__without_time",
								"next_open":        venuePayloadOpeningAt,
							},
						},
					},
				},
			},
		},
	}

	got := ExtractVenueSearchCandidates(payload)
	if len(got) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(got))
	}
	row := got[0]
	if row["venue_id"] != venuePayloadTestID || row["slug"] != venuePayloadTestSlug {
		t.Fatalf("unexpected identity: %#v", row)
	}
	if row["canonical_url"] != venuePayloadTestURL {
		t.Fatalf("canonical_url = %v", row["canonical_url"])
	}
	if row["order_now_available"] != false || row["scheduled_order_available"] != true {
		t.Fatalf("unexpected availability: %#v", row)
	}
	if row["scheduled_pickup_available"] != nil ||
		row["delivers_to_location"] != true ||
		row["next_opening_at"] != venuePayloadOpeningAt {
		t.Fatalf("unexpected optional availability: %#v", row)
	}
}

func TestExtractVenueSearchCandidatesPreservesUnknownAvailability(t *testing.T) {
	tests := []struct {
		name          string
		status        map[string]any
		wantTelemetry string
	}{
		{name: "missing status"},
		{
			name: "unknown telemetry status",
			status: map[string]any{
				"telemetry_status": "future_availability_state",
			},
			wantTelemetry: "future_availability_state",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			venue := map[string]any{
				"id":   "venue-1",
				"slug": "venue-without-known-availability",
			}
			if test.status != nil {
				venue["status"] = test.status
			}
			payload := map[string]any{
				"sections": []any{
					map[string]any{
						"items": []any{
							map[string]any{"venue": venue},
						},
					},
				},
			}

			got := ExtractVenueSearchCandidates(payload)
			if len(got) != 1 {
				t.Fatalf("candidate count = %d, want 1", len(got))
			}
			for _, key := range []string{
				"order_now_available",
				"scheduled_order_available",
				"scheduled_pickup_available",
			} {
				if got[0][key] != nil {
					t.Fatalf("%s = %#v, want nil without a known upstream signal", key, got[0][key])
				}
			}
			if test.wantTelemetry != "" && got[0]["telemetry_status"] != test.wantTelemetry {
				t.Fatalf("telemetry_status = %#v, want %#v", got[0]["telemetry_status"], test.wantTelemetry)
			}
		})
	}
}

func TestExtractVenueSearchCandidatesNormalizesOnlyVenueURLs(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "nested venue URL",
			target: "http://wolt.com/en/test/venue/example-market/categories/fresh?source=test#details",
			want:   "https://wolt.com/en/test/venue/example-market",
		},
		{
			name:   "external URL",
			target: "https://example.com/en/test/venue/example-market",
		},
		{
			name:   "non-venue Wolt URL",
			target: "https://wolt.com/en/test/discovery/example-market",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := map[string]any{
				"sections": []any{
					map[string]any{
						"items": []any{
							map[string]any{
								"link": map[string]any{"target": test.target},
								"venue": map[string]any{
									"id":   "venue-1",
									"slug": "example-market",
								},
							},
						},
					},
				},
			}
			got := ExtractVenueSearchCandidates(payload)
			if len(got) != 1 {
				t.Fatalf("candidate count = %d, want 1", len(got))
			}
			if got[0]["canonical_url"] != test.want {
				t.Fatalf("canonical_url = %#v, want %q", got[0]["canonical_url"], test.want)
			}
		})
	}
}

func TestBuildVenueDetailFromPayloadSeparatesImmediateAndScheduledAvailability(t *testing.T) {
	staticPayload := testStaticVenuePayload()
	dynamicPayload := map[string]any{
		"venue": map[string]any{
			"id": venuePayloadTestID,
			"delivery_open_status": map[string]any{
				"is_open":   false,
				"next_open": venuePayloadOpeningAt,
				"value":     "Next opening at 10:00",
			},
			"open_status": map[string]any{
				"is_open": false,
			},
			"delivery_configs": []any{
				map[string]any{
					"method":   "homedelivery",
					"schedule": "time_slot",
					"tso_schedule": []any{
						map[string]any{
							"time_slots": []any{map[string]any{}},
						},
					},
				},
			},
		},
		"venue_raw": map[string]any{
			"delivery_specs": map[string]any{
				"order_minimum_no_surcharge": 2000,
				"order_minimum_possible":     0,
				"original_delivery_price":    309,
			},
			"discounts": []any{
				map[string]any{
					"id": "campaign-1",
					"description": map[string]any{
						"title": "12 EUR off",
					},
					"conditions": map[string]any{
						"delivery_methods": []any{"homedelivery"},
					},
					"effects": map[string]any{
						"order_discount": map[string]any{"amount": 1200},
					},
				},
			},
		},
	}

	data, warnings, err := BuildVenueDetailFromPayload(
		VenueIdentity{},
		staticPayload,
		dynamicPayload,
		nil,
		&domain.Location{Lat: venuePayloadTestLat, Lon: venuePayloadTestLon},
		map[string]struct{}{"hours": {}, "fees": {}, "promotions": {}},
	)
	if err != nil {
		t.Fatalf("BuildVenueDetailFromPayload: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if data["venue_id"] != venuePayloadTestID || data["slug"] != venuePayloadTestSlug {
		t.Fatalf("unexpected identity: %#v", data)
	}
	if data["timezone"] != venuePayloadTestTimezone {
		t.Fatalf("timezone = %v", data["timezone"])
	}
	availability := data["availability"].(map[string]any)
	if availability["order_now_available"] != false {
		t.Fatalf("order_now_available = %v", availability["order_now_available"])
	}
	if availability["scheduled_order_available"] != true || availability["scheduled_only"] != true {
		t.Fatalf("scheduled availability = %#v", availability)
	}
	if availability["delivers_to_location"] != true {
		t.Fatalf("delivers_to_location = %v", availability["delivers_to_location"])
	}
	if availability["next_opening_at"] != venuePayloadOpeningAt {
		t.Fatalf("next_opening_at = %v", availability["next_opening_at"])
	}
	if got := len(data["opening_windows"].([]map[string]string)); got != 7 {
		t.Fatalf("opening window count = %d, want 7", got)
	}
	deliveryFee := data["delivery_fee"].(map[string]any)
	if deliveryFee["amount"] != 309 || deliveryFee["currency"] != venuePayloadTestCurrency {
		t.Fatalf("delivery_fee = %#v", deliveryFee)
	}
	feeDetails := data["fee_details"].(map[string]any)
	for field, want := range map[string]int{
		"order_minimum":                   2000,
		"order_minimum_possible":          0,
		"order_minimum_without_surcharge": 2000,
	} {
		fee := feeDetails[field].(map[string]any)
		if fee["amount"] != want || fee["currency"] != venuePayloadTestCurrency {
			t.Fatalf("%s = %#v", field, fee)
		}
	}
	promotions := data["promotions"].([]map[string]any)
	if len(promotions) != 1 || promotions[0]["conditions_available"] != true {
		t.Fatalf("promotions = %#v", promotions)
	}
}

func TestBuildVenueDetailFromPayloadFallsBackToDynamicVenueFields(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id":               venuePayloadTestID,
			"slug":             venuePayloadTestSlug,
			"delivery_methods": []any{},
		},
	}
	dynamicPayload := map[string]any{
		"venue": map[string]any{
			"id":      venuePayloadTestID,
			"slug":    venuePayloadTestSlug,
			"address": "200 Dynamic Street",
		},
		"venue_raw": map[string]any{
			"currency":         "EUR",
			"delivery_methods": []any{"homedelivery"},
		},
	}

	data, _, err := BuildVenueDetailFromPayload(
		VenueIdentity{},
		staticPayload,
		dynamicPayload,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildVenueDetailFromPayload: %v", err)
	}
	if data["address"] != "200 Dynamic Street" || data["currency"] != "EUR" {
		t.Fatalf("dynamic venue fields = %#v", data)
	}
	methods := data["delivery_methods"].([]string)
	if len(methods) != 1 || methods[0] != "homedelivery" {
		t.Fatalf("delivery_methods = %#v, want dynamic homedelivery", methods)
	}
}

func TestBuildVenueAvailabilityDoesNotTreatPhysicalOpeningAsOrderNow(t *testing.T) {
	availability := BuildVenueAvailability(
		nil,
		map[string]any{
			"venue": map[string]any{
				"open_status": map[string]any{"is_open": true},
			},
		},
		nil,
		nil,
	)
	if availability["order_now_available"] != nil {
		t.Fatalf("order_now_available = %#v, want unknown", availability["order_now_available"])
	}
	if availability["store_open_now"] != true {
		t.Fatalf("store_open_now = %#v, want true", availability["store_open_now"])
	}
	for _, key := range []string{
		"next_opening_at",
		"next_closing_at",
		"status_text",
		"telemetry_status",
	} {
		if availability[key] != nil {
			t.Fatalf("%s = %#v, want nil", key, availability[key])
		}
	}
}

func TestBuildVenueAvailabilityUsesDynamicOnlineSignal(t *testing.T) {
	availability := BuildVenueAvailability(
		nil,
		map[string]any{
			"venue": map[string]any{"online": false},
		},
		map[string]any{"order_now_available": true},
		nil,
	)
	if availability["order_now_available"] != false {
		t.Fatalf("order_now_available = %#v, want exact dynamic online signal", availability["order_now_available"])
	}
}

func TestBuildVenueAvailabilityResolvesScheduledSignals(t *testing.T) {
	tests := []struct {
		name      string
		venue     map[string]any
		candidate map[string]any
	}{
		{
			name: "candidate with standard configuration",
			venue: map[string]any{
				"delivery_configs": []any{
					map[string]any{"method": "homedelivery", "schedule": "standard"},
				},
			},
			candidate: map[string]any{"scheduled_order_available": true},
		},
		{
			name: "time-slot configuration",
			venue: map[string]any{
				"delivery_configs": []any{
					map[string]any{
						"method":   "homedelivery",
						"schedule": "time_slot",
						"tso_schedule": []any{
							map[string]any{"time_slots": []any{map[string]any{}}},
						},
					},
				},
			},
		},
		{
			name: "scheduled-delivery call to action",
			venue: map[string]any{
				"header": map[string]any{
					"delivery_method_statuses": []any{
						map[string]any{
							"delivery_method": "DELIVERY_SCHEDULED",
							"call_to_action":  map[string]any{"enabled": true},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			availability := BuildVenueAvailability(
				nil,
				map[string]any{"venue": test.venue},
				test.candidate,
				nil,
			)
			if availability["scheduled_order_available"] != true {
				t.Fatalf("availability = %#v, want scheduled ordering", availability)
			}
		})
	}
}

func TestBuildVenueAvailabilityUsesScheduledDeliveryAsServiceAreaEvidence(t *testing.T) {
	location := &domain.Location{Lat: venuePayloadTestLat, Lon: venuePayloadTestLon}
	tests := []struct {
		name      string
		venue     map[string]any
		candidate map[string]any
	}{
		{
			name: "dynamic scheduled slot",
			venue: map[string]any{
				"online": false,
				"delivery_configs": []any{
					map[string]any{
						"method":   "homedelivery",
						"schedule": "time_slot",
						"tso_schedule": []any{
							map[string]any{"time_slots": []any{map[string]any{}}},
						},
					},
				},
			},
			candidate: map[string]any{"delivers_to_location": false},
		},
		{
			name:  "search scheduled signal",
			venue: map[string]any{"online": false},
			candidate: map[string]any{
				"delivers_to_location":      false,
				"scheduled_order_available": true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			availability := BuildVenueAvailability(
				nil,
				map[string]any{"venue": test.venue},
				test.candidate,
				location,
			)
			if availability["delivers_to_location"] != true ||
				availability["scheduled_order_available"] != true ||
				availability["scheduled_only"] != true {
				t.Fatalf("availability = %#v, want scheduled-only home delivery at this location", availability)
			}
		})
	}
}

func TestBuildVenueAvailabilityRequiresLocationForDeliveryAreaEvidence(t *testing.T) {
	availability := BuildVenueAvailability(
		nil,
		map[string]any{
			"venue": map[string]any{
				"delivers": true,
				"delivery_configs": []any{
					map[string]any{
						"method":   "homedelivery",
						"schedule": "time_slot",
						"tso_schedule": []any{
							map[string]any{"time_slots": []any{map[string]any{}}},
						},
					},
				},
			},
		},
		map[string]any{"delivers_to_location": true},
		nil,
	)
	if availability["delivers_to_location"] != nil {
		t.Fatalf("delivers_to_location = %#v, want unknown without a location", availability["delivers_to_location"])
	}
}

func TestBuildVenueAvailabilityPreservesExactDeliveryAndOpeningSignals(t *testing.T) {
	staticPayload := testStaticVenuePayload()
	location := &domain.Location{Lat: venuePayloadTestLat, Lon: venuePayloadTestLon}
	candidate := map[string]any{
		"delivers_to_location": true,
		"next_opening_at":      "2030-02-01T10:00:00Z",
	}

	dynamic := BuildVenueAvailability(
		staticPayload,
		map[string]any{
			"venue": map[string]any{
				"delivers": false,
				"delivery_open_status": map[string]any{
					"next_open": "2030-01-20T10:00:00Z",
				},
			},
		},
		candidate,
		location,
	)
	if dynamic["delivers_to_location"] != false ||
		dynamic["next_opening_at"] != "2030-01-20T10:00:00Z" {
		t.Fatalf("dynamic precedence = %#v", dynamic)
	}

	candidate["delivers_to_location"] = false
	candidateFallback := BuildVenueAvailability(
		staticPayload,
		map[string]any{"venue": map[string]any{}},
		candidate,
		location,
	)
	if candidateFallback["delivers_to_location"] != false ||
		candidateFallback["next_opening_at"] != "2030-02-01T10:00:00Z" {
		t.Fatalf("candidate fallback = %#v", candidateFallback)
	}

	geometryFallback := BuildVenueAvailability(
		staticPayload,
		map[string]any{"venue": map[string]any{}},
		nil,
		location,
	)
	if geometryFallback["delivers_to_location"] != true {
		t.Fatalf("geometry fallback = %#v", geometryFallback)
	}
}

func TestBuildVenueHoursFromPayloadResolvesTimezone(t *testing.T) {
	tests := []struct {
		name           string
		removeTimezone bool
		requested      string
		wantTimezone   any
		warning        string
	}{
		{
			name:         "upstream venue timezone",
			wantTimezone: venuePayloadTestTimezone,
		},
		{
			name:         "conflicting override",
			requested:    "Etc/GMT+1",
			wantTimezone: venuePayloadTestTimezone,
			warning:      "was not applied",
		},
		{
			name:           "caller fallback",
			removeTimezone: true,
			requested:      "Europe/Paris",
			wantTimezone:   "Europe/Paris",
			warning:        "without validating or converting",
		},
		{
			name:           "timezone unavailable",
			removeTimezone: true,
			warning:        "remains unknown",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := testStaticVenuePayload()
			if test.removeTimezone {
				delete(payload["venue"].(map[string]any), "timezone")
			}
			data, warnings, err := BuildVenueHoursFromPayload(VenueIdentity{}, payload, test.requested)
			if err != nil {
				t.Fatalf("BuildVenueHoursFromPayload: %v", err)
			}
			if data["timezone"] != test.wantTimezone {
				t.Fatalf("timezone = %#v, want %#v", data["timezone"], test.wantTimezone)
			}
			if test.warning == "" {
				if len(warnings) != 0 {
					t.Fatalf("warnings = %v, want none", warnings)
				}
			} else if len(warnings) != 1 || !strings.Contains(warnings[0], test.warning) {
				t.Fatalf("warnings = %v, want one containing %q", warnings, test.warning)
			}
			if data["slug"] != venuePayloadTestSlug {
				t.Fatalf("slug = %v", data["slug"])
			}
			if got := len(data["opening_windows"].([]map[string]string)); got != 7 {
				t.Fatalf("opening window count = %d, want 7", got)
			}
		})
	}
}

func TestBuildVenuePromotionDetailsPreservesTextWithoutConditions(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"promotions": []any{
				map[string]any{"text": "Free delivery this weekend"},
			},
			"banners": []any{
				map[string]any{
					"discount": map[string]any{"formatted_text": "20% off selected items"},
				},
			},
		},
	}

	got := buildVenuePromotionDetails(payload)
	if len(got) != 2 {
		t.Fatalf("promotions = %#v, want two text-only promotions", got)
	}
	for _, promotion := range got {
		if strings.TrimSpace(stringFromAny(promotion["text"])) == "" {
			t.Fatalf("promotion lost source text: %#v", promotion)
		}
		if promotion["conditions_available"] != false ||
			promotion["conditions"] != nil ||
			promotion["effects"] != nil {
			t.Fatalf("text-only promotion invented structured conditions: %#v", promotion)
		}
	}
}

func TestBuildVenueHoursFromPayloadPreservesSplitShifts(t *testing.T) {
	payload := testStaticVenuePayload()
	payload["venue_raw"].(map[string]any)["opening_times"].(map[string]any)["monday"] = []any{
		map[string]any{"type": "open", "value": 28800},
		map[string]any{"type": "close", "value": 43200},
		map[string]any{"type": "open", "value": 50400},
		map[string]any{"type": "close", "value": 72000},
	}
	data, _, err := BuildVenueHoursFromPayload(VenueIdentity{}, payload, "")
	if err != nil {
		t.Fatalf("BuildVenueHoursFromPayload: %v", err)
	}
	windows := data["opening_windows"].([]map[string]string)
	monday := []map[string]string{}
	for _, window := range windows {
		if window["day"] == "monday" {
			monday = append(monday, window)
		}
	}
	if len(monday) != 2 ||
		monday[0]["open"] != "08:00" || monday[0]["close"] != "12:00" ||
		monday[1]["open"] != "14:00" || monday[1]["close"] != "20:00" {
		t.Fatalf("monday windows = %#v", monday)
	}
}

func TestDeliveryAvailabilityUsesGeoRangeInsteadOfConfigPresence(t *testing.T) {
	staticPayload := testStaticVenuePayload()
	dynamicPayload := map[string]any{
		"venue": map[string]any{
			"delivery_configs": []any{
				map[string]any{"method": "homedelivery", "schedule": "standard"},
			},
		},
	}

	inside, known := deliveryAvailableAtLocation(
		staticPayload,
		dynamicPayload,
		&domain.Location{Lat: venuePayloadTestLat, Lon: venuePayloadTestLon},
	)
	if !known || !inside {
		t.Fatalf("inside delivery range = (%v, %v), want (true, true)", inside, known)
	}
	outside, known := deliveryAvailableAtLocation(
		staticPayload,
		dynamicPayload,
		&domain.Location{Lat: 50, Lon: 50},
	)
	if !known || outside {
		t.Fatalf("outside delivery range = (%v, %v), want (false, true)", outside, known)
	}

	delete(staticPayload["venue"].(map[string]any), "delivery_geo_range")
	unknown, known := deliveryAvailableAtLocation(
		staticPayload,
		dynamicPayload,
		&domain.Location{Lat: venuePayloadTestLat, Lon: venuePayloadTestLon},
	)
	if known || unknown {
		t.Fatalf("config-only delivery = (%v, %v), want (false, false)", unknown, known)
	}
}

func TestPointInGeoRangeHandlesSupportedShapesAndMalformedInput(t *testing.T) {
	square := func(minLon float64, minLat float64, maxLon float64, maxLat float64) []any {
		return []any{
			[]any{minLon, minLat},
			[]any{maxLon, minLat},
			[]any{maxLon, maxLat},
			[]any{minLon, maxLat},
			[]any{minLon, minLat},
		}
	}
	outer := square(0, 0, 10, 10)
	hole := square(3, 3, 7, 7)

	tests := []struct {
		name       string
		geometry   map[string]any
		lon        float64
		lat        float64
		wantInside bool
		wantKnown  bool
	}{
		{
			name: "polygon hole excludes point",
			geometry: map[string]any{
				"type":        "Polygon",
				"coordinates": []any{outer, hole},
			},
			lon:       5,
			lat:       5,
			wantKnown: true,
		},
		{
			name: "multipolygon includes second polygon",
			geometry: map[string]any{
				"type": "MultiPolygon",
				"coordinates": []any{
					[]any{square(0, 0, 2, 2)},
					[]any{square(20, 20, 30, 30)},
				},
			},
			lon:        25,
			lat:        25,
			wantInside: true,
			wantKnown:  true,
		},
		{
			name: "multipolygon with outside and malformed polygons is unknown",
			geometry: map[string]any{
				"type": "MultiPolygon",
				"coordinates": []any{
					[]any{square(20, 20, 30, 30)},
					[]any{
						[]any{
							[]any{0.0, 0.0},
							[]any{1.0},
							[]any{0.0, 0.0},
						},
					},
				},
			},
			lon: 5,
			lat: 5,
		},
		{
			name: "polygon with containing outer and malformed hole is unknown",
			geometry: map[string]any{
				"type": "Polygon",
				"coordinates": []any{
					outer,
					[]any{
						[]any{3.0, 3.0},
						[]any{4.0},
						[]any{3.0, 3.0},
					},
				},
			},
			lon: 5,
			lat: 5,
		},
		{
			name: "outer boundary is included",
			geometry: map[string]any{
				"type":        "Polygon",
				"coordinates": []any{outer},
			},
			lon:        0,
			lat:        5,
			wantInside: true,
			wantKnown:  true,
		},
		{
			name: "feature unwraps geometry",
			geometry: map[string]any{
				"type": "Feature",
				"geometry": map[string]any{
					"type":        "Polygon",
					"coordinates": []any{outer},
				},
			},
			lon:        2,
			lat:        2,
			wantInside: true,
			wantKnown:  true,
		},
		{
			name: "untyped polygon is detected",
			geometry: map[string]any{
				"coordinates": []any{outer},
			},
			lon:        2,
			lat:        2,
			wantInside: true,
			wantKnown:  true,
		},
		{
			name: "malformed polygon is unknown",
			geometry: map[string]any{
				"type": "Polygon",
				"coordinates": []any{
					[]any{
						[]any{0.0, 0.0},
						[]any{1.0},
						[]any{0.0, 0.0},
					},
				},
			},
			lon: 0.5,
			lat: 0.5,
		},
		{
			name: "unsupported geometry is unknown",
			geometry: map[string]any{
				"type": "LineString",
				"coordinates": []any{
					[]any{0.0, 0.0},
					[]any{10.0, 10.0},
				},
			},
			lon: 5,
			lat: 5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inside, known := pointInGeoRange(test.lon, test.lat, test.geometry)
			if inside != test.wantInside || known != test.wantKnown {
				t.Fatalf(
					"pointInGeoRange(%v, %v) = (%v, %v), want (%v, %v)",
					test.lon,
					test.lat,
					inside,
					known,
					test.wantInside,
					test.wantKnown,
				)
			}
		})
	}
}

func testStaticVenuePayload() map[string]any {
	openingTimes := map[string]any{}
	for _, day := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		openingTimes[day] = []any{
			map[string]any{"type": "open", "value": 36000},
			map[string]any{"type": "close", "value": 72000},
		}
	}
	return map[string]any{
		"order_minimum": 2000,
		"venue": map[string]any{
			"id":               venuePayloadTestID,
			"slug":             venuePayloadTestSlug,
			"name":             venuePayloadTestName,
			"address":          "100 Example Street",
			"currency":         venuePayloadTestCurrency,
			"timezone":         venuePayloadTestTimezone,
			"delivery_methods": []any{"homedelivery", "takeaway"},
			"delivery_geo_range": map[string]any{
				"type": "Polygon",
				"coordinates": []any{
					[]any{
						[]any{20.0, 10.0},
						[]any{21.0, 10.0},
						[]any{21.0, 11.0},
						[]any{20.0, 11.0},
						[]any{20.0, 10.0},
					},
				},
			},
		},
		"venue_raw": map[string]any{
			"id":            venuePayloadTestID,
			"currency":      venuePayloadTestCurrency,
			"opening_times": openingTimes,
		},
	}
}
