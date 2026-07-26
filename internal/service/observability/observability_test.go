package observability_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

func TestBuildDiscoveryFeed(t *testing.T) {
	section := domain.Section{
		Name:  "popular",
		Title: "Popular",
		Items: []domain.Item{
			{
				Title:   "Venue One",
				TrackID: "track-1",
				Link:    domain.Link{Target: "venue-1"},
				Venue: &domain.Venue{
					ID:               "venue-1",
					Slug:             "venue-one",
					Currency:         "PLN",
					DeliveryPriceInt: intPtr(1000),
					EstimateRange:    "25-35",
					Rating:           &domain.Rating{Rating: 3, Score: 9.1},
					PriceRange:       2,
					Promotions:       []any{map[string]any{"text": "Free delivery", "variant": "discount"}},
					ShowWoltPlus:     true,
				},
			},
		},
	}

	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "Krakow", nil, false)
	sections := asSlice(t, data["sections"])
	items := asSlice(t, asMap(t, sections[0])["items"])
	firstItem := asMap(t, items[0])
	if firstItem["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", firstItem["venue_id"])
	}
	deliveryFee := asMap(t, firstItem["delivery_fee"])
	if deliveryFee["formatted_amount"] != "PLN 10.00" {
		t.Fatalf("expected fee PLN 10.00, got %v", deliveryFee["formatted_amount"])
	}
	if firstItem["price_range"] != 2 {
		t.Fatalf("expected price_range 2, got %v", firstItem["price_range"])
	}
	if firstItem["price_range_scale"] != "$$" {
		t.Fatalf("expected price_range_scale $$, got %v", firstItem["price_range_scale"])
	}
	promotions := asSlice(t, firstItem["promotions"])
	if len(promotions) != 1 || promotions[0] != "Free delivery" {
		t.Fatalf("expected promotions [Free delivery], got %v", promotions)
	}
	if firstItem["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true, got %v", firstItem["wolt_plus"])
	}
}

