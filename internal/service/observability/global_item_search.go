package observability

import (
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// BuildGlobalItemSearchResult normalizes the item-targeted Wolt search page.
// The upstream order is significant: it is the relevance ranking returned by
// Wolt and is never re-sorted or re-filtered by the query text here.
func BuildGlobalItemSearchResult(
	query string,
	requestedLimit int,
	payload map[string]any,
	availableOnly bool,
) (map[string]any, []string) {
	warnings := []string{}
	rows := make([]map[string]any, 0, requestedLimit)
	seen := map[string]struct{}{}
	upstreamCount := 0

	for _, rawSection := range toSlice(payload["sections"]) {
		section := toMap(rawSection)
		for _, rawEntry := range toSlice(section["items"]) {
			entry := toMap(rawEntry)
			summary := toMap(entry["menu_item"])
			if summary == nil {
				continue
			}
			upstreamCount++
			details := toMap(toMap(entry["link"])["menu_item_details"])
			row, err := normalizeGlobalItem(summary, details, upstreamCount)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skipped global search row %d: %v", upstreamCount, err))
				continue
			}
			key := strings.ToLower(stringFromAny(row["venue_id"]) + "\x00" + stringFromAny(row["item_id"]))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
		}
	}

	normalizedCount := len(rows)
	if availableOnly {
		filtered := rows[:0]
		for _, row := range rows {
			available, known := row["is_available"].(bool)
			if known && !available {
				continue
			}
			filtered = append(filtered, row)
		}
		rows = filtered
	}
	if normalizedCount == 0 {
		warnings = append(warnings, "no valid menu items were found in the global search response")
	}

	data := map[string]any{
		"query":                   strings.TrimSpace(query),
		"requested_limit":         requestedLimit,
		"upstream_returned_count": upstreamCount,
		"normalized_count":        normalizedCount,
		"returned_count":          len(rows),
		"filtered_out_count":      normalizedCount - len(rows),
		"available_only":          availableOnly,
		"limit_reached":           upstreamCount >= requestedLimit,
		"upstream_cap_reached":    requestedLimit == domain.GlobalItemSearchMaxLimit && upstreamCount >= requestedLimit,
		"completeness":            "unknown",
		"items":                   rows,
		"venue_groups":            buildGlobalItemVenueGroups(rows, strings.TrimSpace(query)),
	}
	return data, warnings
}

