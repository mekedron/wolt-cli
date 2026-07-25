package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// menuItem builds a normalized menu row in the shape produced by
// observability.ExtractMenuItems (and consumed by selectCheapestMenuItem).
func menuItem(id, name string, price int, soldOut bool) map[string]any {
	item := map[string]any{
		"item_id":      id,
		"name":         name,
		"base_price":   map[string]any{"amount": price, "currency": "EUR"},
		"is_available": !soldOut,
		"is_sold_out":  soldOut,
	}
	if soldOut {
		item["disabled_info"] = map[string]any{"disable_text": "Sold out"}
	} else {
		item["disabled_info"] = nil
	}
	return item
}

func TestSelectCheapestMenuItem(t *testing.T) {
	cases := []struct {
		name   string
		items  []map[string]any
		query  string
		wantID string
		wantOK bool
	}{
		{
			name: "cheapest among query matches wins",
			items: []map[string]any{
				menuItem("qp", "Quarter Pounder with Cheese", 760, false),
				menuItem("dqp", "Double Quarter Pounder with Cheese", 1015, false),
				menuItem("cb", "Cheeseburger", 250, false),
			},
			query:  "cheese",
			wantID: "cb",
			wantOK: true,
		},
		{
			name: "sold-out items are skipped even when cheapest",
			items: []map[string]any{
				menuItem("cb", "Cheeseburger", 250, true),
				menuItem("qp", "Quarter Pounder with Cheese", 760, false),
			},
			query:  "cheese",
			wantID: "qp",
			wantOK: true,
		},
		{
			name: "zero and negative priced items are skipped",
			items: []map[string]any{
				menuItem("free", "Free Cheese Sample", 0, false),
				menuItem("bogus", "Cheese Glitch", -100, false),
				menuItem("cb", "Cheeseburger", 250, false),
			},
			query:  "cheese",
			wantID: "cb",
			wantOK: true,
		},
		{
			name: "empty query takes the venue's cheapest orderable item",
			items: []map[string]any{
				menuItem("qp", "Quarter Pounder", 760, false),
				menuItem("fries", "Fries", 190, false),
				menuItem("cola", "Cola", 220, false),
			},
			query:  "",
			wantID: "fries",
			wantOK: true,
		},
		{
			name: "whitespace-only query behaves like empty",
			items: []map[string]any{
				menuItem("qp", "Quarter Pounder", 760, false),
				menuItem("fries", "Fries", 190, false),
			},
			query:  "   ",
			wantID: "fries",
			wantOK: true,
		},
		{
			name: "query is matched case-insensitively",
			items: []map[string]any{
				menuItem("cb", "CheeseBurger", 250, false),
			},
			query:  "CHEESE",
			wantID: "cb",
			wantOK: true,
		},
		{
			name: "no in-stock match returns not found",
			items: []map[string]any{
				menuItem("cb", "Cheeseburger", 250, true),
				menuItem("dcb", "Double Cheeseburger", 390, true),
			},
			query:  "cheese",
			wantOK: false,
		},
		{
			name: "query matching nothing returns not found",
			items: []map[string]any{
				menuItem("fries", "Fries", 190, false),
			},
			query:  "sushi",
			wantOK: false,
		},
		{
			name:   "empty menu returns not found",
			items:  nil,
			query:  "",
			wantOK: false,
		},
		{
			name: "price ties break deterministically by name",
			items: []map[string]any{
				menuItem("z", "Zucchini Bites", 300, false),
				menuItem("a", "Apple Pie", 300, false),
			},
			query:  "",
			wantID: "a",
			wantOK: true,
		},
		{
			name: "rows missing an id or name are skipped",
			items: []map[string]any{
				menuItem("", "Cheeseburger No ID", 100, false),
				menuItem("noname", "", 120, false),
				menuItem("cb", "Cheeseburger", 250, false),
			},
			query:  "",
			wantID: "cb",
			wantOK: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := selectCheapestMenuItem(tc.items, tc.query)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v; want %v (got %+v)", ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				return
			}
			if got.ID != tc.wantID {
				t.Fatalf("ID = %q; want %q (got %+v)", got.ID, tc.wantID, got)
			}
		})
	}
}