func TestBuildVenueSearchResultFiltersQuery(t *testing.T) {
	items := []domain.Item{
		{Title: "Burger Place", Link: domain.Link{Target: "1"}, Venue: &domain.Venue{ID: "1", Address: "Burger Street", Tags: []string{"burger"}, EstimateRange: "20-30", Currency: "PLN", DeliveryPriceInt: intPtr(500), Estimate: 25}},
		{Title: "Sushi Place", Link: domain.Link{Target: "2"}, Venue: &domain.Venue{ID: "2", Address: "Sushi Street", Tags: []string{"sushi"}, EstimateRange: "20-30", Currency: "PLN", DeliveryPriceInt: intPtr(500), Estimate: 25}},
	}

	data, _ := observability.BuildVenueSearchResult(items, "burger", observability.VenueSortRecommended, nil, "", false, false, nil, 0)
	if intValue(data["total"]) != 1 {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	rows := asSlice(t, data["items"])
	if asMap(t, rows[0])["name"] != "Burger Place" {
		t.Fatalf("expected Burger Place, got %v", asMap(t, rows[0])["name"])
	}
}

func TestDiscoveryRowsExposeScheduledOrderingSignals(t *testing.T) {
	online := false
	delivers := false
	item := domain.Item{
		Title: "Closed scheduled venue",
		Link:  domain.Link{Target: "venue-1"},
		OverlayV2: map[string]any{
			"telemetry_status": "scheduled_order__without_time",
			"primary_text":     "Next opening at 10:00",
			"next_open":        "2030-01-15T10:00:00Z",
		},
		Venue: &domain.Venue{
			ID:       "venue-1",
			Slug:     "closed-scheduled-venue",
			Online:   &online,
			Delivers: &delivers,
		},
	}

	assertAvailability := func(t *testing.T, row map[string]any) {
		t.Helper()
		if row["order_now_available"] != false ||
			row["scheduled_order_available"] != true ||
			row["scheduled_pickup_available"] != nil ||
			row["scheduled_only"] != true ||
			row["delivers_to_location"] != true {
			t.Fatalf("availability fields = %#v", row)
		}
		if row["next_opening_at"] != "2030-01-15T10:00:00Z" ||
			row["status_text"] != "Next opening at 10:00" ||
			row["telemetry_status"] != "scheduled_order__without_time" {
			t.Fatalf("status fields = %#v", row)
		}
	}

	feed := observability.BuildDiscoveryFeed(
		[]domain.Section{{Name: "scheduled", Items: []domain.Item{item}}},
		"Test area",
		nil,
		false,
	)
	sections := asSlice(t, feed["sections"])
	feedRows := asSlice(t, asMap(t, sections[0])["items"])
	assertAvailability(t, asMap(t, feedRows[0]))

	search, warnings := observability.BuildVenueSearchResult(
		[]domain.Item{item},
		"scheduled",
		observability.VenueSortRecommended,
		nil,
		"",
		false,
		false,
		nil,
		0,
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	searchRows := asSlice(t, search["items"])
	assertAvailability(t, asMap(t, searchRows[0]))

	unknown := domain.Item{
		Title: "Unknown state",
		OverlayV2: map[string]any{
			"telemetry_status": "future_availability_state",
		},
		Venue: &domain.Venue{ID: "venue-2"},
	}
	unknownSearch, _ := observability.BuildVenueSearchResult(
		[]domain.Item{unknown},
		"",
		observability.VenueSortRecommended,
		nil,
		"",
		false,
		false,
		nil,
		0,
	)
	unknownRows := asSlice(t, unknownSearch["items"])
	unknownRow := asMap(t, unknownRows[0])
	for _, key := range []string{
		"order_now_available",
		"store_open_now",
		"scheduled_order_available",
		"scheduled_pickup_available",
		"scheduled_only",
		"delivers_to_location",
	} {
		if unknownRow[key] != nil {
			t.Fatalf("%s = %#v, want nil for unknown state", key, unknownRow[key])
		}
	}
	if unknownRow["telemetry_status"] != "future_availability_state" {
		t.Fatalf("telemetry_status = %#v, want preserved upstream value", unknownRow["telemetry_status"])
	}
}

func TestAscendingVenueSortsPlaceUnknownMetricsLastStably(t *testing.T) {
	estimateItems := []domain.Item{
		{Title: "Unknown A", Venue: &domain.Venue{ID: "unknown-a"}},
		{Title: "Slow", Venue: &domain.Venue{ID: "slow", Estimate: 35}},
		{Title: "Unknown B", Venue: &domain.Venue{ID: "unknown-b"}},
		{Title: "Fast", Venue: &domain.Venue{ID: "fast", Estimate: 15}},
	}
	feeItems := []domain.Item{
		{Title: "Unknown A", Venue: &domain.Venue{ID: "unknown-a"}},
		{Title: "Paid", Venue: &domain.Venue{ID: "paid", DeliveryPriceInt: intPtr(300)}},
		{Title: "Unknown B", Venue: &domain.Venue{ID: "unknown-b"}},
		{Title: "Free", Venue: &domain.Venue{ID: "free", DeliveryPriceInt: intPtr(0)}},
	}
	for _, test := range []struct {
		name  string
		sort  observability.VenueSort
		items []domain.Item
		want  []string
	}{
		{
			name:  "distance",
			sort:  observability.VenueSortDistance,
			items: estimateItems,
			want:  []string{"Fast", "Slow", "Unknown A", "Unknown B"},
		},
		{
			name:  "delivery time",
			sort:  observability.VenueSortDeliveryTime,
			items: estimateItems,
			want:  []string{"Fast", "Slow", "Unknown A", "Unknown B"},
		},
		{
			name:  "delivery fee",
			sort:  observability.VenueSortDeliveryPrice,
			items: feeItems,
			want:  []string{"Free", "Paid", "Unknown A", "Unknown B"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, _ := observability.BuildVenueSearchResult(
				test.items,
				"",
				test.sort,
				nil,
				"",
				false,
				false,
				nil,
				0,
			)
			rows := asSlice(t, data["items"])
			if len(rows) != len(test.want) {
				t.Fatalf("rows = %#v", rows)
			}
			for index, want := range test.want {
				if got := asMap(t, rows[index])["name"]; got != want {
					t.Fatalf("row %d = %v, want %s", index, got, want)
				}
			}
		})
	}
}

func TestBuildItemSearchResultFallback(t *testing.T) {
	fallback := []domain.Item{
		{
			Title:   "Whopper Meal",
			TrackID: "item-track-1",
			Link:    domain.Link{Target: "venue-1"},
			Venue: &domain.Venue{
				ID:       "venue-1",
				Slug:     "burger-place",
				Currency: "PLN",
			},
		},
	}

	data, warnings := observability.BuildItemSearchResult(
		"whopper",
		nil,
		observability.ItemSortRelevance,
		"",
		nil,
		0,
		fallback,
		observability.ItemVenueContext{},
	)
	if intValue(data["total"]) != 1 {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected fallback warning")
	}
}

func TestBuildItemSearchResultNormalizesBasePrice(t *testing.T) {
	payloads := []map[string]any{
		{
			"venue": map[string]any{
				"currency": "EUR",
			},
			"items": []any{
				map[string]any{
					"id":    "item-1",
					"name":  "Coca-Cola Zero 6-pack",
					"price": 419,
				},
			},
		},
	}

	data, warnings := observability.BuildItemSearchResult(
		"coca",
		payloads,
		observability.ItemSortRelevance,
		"",
		nil,
		0,
		nil,
		observability.ItemVenueContext{},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	items := asSlice(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	first := asMap(t, items[0])
	basePrice := asMap(t, first["base_price"])
	if basePrice["currency"] != "EUR" {
		t.Fatalf("expected base_price currency EUR, got %v", basePrice["currency"])
	}
	formattedAmount, _ := basePrice["formatted_amount"].(string)
	if !strings.Contains(formattedAmount, "4.19") {
		t.Fatalf("expected formatted amount containing 4.19, got %v", basePrice["formatted_amount"])
	}
	if first["currency"] != "EUR" {
		t.Fatalf("expected top-level currency EUR, got %v", first["currency"])
	}
}

func TestBuildItemDetailIncludesUpsell(t *testing.T) {
	payload := map[string]any{
		"item_id":     "item-1",
		"name":        "Whopper Meal",
		"description": "Burger with fries",
		"price":       map[string]any{"amount": 1595, "currency": "PLN"},
		"options": []any{map[string]any{
			"id":       "group-1",
			"name":     "Choose drink",
			"required": true,
			"min":      1,
			"max":      1,
			"values": []any{
				map[string]any{"id": "water", "name": "Water", "price": 0},
				map[string]any{"id": "juice", "name": "Juice", "price": 150},
			},
		}},
		"upsell_items": []any{
			map[string]any{
				"item_id": "item-2",
				"name":    "Nuggets",
				"price":   map[string]any{"amount": 745, "currency": "PLN"},
				"options": []any{
					map[string]any{
						"id":   "upsell-only",
						"name": "Upsell option",
						"values": []any{
							map[string]any{"id": "extra", "name": "Extra", "price": 25},
						},
					},
				},
			},
		},
	}

	data, warnings := observability.BuildItemDetail(
		"item-1", "venue-1", payload, true, observability.ItemVenueContext{},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	upsell := asSlice(t, data["upsell_items"])
	if len(upsell) != 1 {
		t.Fatalf("expected one upsell item, got %d", len(upsell))
	}
	groups := asSlice(t, data["option_groups"])
	if len(groups) != 1 {
		t.Fatalf("option_groups = %#v", groups)
	}
	group := asMap(t, groups[0])
	values := asSlice(t, group["values"])
	if group["group_id"] != "group-1" || len(values) != 2 {
		t.Fatalf("normalized option group = %#v", group)
	}
	if group["group_id"] == "upsell-only" {
		t.Fatalf("upsell option group leaked into requested item: %#v", groups)
	}
	first := asMap(t, values[0])
	second := asMap(t, values[1])
	if first["value_id"] != "juice" || first["price"] != 150 ||
		second["value_id"] != "water" || second["price"] != 0 {
		t.Fatalf("normalized option values = %#v", values)
	}
}

func TestBuildVenueMenuOptionGroupIDsRequireOptInAndUnionAliases(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":               "item-1",
				"name":             "Configurable item",
				"price":            500,
				"option_group_ids": []any{" ", "size"},
				"option_groups": []any{
					map[string]any{"id": "SIZE"},
					map[string]any{"group_id": "addon"},
				},
				"options": []any{
					map[string]any{"option_id": "addon"},
					map[string]any{"id": "temperature"},
				},
			},
		},
	}

	withoutOptions, _ := observability.BuildVenueMenu(
		"venue-1",
		[]map[string]any{payload},
		"",
		false,
		nil,
	)
	withoutRow := asMap(t, asSlice(t, withoutOptions["items"])[0])
	if _, exists := withoutRow["option_group_ids"]; exists {
		t.Fatalf("option_group_ids leaked without opt-in: %#v", withoutRow)
	}

	withOptions, _ := observability.BuildVenueMenu(
		"venue-1",
		[]map[string]any{payload},
		"",
		true,
		nil,
	)
	withRow := asMap(t, asSlice(t, withOptions["items"])[0])
	got := asSlice(t, withRow["option_group_ids"])
	want := []any{"size", "addon", "temperature"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("option_group_ids = %#v, want %#v", got, want)
	}
}

func TestBuildItemDetailNormalizesPricesWithFallbackCurrency(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"currency": "EUR",
		},
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Coca-Cola Zero 6-pack",
				"price": 419,
			},
		},
		"upsell_items": []any{
			map[string]any{
				"item_id": "item-2",
				"name":    "Nuggets",
				"price":   745,
			},
		},
	}

	data, warnings := observability.BuildItemDetail(
		"item-1", "venue-1", payload, true, observability.ItemVenueContext{},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	price := asMap(t, data["price"])
	if price["currency"] != "EUR" {
		t.Fatalf("expected item price currency EUR, got %v", price["currency"])
	}
	formattedPrice, _ := price["formatted_amount"].(string)
	if !strings.Contains(formattedPrice, "4.19") {
		t.Fatalf("expected item price containing 4.19, got %v", price["formatted_amount"])
	}
	upsell := asSlice(t, data["upsell_items"])
	if len(upsell) != 1 {
		t.Fatalf("expected one upsell item, got %d", len(upsell))
	}
	upsellPrice := asMap(t, asMap(t, upsell[0])["price"])
	if upsellPrice["currency"] != "EUR" {
		t.Fatalf("expected upsell price currency EUR, got %v", upsellPrice["currency"])
	}
	upsellFormatted, _ := upsellPrice["formatted_amount"].(string)
	if !strings.Contains(upsellFormatted, "7.45") {
		t.Fatalf("expected upsell formatted amount containing 7.45, got %v", upsellPrice["formatted_amount"])
	}
}

