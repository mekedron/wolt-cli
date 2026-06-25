package checkoutpayload

import (
	"context"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// fakeAPI embeds the gateway interface so we only implement the handful of
// methods Build actually touches; any un-overridden call would panic, which
// keeps the tests honest about what Build depends on.
type fakeAPI struct {
	woltgateway.API
	venueItemPageFn func(ctx context.Context, venueID, itemID string) (map[string]any, error)
	assortmentFn    func(ctx context.Context, slug string) (map[string]any, error)
}

func (f *fakeAPI) VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error) {
	if f.venueItemPageFn != nil {
		return f.venueItemPageFn(ctx, venueID, itemID)
	}
	return map[string]any{}, nil
}

func (f *fakeAPI) AssortmentByVenueSlug(ctx context.Context, slug string) (map[string]any, error) {
	if f.assortmentFn != nil {
		return f.assortmentFn(ctx, slug)
	}
	return map[string]any{}, nil
}

// basketWithItem builds a minimal basket whose single item already carries a
// category_id, so Build needs no upstream enrichment (wolt may be nil).
func basketWithItem(total string, item map[string]any) map[string]any {
	return map[string]any{
		"venue": map[string]any{
			"id":      "5f9a1b2c3d4e5f6071829304",
			"country": "FIN",
		},
		"total": total,
		"items": []any{item},
	}
}

func purchasePlan(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	plan, ok := payload["purchase_plan"].(map[string]any)
	if !ok {
		t.Fatalf("payload has no purchase_plan map; got keys %v", mapKeys(payload))
	}
	return plan
}

