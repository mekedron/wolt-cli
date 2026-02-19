package observability_test

import (
	"testing"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
	"github.com/Valaraucoo/wolt-cli/internal/service/observability"
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
				},
			},
		},
	}

	data := observability.BuildDiscoveryFeed([]domain.Section{section}, "Krakow", nil)
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