func TestBuildVenueSearchResultIncludesPromotionsAndPriceRange(t *testing.T) {
	items := []domain.Item{
		{
			Title: "Burger Place",
			Link:  domain.Link{Target: "1"},
			Venue: &domain.Venue{
				ID:               "1",
				Address:          "Burger Street",
				Tags:             []string{"burger"},
				EstimateRange:    "20-30",
				Currency:         "PLN",
				DeliveryPriceInt: intPtr(500),
				Estimate:         25,
				PriceRange:       3,
				Promotions:       []any{map[string]any{"text": "20% off"}},
			},
		},
	}

	data, _ := observability.BuildVenueSearchResult(items, "burger", observability.VenueSortRecommended, nil, "", false, false, nil, 0)
	rows := asSlice(t, data["items"])
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	first := asMap(t, rows[0])
	if first["price_range"] != 3 {
		t.Fatalf("expected price_range 3, got %v", first["price_range"])
	}
	if first["price_range_scale"] != "$$$" {
		t.Fatalf("expected price_range_scale $$$, got %v", first["price_range_scale"])
	}
	promotions := asSlice(t, first["promotions"])
	if len(promotions) != 1 || promotions[0] != "20% off" {
		t.Fatalf("expected promotions [20%% off], got %v", promotions)
	}
}

