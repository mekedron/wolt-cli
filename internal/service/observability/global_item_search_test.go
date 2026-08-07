package observability

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestBuildGlobalItemSearchResultPreservesRankingAndAvailabilityState(t *testing.T) {
	payload := map[string]any{
		"future_field": "ignored",
		"sections": []any{
			map[string]any{
				"items": []any{
					globalSearchTestEntry("item-a", "venue-a", "Chef's selection", false, true),
					globalSearchTestEntry("item-b", "venue-b", "Seasonal bowl", nil, true),
					globalSearchTestEntry("item-a", "venue-a", "Duplicate", true, true),
					globalSearchTestEntry("", "venue-c", "Malformed", true, true),
					map[string]any{"template": "unrelated-template"},
				},
			},
		},
	}

	data, warnings := BuildGlobalItemSearchResult("literal-not-in-item-names", 4, payload, true)
	if got := intValue(data["upstream_returned_count"]); got != 4 {
		t.Fatalf("upstream_returned_count = %d, want 4", got)
	}
	if got := intValue(data["normalized_count"]); got != 2 {
		t.Fatalf("normalized_count = %d, want 2", got)
	}
	if got := intValue(data["returned_count"]); got != 1 {
		t.Fatalf("returned_count = %d, want 1", got)
	}
	if got := intValue(data["filtered_out_count"]); got != 1 {
		t.Fatalf("filtered_out_count = %d, want 1", got)
	}
	if data["completeness"] != "unknown" || data["limit_reached"] != true || data["upstream_cap_reached"] != false {
		t.Fatalf("boundary metadata = %#v", data)
	}
	items := data["items"].([]map[string]any)
	row := items[0]
	if row["global_rank"] != 2 || row["name"] != "Seasonal bowl" {
		t.Fatalf("filtered row = %#v; server rank and semantic match must be preserved", row)
	}
	if row["is_available"] != nil {
		t.Fatalf("missing availability was coerced to %#v", row["is_available"])
	}
	if got := stringFromAny(toMap(row["base_price"])["formatted_amount"]); got != "EUR 12.34" {
		t.Fatalf("formatted price = %q", got)
	}
	groups := data["venue_groups"].([]map[string]any)
	if len(groups) != 1 {
		t.Fatalf("venue group count = %d, want 1", len(groups))
	}
	group := groups[0]
	if group["item_count"] != 1 || !slices.Equal(group["item_ranks"].([]int), []int{2}) {
		t.Fatalf("venue group = %#v", group)
	}
	expand := toMap(group["expand"])
	if expand["mcp_tool"] != "wolt_venue_search_items" || expand["venue"] != "venue-b-slug" {
		t.Fatalf("expand action = %#v", expand)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "missing item id") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildGlobalItemSearchResultReportsCapWithoutClaimingCompleteness(t *testing.T) {
	entries := make([]any, domain.GlobalItemSearchMaxLimit)
	for index := range entries {
		entries[index] = globalSearchTestEntry(
			fmt.Sprintf("item-%d", index),
			fmt.Sprintf("venue-%d", index),
			"Ranked item",
			true,
			false,
		)
	}
	data, warnings := BuildGlobalItemSearchResult("query", domain.GlobalItemSearchMaxLimit, map[string]any{
		"sections": []any{map[string]any{"items": entries}},
	}, false)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if data["upstream_cap_reached"] != true || data["completeness"] != "unknown" {
		t.Fatalf("cap metadata = %#v", data)
	}
}

func globalSearchTestEntry(itemID, venueID, name string, available any, withDetails bool) map[string]any {
	summary := map[string]any{
		"id":         itemID,
		"venue_id":   venueID,
		"venue_name": "Venue " + venueID,
		"name":       name,
		"price":      1234,
		"currency":   "EUR",
	}
	if available != nil {
		summary["is_available"] = available
	}
	entry := map[string]any{"menu_item": summary}
	if withDetails {
		entry["link"] = map[string]any{
			"menu_item_details": map[string]any{
				"id":         itemID,
				"venue_id":   venueID,
				"venue_slug": venueID + "-slug",
				"name":       name,
				"price":      1234,
				"currency":   "EUR",
			},
		}
	}
	return entry
}
