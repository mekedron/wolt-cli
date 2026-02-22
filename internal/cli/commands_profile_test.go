package cli

import "testing"

func TestExtractPaymentMethodsFromNestedResultsCards(t *testing.T) {
	payload := map[string]any{
		"results": map[string]any{
			"cards": []any{
				map[string]any{
					"id":                        "card-1",
					"payment_method_type":       "card",
					"card_brand":                "visa",
					"card_last_four":            "1234",
					"is_available_for_checkout": true,
					"is_default":                true,
				},
			},
		},
	}

	methods := extractPaymentMethods(payload, false)
	if len(methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(methods))
	}
	method := asMap(methods[0])
	if method["method_id"] != "card-1" {
		t.Fatalf("unexpected method id: %v", method["method_id"])
	}
	if method["type"] != "card" {
		t.Fatalf("unexpected type: %v", method["type"])
	}
	if method["label"] != "visa" {
		t.Fatalf("unexpected label: %v", method["label"])
	}
}

func TestExtractPaymentMethodsFromPaymentProfileElements(t *testing.T) {
	payload := map[string]any{
		"profile": map[string]any{
			"root_element": map[string]any{
				"children": []any{
					map[string]any{
						"children": []any{
							map[string]any{
								"element_type": "payment-method",
								"id":           "method-revolut",
								"type":         "card",
								"title":        "Revolut Personal",
								"is_default":   true,
							},
						},
					},
				},
			},
		},
		"saved": map[string]any{
			"results": map[string]any{
				"cards": []any{
					map[string]any{
						"id":                  "method-revolut",
						"payment_method_type": "card",
						"card_last_four":      "7890",
					},
				},
			},
		},
	}

	methods := extractPaymentMethods(payload, false)
	if len(methods) != 1 {
		t.Fatalf("expected deduped method list with 1 item, got %d", len(methods))
	}
	method := asMap(methods[0])
	if method["method_id"] != "method-revolut" {
		t.Fatalf("unexpected method id: %v", method["method_id"])
	}
	if method["label"] != "Revolut Personal" {
		t.Fatalf("unexpected label: %v", method["label"])
	}
}

func TestFilterPaymentMethodsByLabel(t *testing.T) {
	methods := []any{
		map[string]any{"label": "Revolut Personal"},
		map[string]any{"label": "Edenred"},
	}

	filtered := filterPaymentMethodsByLabel(methods, "revolut")
	if len(filtered) != 1 {
		t.Fatalf("expected one method after filter, got %d", len(filtered))
	}
	if asMap(filtered[0])["label"] != "Revolut Personal" {
		t.Fatalf("unexpected filtered label: %v", asMap(filtered[0])["label"])
	}
}

func TestPaymentCountryFromToken(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyIjp7ImNvdW50cnkiOiJGSU4ifX0.sig"
	if got := paymentCountryFromToken(token); got != "FIN" {
		t.Fatalf("expected FIN, got %q", got)
	}
}

func TestBuildDeliveryInfoPayload(t *testing.T) {
	payload, err := buildDeliveryInfoPayload("Test street", 60.1, 24.9, "other", []string{"other_address_details=door A"}, "home", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	location := asMap(payload["location"])
	if location["location_type"] != "other" {
		t.Fatalf("unexpected location_type: %v", location["location_type"])
	}
	if payload["label_type"] != "home" {
		t.Fatalf("unexpected label_type: %v", payload["label_type"])
	}
	coords := asMap(location["user_coordinates"])
	if coords["type"] != "Point" {
		t.Fatalf("unexpected geometry type: %v", coords["type"])
	}
	values := asSlice(coords["coordinates"])
	if len(values) != 2 {
		t.Fatalf("expected 2 coordinates, got %d", len(values))
	}
}

func TestBuildAddressMapLinks(t *testing.T) {
	entry := map[string]any{
		"location": map[string]any{
			"address": "Iivisniemenkatu 2 B 36, 02260 Espoo",
			"address_form_data": map[string]any{
				"other_address_details":   "Entrance B, Apartment 36",
				"additional_instructions": "Door code 4806#; right door on floor #6",
			},
			"user_coordinates": map[string]any{
				"type":        "Point",
				"coordinates": []any{24.6911577, 60.14889},
			},
		},
	}

	links := buildAddressMapLinks(entry)
	if asString(links["address_link"]) == "" {
		t.Fatalf("expected non-empty address_link")
	}
	if asString(links["entrance_link"]) == "" {
		t.Fatalf("expected non-empty entrance_link")
	}
	if asString(links["coordinates_link"]) == "" {
		t.Fatalf("expected non-empty coordinates_link")
	}
}

func TestVenueSlugFromInput(t *testing.T) {
	cases := map[string]string{
		"https://wolt.com/en/fin/espoo/restaurant/rioni-espoo": "rioni-espoo",
		"/en/fin/espoo/restaurant/rioni-espoo":                 "rioni-espoo",
		"rioni-espoo":                                          "rioni-espoo",
	}
	for input, expected := range cases {
		if got := venueSlugFromInput(input); got != expected {
			t.Fatalf("expected slug %q from %q, got %q", expected, input, got)
		}
	}
}

func TestExtractFavoriteVenues(t *testing.T) {
	payload := map[string]any{
		"sections": []any{
			map[string]any{
				"items": []any{
					map[string]any{
						"title": "Rioni Espoo",
						"link":  map[string]any{"target": "https://wolt.com/en/fin/espoo/restaurant/rioni-espoo"},
						"venue": map[string]any{
							"id":        "5a8426f188b5de000b8857bb",
							"slug":      "rioni-espoo",
							"name":      "Rioni Espoo",
							"address":   "Espoonlahdenkatu 8",
							"favourite": true,
							"rating":    map[string]any{"score": 9.0},
						},
					},
				},
			},
		},
	}

	rows := extractFavoriteVenues(payload)
	if len(rows) != 1 {
		t.Fatalf("expected one favorite venue, got %d", len(rows))
	}
	row := asMap(rows[0])
	if row["venue_id"] != "5a8426f188b5de000b8857bb" {
		t.Fatalf("unexpected venue_id: %v", row["venue_id"])
	}
	if row["slug"] != "rioni-espoo" {
		t.Fatalf("unexpected slug: %v", row["slug"])
	}
	if row["name"] != "Rioni Espoo" {
		t.Fatalf("unexpected name: %v", row["name"])
	}
	if !asBool(row["is_favorite"]) {
		t.Fatalf("expected is_favorite=true, got %v", row["is_favorite"])
	}
}