func TestBuildVenueMenuIncludesDiscounts(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"show_wolt_plus": true,
		},
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Fries",
				"price": 599,
				"promotions": []any{
					map[string]any{"text": "2 for 1"},
				},
			},
		},
	}

	data, warnings := observability.BuildVenueMenu(
		"venue-1", []map[string]any{payload}, "", false, nil, observability.ItemVenueContext{},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	items := asSlice(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one menu item, got %d", len(items))
	}
	if data["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true, got %v", data["wolt_plus"])
	}
	first := asMap(t, items[0])
	discounts := asSlice(t, first["discounts"])
	if len(discounts) != 1 || discounts[0] != "2 for 1" {
		t.Fatalf("expected discounts [2 for 1], got %v", discounts)
	}
}

func TestBuildVenueMenuMergesDynamicCampaignDiscounts(t *testing.T) {
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Steakhouse",
				"price": 1075,
			},
		},
	}
	dynamicPayload := map[string]any{
		"venue_raw": map[string]any{
			"discounts": []any{
				map[string]any{
					"effects": map[string]any{
						"item_discount": map[string]any{
							"fraction": 0.4,
							"include": map[string]any{
								"items": []any{"item-1"},
							},
						},
					},
					"effect_item_badge": map[string]any{
						"text": "40% off selected items",
					},
				},
			},
		},
	}

	data, warnings := observability.BuildVenueMenu(
		"venue-1",
		[]map[string]any{assortmentPayload, dynamicPayload},
		"",
		false,
		nil,
		observability.ItemVenueContext{},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	items := asSlice(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one menu item, got %d", len(items))
	}
	first := asMap(t, items[0])
	basePrice := asMap(t, first["base_price"])
	if intValue(basePrice["amount"]) != 645 {
		t.Fatalf("expected discounted base price 645, got %v", basePrice["amount"])
	}
	originalPrice := asMap(t, first["original_price"])
	if intValue(originalPrice["amount"]) != 1075 {
		t.Fatalf("expected original price 1075, got %v", originalPrice["amount"])
	}
	discounts := asSlice(t, first["discounts"])
	if len(discounts) != 1 || discounts[0] != "40% off selected items" {
		t.Fatalf("expected discounts [40%% off selected items], got %v", discounts)
	}
}