func firstMenuItem(t *testing.T, plan map[string]any) map[string]any {
	t.Helper()
	items, ok := plan["menu_items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("purchase_plan.menu_items missing or empty: %v", plan["menu_items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("menu_items[0] is not a map: %T", items[0])
	}
	return item
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestBuildProducesPurchasePlanShape is the regression lock for the bug PR #23
// fixes: the request must be wrapped in a top-level purchase_plan, NOT the old
// flat {venue_id, currency, items, delivery_mode, location} body that Wolt now
// rejects with `('body', 'purchase_plan'): Field required`.
func TestBuildProducesPurchasePlanShape(t *testing.T) {
	basket := basketWithItem("€12.50", map[string]any{
		"id":          "627cb2c7e2a6f0a1b2c3d4e5",
		"count":       2,
		"price":       625,
		"category_id": "cat-burgers",
	})

	payload, warnings, err := Build(
		context.Background(),
		nil, // no upstream: item already carries category_id
		nil,
		basket,
		domain.Location{Lat: 60.1699, Lon: 24.9384},
		"standard",
		0,
		"",
	)
	if err != nil {
		t.Fatalf("Build returned error: %v (warnings: %v)", err, warnings)
	}

	// Exactly one top-level key, and it is purchase_plan.
	if got := mapKeys(payload); len(got) != 1 || got[0] != "purchase_plan" {
		t.Fatalf("top-level keys = %v, want exactly [purchase_plan]", got)
	}
	// The rejected flat shape must NOT leak at the top level.
	for _, banned := range []string{"venue_id", "currency", "items", "delivery_mode", "location"} {
		if _, exists := payload[banned]; exists {
			t.Errorf("payload must not contain top-level %q (old rejected shape)", banned)
		}
	}

	plan := purchasePlan(t, payload)
	venue, _ := plan["venue"].(map[string]any)
	if venue["id"] != "5f9a1b2c3d4e5f6071829304" {
		t.Errorf("purchase_plan.venue.id = %v", venue["id"])
	}
	if venue["currency"] != "EUR" {
		t.Errorf("purchase_plan.venue.currency = %v, want EUR", venue["currency"])
	}
	if venue["country"] != "FIN" {
		t.Errorf("purchase_plan.venue.country = %v, want FIN", venue["country"])
	}
	if plan["delivery_method"] != "homedelivery" {
		t.Errorf("delivery_method = %v, want homedelivery", plan["delivery_method"])
	}
	if plan["is_priority_delivery"] != false {
		t.Errorf("is_priority_delivery = %v, want false", plan["is_priority_delivery"])
	}

	// menu_items mapping: count, base_price, derived end_amount, category_id.
	item := firstMenuItem(t, plan)
	if item["base_price"] != 625 {
		t.Errorf("base_price = %v, want 625", item["base_price"])
	}
	if item["count"] != 2 {
		t.Errorf("count = %v, want 2", item["count"])
	}
	if item["end_amount"] != 1250 {
		t.Errorf("end_amount = %v, want 1250 (count*price)", item["end_amount"])
	}
	if item["category_id"] != "cat-burgers" {
		t.Errorf("category_id = %v, want cat-burgers", item["category_id"])
	}
	if item["venue_id"] != "5f9a1b2c3d4e5f6071829304" {
		t.Errorf("menu_item.venue_id = %v", item["venue_id"])
	}

	// Delivery coordinates come straight from the resolved location.
	delivery, _ := plan["delivery"].(map[string]any)
	coords, _ := delivery["delivery_coordinates"].(map[string]any)
	if coords["latitude"] != 60.1699 || coords["longitude"] != 24.9384 {
		t.Errorf("delivery_coordinates = %v, want {60.1699, 24.9384}", coords)
	}
}

func TestBuildDeliveryModeNormalisation(t *testing.T) {
	cases := []struct {
		mode         string
		wantPriority bool
		wantErr      bool
	}{
		{"", false, false},           // defaults to standard
		{"standard", false, false},   //
		{"priority", true, false},    // toggles is_priority_delivery
		{"PRIORITY", true, false},    // case-insensitive
		{" schedule ", false, false}, // trimmed + accepted
		{"takeaway", false, true},    // unsupported -> error
	}
	for _, c := range cases {
		c := c
		t.Run(strings.TrimSpace(c.mode)+"_", func(t *testing.T) {
			basket := basketWithItem("€5", map[string]any{
				"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500, "category_id": "cat",
			})
			payload, _, err := Build(context.Background(), nil, nil, basket,
				domain.Location{Lat: 1, Lon: 2}, c.mode, 0, "")
			if c.wantErr {
				if err == nil {
					t.Fatalf("mode %q: expected error, got none", c.mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("mode %q: unexpected error: %v", c.mode, err)
			}
			plan := purchasePlan(t, payload)
			if plan["is_priority_delivery"] != c.wantPriority {
				t.Errorf("mode %q: is_priority_delivery = %v, want %v", c.mode, plan["is_priority_delivery"], c.wantPriority)
			}
		})
	}
}

func TestBuildRejectsMissingBasePrice(t *testing.T) {
	basket := basketWithItem("€5", map[string]any{
		"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 0, "category_id": "cat",
	})
	_, _, err := Build(context.Background(), nil, nil, basket,
		domain.Location{}, "standard", 0, "")
	if err == nil || !strings.Contains(err.Error(), "base_price") {
		t.Fatalf("expected base_price error, got %v", err)
	}
}

func TestBuildCurrencyInferenceAndDefault(t *testing.T) {
	cases := []struct {
		total string
		want  string
	}{
		{"$9.99", "USD"},
		{"€4.00", "EUR"},
		{"", "EUR"},      // no symbol -> default EUR
		{"12.00", "EUR"}, // unrecognised -> default EUR
	}
	for _, c := range cases {
		basket := basketWithItem(c.total, map[string]any{
			"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500, "category_id": "cat",
		})
		payload, _, err := Build(context.Background(), nil, nil, basket,
			domain.Location{}, "standard", 0, "")
		if err != nil {
			t.Fatalf("total %q: %v", c.total, err)
		}
		venue, _ := purchasePlan(t, payload)["venue"].(map[string]any)
		if venue["currency"] != c.want {
			t.Errorf("total %q: currency = %v, want %v", c.total, venue["currency"], c.want)
		}
	}
}

func TestBuildAppliesTipAndPromo(t *testing.T) {
	basket := basketWithItem("€5", map[string]any{
		"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500, "category_id": "cat",
	})
	payload, _, err := Build(context.Background(), nil, nil, basket,
		domain.Location{}, "standard", 200, "SAVE10")
	if err != nil {
		t.Fatal(err)
	}
	plan := purchasePlan(t, payload)
	if plan["courier_tip"] != 200 {
		t.Errorf("courier_tip = %v, want 200", plan["courier_tip"])
	}
	promo, _ := plan["use_promo_discount_ids"].([]any)
	if len(promo) != 1 || promo[0] != "SAVE10" {
		t.Errorf("use_promo_discount_ids = %v, want [SAVE10]", promo)
	}
}

// TestBuildFallsBackToItemIDForCategory locks in the documented fallback: when
// no category can be resolved but the item id looks like a Mongo object id,
// Build uses the item id as the category and records a warning rather than
// failing the whole preview.
func TestBuildFallsBackToItemIDForCategory(t *testing.T) {
	itemID := "627cb2c7e2a6f0a1b2c3d4e5" // 24 hex chars
	basket := basketWithItem("€5", map[string]any{
		"id": itemID, "count": 1, "price": 500,
	})
	payload, warnings, err := Build(context.Background(), nil, nil, basket,
		domain.Location{}, "standard", 0, "")
	if err != nil {
		t.Fatalf("expected fallback, got error: %v", err)
	}
	item := firstMenuItem(t, purchasePlan(t, payload))
	if item["category_id"] != itemID {
		t.Errorf("category_id = %v, want fallback to item id %v", item["category_id"], itemID)
	}
	if !containsSubstr(warnings, "falling back to item id") {
		t.Errorf("expected a fallback warning, got %v", warnings)
	}
}

func TestBuildRejectsUnresolvableCategory(t *testing.T) {
	// A non-object-id item with no category anywhere cannot be priced.
	basket := basketWithItem("€5", map[string]any{
		"id": "human-readable-slug", "count": 1, "price": 500,
	})
	_, _, err := Build(context.Background(), nil, nil, basket,
		domain.Location{}, "standard", 0, "")
	if err == nil || !strings.Contains(err.Error(), "category_id") {
		t.Fatalf("expected category_id error, got %v", err)
	}
}

// TestBuildEnrichesCategoryFromItemPage exercises the wolt != nil branch: the
// item carries no category, but VenueItemPage supplies one. Confirms Build
// consults the gateway and threads the venuePageStatic callback safely.
func TestBuildEnrichesCategoryFromItemPage(t *testing.T) {
	itemPageCalls := 0
	staticCalls := 0
	api := &fakeAPI{
		venueItemPageFn: func(_ context.Context, venueID, itemID string) (map[string]any, error) {
			itemPageCalls++
			if venueID != "5f9a1b2c3d4e5f6071829304" {
				t.Errorf("VenueItemPage venueID = %q", venueID)
			}
			return map[string]any{"category_id": "cat-from-page"}, nil
		},
	}
	venuePageStatic := func(_ context.Context, _ string) (map[string]any, error) {
		staticCalls++
		return map[string]any{}, nil
	}

	basket := basketWithItem("€5", map[string]any{
		"id": "627cb2c7e2a6f0a1b2c3d4e5", "count": 1, "price": 500,
	})
	// Give the venue a slug so the static/assortment enrichment branch runs too.
	basket["venue"].(map[string]any)["slug"] = "burger-place"

	payload, _, err := Build(context.Background(), api, venuePageStatic, basket,
		domain.Location{}, "standard", 0, "")
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if itemPageCalls != 1 {
		t.Errorf("VenueItemPage called %d times, want 1", itemPageCalls)
	}
	if staticCalls != 1 {
		t.Errorf("venuePageStatic called %d times, want 1", staticCalls)
	}
	item := firstMenuItem(t, purchasePlan(t, payload))
	if item["category_id"] != "cat-from-page" {
		t.Errorf("category_id = %v, want cat-from-page (from item page)", item["category_id"])
	}
}

func containsSubstr(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
