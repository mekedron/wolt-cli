package observability_test

import (
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
	)
	if intValue(data["total"]) != 1 {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected fallback warning")
	}
}

func TestBuildVenueDetailIncludesTags(t *testing.T) {
	item := &domain.Item{
		Title: "Burger Place",
		Link:  domain.Link{Target: "venue-1"},
		Venue: &domain.Venue{ID: "venue-1", Slug: "burger-place", Currency: "PLN", DeliveryPriceInt: intPtr(500)},
	}
	restaurant := &domain.Restaurant{ID: "venue-1", Slug: "burger-place", Address: "Street 1", Currency: "PLN", FoodTags: []string{"burger"}}

	data, warnings, err := observability.BuildVenueDetail(item, restaurant, map[string]struct{}{"tags": {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
	tags := asSlice(t, data["tags"])
	if len(tags) != 1 || tags[0] != "burger" {
		t.Fatalf("expected tags [burger], got %v", tags)
	}
}

func TestBuildItemDetailIncludesUpsell(t *testing.T) {
	payload := map[string]any{
		"item_id":       "item-1",
		"name":          "Whopper Meal",
		"description":   "Burger with fries",
		"price":         map[string]any{"amount": 1595, "currency": "PLN"},
		"option_groups": []any{map[string]any{"id": "group-1", "name": "Choose drink", "required": true, "min": 1, "max": 1}},
		"upsell_items":  []any{map[string]any{"item_id": "item-2", "name": "Nuggets", "price": map[string]any{"amount": 745, "currency": "PLN"}}},
	}

	data, warnings := observability.BuildItemDetail("item-1", "venue-1", payload, true)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	upsell := asSlice(t, data["upsell_items"])
	if len(upsell) != 1 {
		t.Fatalf("expected one upsell item, got %d", len(upsell))
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

	data, warnings := observability.BuildVenueMenu("venue-1", []map[string]any{payload}, "", false, nil)
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
	data, _ := observability.BuildVenueMenu("venue-1", []map[string]any{payload}, "", false, nil)
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