func TestBuildVenueMenuAppliesCampaignFractionOnlyToUndiscountedBase(t *testing.T) {
	for _, test := range []struct {
		name          string
		price         int
		originalPrice int
		wantBase      int
		wantOriginal  int
	}{
		{
			name:          "explicit original proves base is already discounted",
			price:         900,
			originalPrice: 1200,
			wantBase:      900,
			wantOriginal:  1200,
		},
		{
			name:         "missing original derives campaign price",
			price:        1200,
			wantBase:     900,
			wantOriginal: 1200,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := map[string]any{
				"id":    "item-1",
				"name":  "Campaign item",
				"price": test.price,
			}
			if test.originalPrice > 0 {
				item["original_price"] = test.originalPrice
			}
			data, warnings := observability.BuildVenueMenu(
				"venue-1",
				[]map[string]any{
					{"items": []any{item}},
					{
						"venue_raw": map[string]any{
							"discounts": []any{
								map[string]any{
									"effects": map[string]any{
										"item_discount": map[string]any{
											"fraction": 0.25,
											"include": map[string]any{
												"items": []any{"item-1"},
											},
										},
									},
								},
							},
						},
					},
				},
				"",
				false,
				nil,
				observability.ItemVenueContext{},
			)
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v", warnings)
			}
			items := asSlice(t, data["items"])
			first := asMap(t, items[0])
			basePrice := asMap(t, first["base_price"])
			originalPrice := asMap(t, first["original_price"])
			if intValue(basePrice["amount"]) != test.wantBase ||
				intValue(originalPrice["amount"]) != test.wantOriginal {
				t.Fatalf(
					"prices = base %v, original %v; want %d/%d",
					basePrice["amount"],
					originalPrice["amount"],
					test.wantBase,
					test.wantOriginal,
				)
			}
		})
	}
}

func TestExtractVenuePromotionLabelsFromDynamicPayload(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"banners": []any{
				map[string]any{
					"discount": map[string]any{
						"formatted_text": "40% off selected items",
					},
				},
			},
		},
		"venue_raw": map[string]any{
			"discounts": []any{
				map[string]any{
					"description": map[string]any{"title": "40% off selected items"},
				},
				map[string]any{
					"description": map[string]any{"title": "€0 delivery fee"},
				},
			},
		},
	}

	labels := observability.ExtractVenuePromotionLabels(payload)
	if len(labels) != 2 {
		t.Fatalf("expected two labels, got %v", labels)
	}
	if labels[0] != "40% off selected items" && labels[1] != "40% off selected items" {
		t.Fatalf("expected labels to include campaign text, got %v", labels)
	}
}

func TestExtractVenueWoltPlusFromPayload(t *testing.T) {
	payload := map[string]any{
		"venue_raw": map[string]any{
			"is_wolt_plus": true,
		},
	}
	if !observability.ExtractVenueWoltPlus(payload) {
		t.Fatalf("expected ExtractVenueWoltPlus to return true")
	}
}

func TestExtractMenuItemsDerivesDiscountFromOriginalPrice(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":             "item-1",
				"name":           "Coca-Cola Zero 6-pack",
				"price":          419,
				"original_price": 529,
			},
		},
	}

	items := observability.ExtractMenuItems(payload, "venue-1", "venue-slug")
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	first := items[0]
	basePrice := asMap(t, first["base_price"])
	if intValue(basePrice["amount"]) != 419 {
		t.Fatalf("expected base price amount 419, got %v", basePrice["amount"])
	}
	originalPrice := asMap(t, first["original_price"])
	if intValue(originalPrice["amount"]) != 529 {
		t.Fatalf("expected original price amount 529, got %v", originalPrice["amount"])
	}
	discounts := asSlice(t, first["discounts"])
	if len(discounts) == 0 {
		t.Fatalf("expected derived discount label, got %v", discounts)
	}
	label, _ := discounts[0].(string)
	if !strings.Contains(strings.ToLower(label), "off") {
		t.Fatalf("expected derived discount label, got %v", discounts)
	}
}