func normalizeGlobalItem(summary map[string]any, details map[string]any, rank int) (map[string]any, error) {
	itemID := firstGlobalSearchString(details["id"], summary["id"])
	venueID := firstGlobalSearchString(details["venue_id"], summary["venue_id"])
	name := firstGlobalSearchString(details["name"], summary["name"])
	if itemID == "" || venueID == "" || name == "" {
		return nil, fmt.Errorf("missing item id, venue id, or name")
	}
	if detailID := strings.TrimSpace(stringFromAny(details["id"])); detailID != "" &&
		!strings.EqualFold(detailID, strings.TrimSpace(stringFromAny(summary["id"]))) &&
		strings.TrimSpace(stringFromAny(summary["id"])) != "" {
		return nil, fmt.Errorf("summary and detail item ids disagree")
	}
	if detailVenueID := strings.TrimSpace(stringFromAny(details["venue_id"])); detailVenueID != "" &&
		!strings.EqualFold(detailVenueID, strings.TrimSpace(stringFromAny(summary["venue_id"]))) &&
		strings.TrimSpace(stringFromAny(summary["venue_id"])) != "" {
		return nil, fmt.Errorf("summary and detail venue ids disagree")
	}

	currency := firstGlobalSearchString(details["currency"], summary["currency"])
	row := map[string]any{
		"global_rank":          rank,
		"item_id":              itemID,
		"name":                 name,
		"description":          emptyToNil(firstGlobalSearchString(details["description"], summary["description"])),
		"base_price":           normalizeGlobalSearchPrice(firstGlobalSearchValue(details["price"], summary["price"]), currency),
		"original_price":       normalizeGlobalSearchPrice(firstGlobalSearchValue(details["original_price"], summary["original_price"]), currency),
		"price_type":           emptyToNil(firstGlobalSearchString(details["price_type"], summary["price_type"])),
		"unit_price":           normalizeGlobalSearchPrice(details["unit_price"], currency),
		"unit_price_type":      emptyToNil(stringFromAny(details["unit_price_type"])),
		"unit_size":            details["unit_size"],
		"unit_size_type":       emptyToNil(stringFromAny(details["unit_size_type"])),
		"is_sold_by_weight":    boolOrNil(details["is_sold_by_weight"]),
		"is_available":         boolOrNil(summary["is_available"]),
		"image_url":            emptyToNil(firstGlobalSearchString(toMap(details["image"])["url"], toMap(summary["image"])["url"])),
		"product_line":         emptyToNil(stringFromAny(details["product_line"])),
		"tags":                 firstGlobalSearchSlice(details["tags"], summary["tags"]),
		"action_link":          emptyToNil(stringFromAny(details["action_link"])),
		"venue_id":             venueID,
		"venue_slug":           emptyToNil(stringFromAny(details["venue_slug"])),
		"venue_name":           emptyToNil(firstGlobalSearchString(details["venue_name"], summary["venue_name"])),
		"venue_status":         emptyToNil(firstGlobalSearchString(details["venue_status"], summary["venue_status"])),
		"venue_rating":         firstGlobalSearchValue(details["venue_rating"], summary["venue_rating"]),
		"venue_image_url":      emptyToNil(stringFromAny(toMap(details["venue_image"])["url"])),
		"delivery_estimate":    firstGlobalSearchValue(details["estimate_range"], summary["estimate_range"]),
		"delivery_method":      emptyToNil(firstGlobalSearchString(details["delivery_method"], summary["delivery_method"])),
		"delivery_method_type": emptyToNil(firstGlobalSearchString(details["delivery_method_type"], summary["delivery_method_type"])),
		"show_wolt_plus":       boolOrNil(firstGlobalSearchValue(details["show_wolt_plus"], summary["show_wolt_plus"])),
	}
	removeEmptyGlobalSearchPrices(row)
	return row, nil
}

func buildGlobalItemVenueGroups(items []map[string]any, query string) []map[string]any {
	groups := make([]map[string]any, 0)
	byVenue := map[string]map[string]any{}
	for _, item := range items {
		venueID := strings.TrimSpace(stringFromAny(item["venue_id"]))
		key := strings.ToLower(venueID)
		group, ok := byVenue[key]
		if !ok {
			venueRef := firstGlobalSearchString(item["venue_slug"], venueID)
			group = map[string]any{
				"venue_id":     venueID,
				"venue_slug":   item["venue_slug"],
				"venue_name":   item["venue_name"],
				"venue_status": item["venue_status"],
				"venue_rating": item["venue_rating"],
				"item_count":   0,
				"item_ranks":   []int{},
				"expand": map[string]any{
					"mcp_tool":    "wolt_venue_search_items",
					"venue":       venueRef,
					"query":       query,
					"cli_command": "wolt venue menu",
					"cli_args":    []string{venueRef, "--query", query},
				},
			}
			byVenue[key] = group
			groups = append(groups, group)
		}
		group["item_count"] = group["item_count"].(int) + 1
		group["item_ranks"] = append(group["item_ranks"].([]int), item["global_rank"].(int))
	}
	return groups
}

func normalizeGlobalSearchPrice(value any, currency string) map[string]any {
	price := toMap(value)
	if price == nil && value != nil {
		price = map[string]any{"amount": value}
	}
	return normalizeBasePrice(price, currency)
}

func removeEmptyGlobalSearchPrices(row map[string]any) {
	for _, key := range []string{"base_price", "original_price", "unit_price"} {
		price := toMap(row[key])
		if len(price) == 0 || (price["amount"] == nil && strings.TrimSpace(stringFromAny(price["formatted_amount"])) == "") {
			delete(row, key)
		}
	}
}

func firstGlobalSearchString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringFromAny(value)); text != "" {
			return text
		}
	}
	return ""
}

func firstGlobalSearchValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstGlobalSearchSlice(values ...any) []any {
	for _, value := range values {
		if list := toSlice(value); list != nil {
			return list
		}
	}
	return []any{}
}

func boolOrNil(value any) any {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return nil
}
