package observability

import (
	"math"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// BuildVenueMenu builds normalized venue menu payload.
func BuildVenueMenu(venueID string, payloads []map[string]any, category string, includeOptions bool, limit *int) (map[string]any, []string) {
	warnings := []string{}
	menuItems := []map[string]any{}
	isWoltPlus := false
	fallbackCurrency := resolvePayloadCurrency(payloads)
	campaignDiscounts := collectCampaignItemDiscounts(payloads)

	for _, payload := range payloads {
		menuItems = append(menuItems, ExtractMenuItems(payload, venueID, "")...)
		if !isWoltPlus && payloadVenueWoltPlus(payload) {
			isWoltPlus = true
		}
	}
	menuItems = dedupeMenuItemsByID(menuItems)

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
		basePrice := normalizeBasePrice(toMap(item["base_price"]), fallbackCurrency)
		originalPrice := normalizeBasePrice(toMap(item["original_price"]), fallbackCurrency)
		itemID := strings.TrimSpace(stringFromAny(item["item_id"]))
		discountLabels := labelsFromAny(item["discounts"])
		if campaign, ok := campaignDiscounts[itemID]; ok {
			discountLabels = mergeStringLabels(discountLabels, campaign.labels)
			applyCampaignPriceFraction(basePrice, originalPrice, campaign.maxFraction)
		}
		row := map[string]any{
			"item_id":     item["item_id"],
			"name":        item["name"],
			"base_price":  basePrice,
			"discounts":   discountLabels,
			"is_sold_out": boolValue(item["is_sold_out"]),
		}
		if intValue(originalPrice["amount"]) > 0 {
			row["original_price"] = originalPrice
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

func dedupeMenuItemsByID(items []map[string]any) []map[string]any {
	seen := map[string]struct{}{}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		itemID := strings.TrimSpace(stringFromAny(item["item_id"]))
		if itemID == "" {
			continue
		}
		if _, ok := seen[itemID]; ok {
			continue
		}
		seen[itemID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func resolvePayloadCurrency(payloads []map[string]any) string {
	for _, payload := range payloads {
		candidates := []any{
			payload["currency"],
			payload["currency_code"],
			toMap(payload["venue"])["currency"],
			toMap(toMap(payload["venue"])["price"])["currency"],
			toMap(payload["venue_raw"])["currency"],
			toMap(toMap(payload["venue_raw"])["price"])["currency"],
		}
		for _, candidate := range candidates {
			currency := strings.TrimSpace(stringFromAny(candidate))
			if currency == "" {
				continue
			}
			return currency
		}
	}
	return ""
}

func normalizeBasePrice(basePrice map[string]any, fallbackCurrency string) map[string]any {
	if basePrice == nil {
		basePrice = map[string]any{}
	}
	normalized := map[string]any{}
	for key, value := range basePrice {
		normalized[key] = value
	}

	currency := strings.TrimSpace(stringFromAny(normalized["currency"]))
	if currency == "" {
		currency = strings.TrimSpace(fallbackCurrency)
	}
	if currency != "" {
		normalized["currency"] = currency
	}

	hasFormattedAmount := strings.TrimSpace(stringFromAny(normalized["formatted_amount"])) != ""
	if !hasFormattedAmount && currency != "" {
		if _, exists := normalized["amount"]; exists {
			amount := intValue(normalized["amount"])
			if formatted := formatAmount(&amount, currency); formatted != nil {
				normalized["formatted_amount"] = *formatted
			}
		}
	}
	return normalized
}

func applyCampaignPriceFraction(basePrice map[string]any, originalPrice map[string]any, fraction float64) {
	if fraction <= 0 || fraction >= 1 || basePrice == nil {
		return
	}
	currentAmount := intValue(basePrice["amount"])
	if currentAmount <= 0 {
		return
	}

	if originalPrice == nil {
		originalPrice = map[string]any{}
	}
	if intValue(originalPrice["amount"]) <= 0 {
		originalPrice["amount"] = currentAmount
		if currency := strings.TrimSpace(stringFromAny(coalesce(originalPrice["currency"], basePrice["currency"]))); currency != "" {
			originalPrice["currency"] = currency
			if formatted := formatAmount(&currentAmount, currency); formatted != nil {
				originalPrice["formatted_amount"] = *formatted
			}
		}
	}

	discountedAmount := int(math.Round(float64(currentAmount) * (1 - fraction)))
	if discountedAmount < 0 {
		discountedAmount = 0
	}
	basePrice["amount"] = discountedAmount
	currency := strings.TrimSpace(stringFromAny(basePrice["currency"]))
	if currency == "" {
		currency = strings.TrimSpace(stringFromAny(originalPrice["currency"]))
		if currency != "" {
			basePrice["currency"] = currency
		}
	}
	if currency != "" {
		if formatted := formatAmount(&discountedAmount, currency); formatted != nil {
			basePrice["formatted_amount"] = *formatted
		}
	}
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
	fallbackCurrency := resolvePayloadCurrency(payloads)

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
		basePrice := normalizeBasePrice(toMap(item["base_price"]), fallbackCurrency)
		originalPrice := normalizeBasePrice(toMap(item["original_price"]), fallbackCurrency)
		rows = append(rows, map[string]any{
			"item_id":        item["item_id"],
			"venue_id":       coalesce(item["venue_id"], ""),
			"venue_slug":     coalesce(item["venue_slug"], ""),
			"name":           item["name"],
			"base_price":     basePrice,
			"original_price": originalPrice,
			"discounts":      item["discounts"],
			"currency":       basePrice["currency"],
			"is_sold_out":    boolValue(item["is_sold_out"]),
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
	fallbackCurrency := resolvePayloadCurrency([]map[string]any{payload})

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
		for _, upsell := range upsellItems {
			upsell["price"] = normalizeBasePrice(toMap(upsell["price"]), fallbackCurrency)
		}
	}
	price := normalizeBasePrice(toMap(sourceItem["base_price"]), fallbackCurrency)

	data := map[string]any{
		"item_id":       itemID,
		"venue_id":      venueID,
		"name":          sourceItem["name"],
		"description":   coalesce(sourceItem["description"], ""),
		"price":         price,
		"option_groups": extractOptionGroups(payload),
		"upsell_items":  upsellItems,
	}
	return data, warnings
}