func TestExtractMenuItemsUsesCurrentAvailabilityAndProductMetadata(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":                  "item-1",
				"name":                "Boneless chicken thigh",
				"price":               1645,
				"disabled_info":       map[string]any{"disable_text": "Sold out"},
				"purchasable_balance": 0,
				"unit_info":           "500 g",
				"unit_price":          map[string]any{"price": 1645, "unit": "kilogram"},
				"sell_by_weight_config": map[string]any{
					"grams_per_step": 500,
					"price_per_kg":   1645,
				},
				"images": []any{
					map[string]any{
						"url":      "https://imageproxy.wolt.com/assets/chicken",
						"blurhash": "blur",
					},
				},
			},
		},
	}

	items := observability.ExtractMenuItems(payload, "venue-1", "venue-slug")
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	item := items[0]
	if available, _ := item["is_available"].(bool); available {
		t.Fatalf("expected item to be unavailable: %#v", item)
	}
	if reason := item["unavailable_reason"]; reason != "Sold out" {
		t.Fatalf("unavailable_reason = %v, want Sold out", reason)
	}
	if legacy, _ := item["is_sold_out"].(bool); !legacy {
		t.Fatalf("expected compatibility is_sold_out alias to be true")
	}
	if imageURL := item["image_url"]; imageURL != "https://imageproxy.wolt.com/assets/chicken" {
		t.Fatalf("image_url = %v", imageURL)
	}
	if unitInfo := item["unit_info"]; unitInfo != "500 g" {
		t.Fatalf("unit_info = %v", unitInfo)
	}
}

func TestBuildDiscoveryFeedDetectsWoltPlusFromIcon(t *testing.T) {
	section := domain.Section{
		Name:  "popular",
		Title: "Popular",
		Items: []domain.Item{
			{
				Title:   "Venue One",
				TrackID: "track-1",
				Link:    domain.Link{Target: "venue-1"},
				Venue: &domain.Venue{
					ID:               "venue-1",
					Slug:             "venue-one",
					Currency:         "PLN",
					DeliveryPriceInt: intPtr(1000),
					EstimateRange:    "25-35",
					Icon:             "wolt-plus",
				},
			},
		},
	}

	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "Krakow", nil, false)
	sections := asSlice(t, data["sections"])
	items := asSlice(t, asMap(t, sections[0])["items"])
	firstItem := asMap(t, items[0])
	if firstItem["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true from icon fallback, got %v", firstItem["wolt_plus"])
	}
}

func TestBuildDiscoveryFeedWoltPlusOnlyFilter(t *testing.T) {
	section := domain.Section{
		Name:  "popular",
		Title: "Popular",
		Items: []domain.Item{
			{
				Title:   "Wolt Plus Venue",
				TrackID: "track-1",
				Link:    domain.Link{Target: "venue-1"},
				Venue: &domain.Venue{
					ID:         "venue-1",
					Slug:       "venue-one",
					Icon:       "wolt-plus",
					Tags:       []string{"burger"},
					PriceRange: 2,
				},
			},
			{
				Title:   "Regular Venue",
				TrackID: "track-2",
				Link:    domain.Link{Target: "venue-2"},
				Venue: &domain.Venue{
					ID:   "venue-2",
					Slug: "venue-two",
					Tags: []string{"pizza"},
				},
			},
		},
	}

	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "Krakow", nil, true)
	if data["wolt_plus_only"] != true {
		t.Fatalf("expected wolt_plus_only true, got %v", data["wolt_plus_only"])
	}
	sections := asSlice(t, data["sections"])
	if len(sections) != 1 {
		t.Fatalf("expected one section, got %d", len(sections))
	}
	items := asSlice(t, asMap(t, sections[0])["items"])
	if len(items) != 1 {
		t.Fatalf("expected one filtered item, got %d", len(items))
	}
	if asMap(t, items[0])["name"] != "Wolt Plus Venue" {
		t.Fatalf("expected Wolt Plus Venue, got %v", asMap(t, items[0])["name"])
	}
}

