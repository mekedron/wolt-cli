package observability

import (
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// BuildVenueMenu builds normalized venue menu payload.
func BuildVenueMenu(venueID string, payloads []map[string]any, category string, includeOptions bool, limit *int) (map[string]any, []string) {
	warnings := []string{}
	menuItems := []map[string]any{}
	isWoltPlus := false

	for _, payload := range payloads {
		menuItems = append(menuItems, ExtractMenuItems(payload, venueID, "")...)
		if !isWoltPlus && payloadVenueWoltPlus(payload) {
			isWoltPlus = true
		}
	}

	if strings.TrimSpace(category) != "" {
		loweredCategory := strings.ToLower(strings.TrimSpace(category))
		filtered := []map[string]any{}
		for _, item := range menuItems {
			if strings.Contains(strings.ToLower(stringFromAny(item["category"])), loweredCategory) {
				filtered = append(filtered, item)
			}
		}
		menuItems = filtered
	}

	menuItems = limitSlice(menuItems, limit)
	if len(menuItems) == 0 {
		warnings = append(warnings, "no menu items were discovered in upstream venue payloads")
	}

	categorySet := map[string]struct{}{}
	rows := make([]map[string]any, 0, len(menuItems))
	for _, item := range menuItems {
		categorySet[stringFromAny(item["category"])] = struct{}{}
		row := map[string]any{
			"item_id":    item["item_id"],
			"name":       item["name"],
			"base_price": item["base_price"],
			"discounts":  item["discounts"],
		}
		if includeOptions {
			row["option_group_ids"] = item["option_group_ids"]
		}
		rows = append(rows, row)
	}

	categories := make([]string, 0, len(categorySet))
	for categoryValue := range categorySet {
		categories = append(categories, categoryValue)
	}
	sort.Strings(categories)

	return map[string]any{
		"venue_id":   venueID,
		"wolt_plus":  isWoltPlus,
		"categories": categories,
		"items":      rows,
	}, warnings
}

func payloadVenueWoltPlus(payload map[string]any) bool {
	venue := toMap(payload["venue"])
	venueRaw := toMap(payload["venue_raw"])
	if boolValue(payload["show_wolt_plus"]) || boolValue(payload["wolt_plus"]) ||
		boolValue(venue["show_wolt_plus"]) || boolValue(venue["wolt_plus"]) ||
		boolValue(venueRaw["show_wolt_plus"]) || boolValue(venueRaw["wolt_plus"]) {
		return true
	}
	if payloadHasWoltPlusText(payload) || payloadHasWoltPlusText(venue) || payloadHasWoltPlusText(venueRaw) {
		return true
	}
	return false
}

func payloadHasWoltPlusText(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	candidates := []string{
		stringFromAny(payload["icon"]),
		stringFromAny(payload["badge"]),
		stringFromAny(payload["badge_text"]),
	}
	for _, candidate := range candidates {
		if isWoltPlusText(candidate) {
			return true
		}
	}
	for _, key := range []string{"badges", "telemetry_venue_badges", "tags"} {
		for _, rawValue := range toSlice(payload[key]) {
			valueMap := toMap(rawValue)
			if valueMap != nil {
				if isWoltPlusText(stringFromAny(valueMap["text"])) ||
					isWoltPlusText(stringFromAny(valueMap["title"])) ||
					isWoltPlusText(stringFromAny(valueMap["name"])) ||
					isWoltPlusText(stringFromAny(valueMap["variant"])) {
					return true
				}
				continue
			}
			if isWoltPlusText(stringFromAny(rawValue)) {
				return true
			}
		}
	}
	return false
}

// BuildItemSearchResult normalizes item search and fallback data.
func BuildItemSearchResult(
	query string,
	payloads []map[string]any,
	sortMode ItemSort,
	category string,
	limit *int,
	offset int,
	fallbackItems []domain.Item,
) (map[string]any, []string) {
	warnings := []string{}
	menuItems := []map[string]any{}
	loweredQuery := strings.ToLower(strings.TrimSpace(query))
	loweredCategory := strings.ToLower(strings.TrimSpace(category))

	for _, payload := range payloads {
		menuItems = append(menuItems, ExtractMenuItems(payload, "", "")...)
	}

	filtered := []map[string]any{}
	for _, item := range menuItems {
		if strings.Contains(strings.ToLower(stringFromAny(item["name"])), loweredQuery) {
			filtered = append(filtered, item)
		}
	}
	menuItems = filtered

	if loweredCategory != "" {
		filtered = []map[string]any{}
		for _, item := range menuItems {
			if strings.Contains(strings.ToLower(stringFromAny(item["category"])), loweredCategory) {
				filtered = append(filtered, item)
			}
		}
		menuItems = filtered
	}

	if len(menuItems) == 0 && len(fallbackItems) > 0 {
		warnings = append(warnings, "item-level search is unavailable upstream; returning venue-level placeholders")
		for _, item := range fallbackItems {
			if item.Venue == nil {
				continue
			}
			if !strings.Contains(strings.ToLower(item.Title), loweredQuery) {
				continue
			}
			menuItems = append(menuItems, map[string]any{
				"item_id":    item.TrackID,
				"venue_id":   domain.NormalizeID(coalesce(item.Venue.ID, item.Link.Target)),
				"venue_slug": item.Venue.Slug,
				"name":       item.Title,
				"base_price": map[string]any{
					"amount":           nil,
					"currency":         emptyToNil(item.Venue.Currency),
					"formatted_amount": nil,
				},
				"category":    "venue",
				"is_sold_out": false,
			})
		}
	}

	switch sortMode {
	case ItemSortPrice:
		sort.SliceStable(menuItems, func(i, j int) bool {
			left := intValue(toMap(menuItems[i]["base_price"])["amount"])
			right := intValue(toMap(menuItems[j]["base_price"])["amount"])
			return left < right
		})
	case ItemSortName:
		sort.SliceStable(menuItems, func(i, j int) bool {
			return strings.ToLower(stringFromAny(menuItems[i]["name"])) < strings.ToLower(stringFromAny(menuItems[j]["name"]))
		})
	}

	total := len(menuItems)
	if offset > 0 {
		if offset >= len(menuItems) {
			menuItems = []map[string]any{}
		} else {
			menuItems = menuItems[offset:]
		}
	}
	menuItems = limitSlice(menuItems, limit)

	rows := make([]map[string]any, 0, len(menuItems))
	for _, item := range menuItems {
		basePrice := toMap(item["base_price"])
		rows = append(rows, map[string]any{
			"item_id":     item["item_id"],
			"venue_id":    coalesce(item["venue_id"], ""),
			"venue_slug":  coalesce(item["venue_slug"], ""),
			"name":        item["name"],
			"base_price":  basePrice,
			"currency":    basePrice["currency"],
			"is_sold_out": boolValue(item["is_sold_out"]),
		})
	}

	return map[string]any{
		"query": query,
		"total": total,
		"items": rows,
	}, warnings
}

// BuildItemDetail returns normalized item details for the item show command.
func BuildItemDetail(itemID string, venueID string, payload map[string]any, includeUpsell bool) (map[string]any, []string) {
	warnings := []string{}
	menuItems := ExtractMenuItems(payload, venueID, "")

	var sourceItem map[string]any
	for _, item := range menuItems {
		if stringFromAny(item["item_id"]) == itemID {
			sourceItem = item
			break
		}
	}
	if sourceItem == nil {
		warnings = append(warnings, "item payload did not contain a complete menu entry; returning minimal details")
		sourceItem = map[string]any{
			"item_id":     itemID,
			"name":        itemID,
			"description": "",
			"base_price": map[string]any{
				"amount":           nil,
				"currency":         nil,
				"formatted_amount": nil,
			},
		}
	}

	upsellItems := []map[string]any{}
	if includeUpsell {
		upsellItems = extractUpsellItems(payload)
	}

	data := map[string]any{
		"item_id":       itemID,
		"venue_id":      venueID,
		"name":          sourceItem["name"],
		"description":   coalesce(sourceItem["description"], ""),
		"price":         sourceItem["base_price"],
		"option_groups": extractOptionGroups(payload),
		"upsell_items":  upsellItems,
	}
	return data, warnings
}