func TestSelectCheapestMenuItemReportsPriceAndName(t *testing.T) {
	items := []map[string]any{
		menuItem("qp", "Quarter Pounder with Cheese", 760, false),
		menuItem("cb", "Cheeseburger", 250, false),
	}
	got, ok := selectCheapestMenuItem(items, "cheese")
	if !ok {
		t.Fatalf("expected a match")
	}
	if got.ID != "cb" || got.Name != "Cheeseburger" || got.Price != 250 {
		t.Fatalf("unexpected candidate: %+v", got)
	}
}

func assortmentPayload(items ...map[string]any) map[string]any {
	rows := make([]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, item)
	}
	return map[string]any{"items": rows}
}

func TestResolveCheapestItemPicksCheapestMatch(t *testing.T) {
	var gotSlug string
	wolt := &testWoltAPI{
		assortmentBySlugFn: func(_ context.Context, slug string) (map[string]any, error) {
			gotSlug = slug
			return assortmentPayload(
				menuItem("qp", "Quarter Pounder with Cheese", 760, false),
				menuItem("dqp", "Double Quarter Pounder with Cheese", 1015, false),
				menuItem("cb", "Cheeseburger", 250, false),
			), nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	got, err := resolveCheapestItem(context.Background(), deps, "mcdonalds-kamppi-1", "venue-id", "cheese")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSlug != "mcdonalds-kamppi-1" {
		t.Fatalf("expected slug passed through, got %q", gotSlug)
	}
	if got.ID != "cb" || got.Price != 250 {
		t.Fatalf("expected cheapest cheese item, got %+v", got)
	}
}

func TestResolveCheapestItemNoQueryTakesVenueCheapest(t *testing.T) {
	wolt := &testWoltAPI{
		assortmentBySlugFn: func(_ context.Context, _ string) (map[string]any, error) {
			return assortmentPayload(
				menuItem("qp", "Quarter Pounder", 760, false),
				menuItem("fries", "Fries", 190, false),
			), nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	got, err := resolveCheapestItem(context.Background(), deps, "mcdonalds-kamppi-1", "venue-id", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "fries" {
		t.Fatalf("expected the venue's cheapest item, got %+v", got)
	}
}

func TestResolveCheapestItemQueryMissIsExplanatory(t *testing.T) {
	wolt := &testWoltAPI{
		assortmentBySlugFn: func(_ context.Context, _ string) (map[string]any, error) {
			// Every cheese item is sold out — the breakfast-hours failure mode
			// that flaked the live smoke (issue #25).
			return assortmentPayload(
				menuItem("qp", "Quarter Pounder with Cheese", 760, true),
				menuItem("hash", "Hash Browns", 200, false),
			), nil
		},
	}
	deps := Dependencies{Wolt: wolt}

	_, err := resolveCheapestItem(context.Background(), deps, "mcdonalds-kamppi-1", "venue-id", "cheese")
	if err == nil {
		t.Fatalf("expected an error when nothing in stock matched the query")
	}
	if !strings.Contains(err.Error(), "cheese") {
		t.Fatalf("error should name the query, got %q", err.Error())
	}
}

func TestResolveCheapestItemPropagatesLookupError(t *testing.T) {
	wolt := &testWoltAPI{
		assortmentBySlugFn: func(_ context.Context, _ string) (map[string]any, error) {
			return nil, errors.New("boom")
		},
	}
	deps := Dependencies{Wolt: wolt}

	_, err := resolveCheapestItem(context.Background(), deps, "slug", "venue-id", "cheese")
	if err == nil || !strings.Contains(err.Error(), "venue menu lookup failed") {
		t.Fatalf("expected wrapped lookup error, got %v", err)
	}
}