func TestBuildVenueMenuDetectsWoltPlusFromBadges(t *testing.T) {
	payload := map[string]any{
		"venue": map[string]any{
			"show_wolt_plus": false,
			"badges":         []any{map[string]any{"text": "Wolt+"}},
		},
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Fries",
				"price": 599,
			},
		},
	}
	data, _ := observability.BuildVenueMenu(
		"venue-1", []map[string]any{payload}, "", false, nil, observability.ItemVenueContext{},
	)
	if data["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true from badges fallback, got %v", data["wolt_plus"])
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", value)
	}
	return m
}

func asSlice(t *testing.T, value any) []any {
	t.Helper()
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		t.Fatalf("expected slice, got %T", value)
		return nil
	}
}

func intValue(v any) int {
	if value, ok := v.(int); ok {
		return value
	}
	if value, ok := v.(float64); ok {
		return int(value)
	}
	return 0
}

func intPtr(v int) *int {
	return &v
}

func TestParseVenueSortAcceptsCanonicalAndCompatibilityAliases(t *testing.T) {
	cases := []struct {
		in   string
		want observability.VenueSort
	}{
		{"recommended", observability.VenueSortRecommended},
		{"distance", observability.VenueSortDistance},
		{"rating", observability.VenueSortRating},
		{"delivery_time", observability.VenueSortDeliveryTime},
		{"delivery-time", observability.VenueSortDeliveryTime},
		{"delivery", observability.VenueSortDeliveryTime},
		{"fee", observability.VenueSortDeliveryPrice},
		{"DELIVERY-PRICE", observability.VenueSortDeliveryPrice},
		{"  delivery-time  ", observability.VenueSortDeliveryTime},
	}
	for _, c := range cases {
		got, err := observability.ParseVenueSort(c.in)
		if err != nil {
			t.Fatalf("ParseVenueSort(%q) errored: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseVenueSort(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := observability.ParseVenueSort("nonsense"); err == nil {
		t.Fatal("expected ParseVenueSort to reject unknown sort key")
	} else {
		message := err.Error()
		for _, allowed := range observability.VenueSortInputValues() {
			if !strings.Contains(message, allowed) {
				t.Errorf("error %q does not list allowed value %q", message, allowed)
			}
		}
	}
}

func TestBuildDiscoveryFeedSurfacesBadgesAndMenuHighlights(t *testing.T) {
	section := domain.Section{
		Name:  "popular",
		Title: "Popular",
		Items: []domain.Item{
			{
				Title:   "Featured Venue",
				TrackID: "track-1",
				Link:    domain.Link{Target: "venue-feat"},
				Venue: &domain.Venue{
					ID:       "venue-feat",
					Slug:     "featured-venue",
					Currency: "EUR",
					BadgesV2: []domain.Badge{
						{Icon: "wolt-plus", Variant: "primary", Text: "Wolt+"},
						{Icon: "coupon-fill", Variant: "discount", Text: "20% off"},
					},
					PreviewItems: []any{
						map[string]any{"name": "Cheeseburger pizza", "formatted_price": "19.90 €"},
						map[string]any{"name": "Garlic bread", "price": "4.50 €"},
					},
				},
			},
		},
	}

	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "Helsinki", nil, false)
	items := asSlice(t, asMap(t, asSlice(t, data["sections"])[0])["items"])
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	row := asMap(t, items[0])

	badges := asSlice(t, row["badges"])
	if len(badges) != 2 {
		t.Fatalf("expected 2 badges, got %d", len(badges))
	}
	firstBadge := asMap(t, badges[0])
	if firstBadge["icon"] != "wolt-plus" || firstBadge["variant"] != "primary" || firstBadge["text"] != "Wolt+" {
		t.Fatalf("unexpected first badge shape: %v", firstBadge)
	}

	highlights := asSlice(t, row["menu_highlights"])
	if len(highlights) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(highlights))
	}
	first := asMap(t, highlights[0])
	if first["name"] != "Cheeseburger pizza" || first["formatted_price"] != "19.90 €" {
		t.Fatalf("unexpected highlight shape: %v", first)
	}
	second := asMap(t, highlights[1])
	if second["name"] != "Garlic bread" || second["formatted_price"] != "4.50 €" {
		t.Fatalf("expected fallback `price` key to populate formatted_price, got %v", second)
	}
}

func TestBuildDiscoveryFeedEmitsEmptyBadgesAndHighlights(t *testing.T) {
	section := domain.Section{
		Name:  "boring",
		Title: "Boring",
		Items: []domain.Item{
			{
				Title: "Plain Venue",
				Link:  domain.Link{Target: "venue-plain"},
				Venue: &domain.Venue{ID: "venue-plain", Slug: "plain-venue", Currency: "EUR"},
			},
		},
	}
	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "", nil, false)
	row := asMap(t, asSlice(t, asMap(t, asSlice(t, data["sections"])[0])["items"])[0])
	if badges := asSlice(t, row["badges"]); len(badges) != 0 {
		t.Fatalf("expected empty badges slice, got %v", badges)
	}
	if highlights := asSlice(t, row["menu_highlights"]); len(highlights) != 0 {
		t.Fatalf("expected empty menu_highlights slice, got %v", highlights)
	}
}

func TestBuildDiscoveryFeedClassifiesBrandSection(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "FIN_NV_SB_popular-stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "Wolt Market", Link: domain.Link{Target: "woltmarket-popular-brands:helsinki"}},
				{Title: "K-Market", Link: domain.Link{Target: "k-market-curated:helsinki"}},
			},
		},
		{
			Name:  "dinner-venues",
			Title: "Dinner",
			Items: []domain.Item{
				{Title: "Bastard Burgers", Link: domain.Link{Target: "venue-1"}, Venue: &domain.Venue{ID: "venue-1", Slug: "bastard", Currency: "EUR"}},
			},
		},
	}
	data := observability.BuildDiscoveryFeed(sections, "Helsinki", nil, false)
	sectionsOut := asSlice(t, data["sections"])
	if len(sectionsOut) != 2 {
		t.Fatalf("expected both sections kept, got %d", len(sectionsOut))
	}

	brandsSection := asMap(t, sectionsOut[0])
	if brandsSection["kind"] != "brands" {
		t.Fatalf("expected kind 'brands', got %v", brandsSection["kind"])
	}
	items := asSlice(t, brandsSection["items"])
	if len(items) != 0 {
		t.Fatalf("expected empty items[] for brand kind, got %d", len(items))
	}
	brands := asSlice(t, brandsSection["brands"])
	if len(brands) != 2 {
		t.Fatalf("expected 2 brands, got %d", len(brands))
	}
	if asMap(t, brands[0])["name"] != "Wolt Market" {
		t.Fatalf("expected first brand 'Wolt Market', got %v", brands[0])
	}
	if asMap(t, brands[0])["slug"] != "woltmarket-popular-brands:helsinki" {
		t.Fatalf("expected first brand slug to be the link target, got %v", brands[0])
	}

	venueSection := asMap(t, sectionsOut[1])
	if venueSection["kind"] != "venues" {
		t.Fatalf("expected kind 'venues', got %v", venueSection["kind"])
	}
	if _, ok := venueSection["brands"]; ok {
		t.Fatalf("expected no brands key on venue section, got %v", venueSection["brands"])
	}
}

func TestBuildDiscoveryFeedSkipsBrandSectionUnderWoltPlusOnly(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular-stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "Wolt Market", Link: domain.Link{Target: "woltmarket"}},
			},
		},
	}
	data := observability.BuildDiscoveryFeed(sections, "Helsinki", nil, true)
	if len(asSlice(t, data["sections"])) != 0 {
		t.Fatalf("expected brand section dropped under wolt_plus_only, got %v", data["sections"])
	}
}

func TestBuildVenueSearchResultSurfacesBadgesAndMenuHighlights(t *testing.T) {
	items := []domain.Item{
		{
			Title:   "Burger Place",
			TrackID: "t1",
			Link:    domain.Link{Target: "v1"},
			Venue: &domain.Venue{
				ID:       "v1",
				Slug:     "burger-place",
				Currency: "EUR",
				BadgesV2: []domain.Badge{{Icon: "bike", Variant: "primary", Text: "Fast"}},
				PreviewItems: []any{
					map[string]any{"title": "Bacon Burger", "formatted_price": "9.90 €"},
				},
			},
		},
	}
	data, _ := observability.BuildVenueSearchResult(items, "burger", observability.VenueSortRecommended, nil, "", false, false, nil, 0)
	row := asMap(t, asSlice(t, data["items"])[0])
	badges := asSlice(t, row["badges"])
	if len(badges) != 1 || asMap(t, badges[0])["icon"] != "bike" {
		t.Fatalf("expected bike badge, got %v", badges)
	}
	highlights := asSlice(t, row["menu_highlights"])
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %v", highlights)
	}
	first := asMap(t, highlights[0])
	if first["name"] != "Bacon Burger" || first["formatted_price"] != "9.90 €" {
		t.Fatalf("expected fallback `title` key to populate name, got %v", first)
	}
}
