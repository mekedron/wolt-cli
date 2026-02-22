package observability

import (
	"fmt"
	"sort"
	"strings"
)

func walkObjects(node any) []map[string]any {
	objects := []map[string]any{}
	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			objects = append(objects, v)
			for _, nested := range v {
				walk(nested)
			}
		case []any:
			for _, nested := range v {
				walk(nested)
			}
		}
	}
	walk(node)
	return objects
}

func toMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func toSlice(value any) []any {
	if value == nil {
		return nil
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return nil
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func extractAmount(node map[string]any) *int {
	for _, key := range []string{"base_price", "price_int", "amount", "minor_units"} {
		if value, ok := node[key]; ok {
			switch t := value.(type) {
			case float64:
				amount := int(t)
				return &amount
			case int:
				amount := t
				return &amount
			}
		}
	}
	for _, key := range []string{"price", "basePrice", "base_price"} {
		if value, ok := node[key]; ok {
			switch t := value.(type) {
			case map[string]any:
				if nested := extractAmount(t); nested != nil {
					return nested
				}
			case float64:
				amount := int(t)
				return &amount
			case int:
				amount := t
				return &amount
			}
		}
	}
	return nil
}

func extractCurrency(node map[string]any) string {
	for _, key := range []string{"currency", "currency_code", "currencyCode"} {
		if value, ok := node[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, key := range []string{"price", "basePrice", "base_price", "original_price", "unit_price"} {
		if nestedMap := toMap(node[key]); nestedMap != nil {
			if nested := extractCurrency(nestedMap); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func extractOriginalAmount(node map[string]any) *int {
	for _, key := range []string{"original_price", "originalPrice", "list_price", "regular_price"} {
		if value, ok := node[key]; ok {
			if amount := extractAmountValue(value); amount != nil {
				return amount
			}
		}
	}
	for _, key := range []string{"price", "basePrice", "base_price"} {
		if nestedMap := toMap(node[key]); nestedMap != nil {
			if amount := extractOriginalAmount(nestedMap); amount != nil {
				return amount
			}
		}
	}
	return nil
}

func extractAmountValue(value any) *int {
	switch typed := value.(type) {
	case int:
		amount := typed
		return &amount
	case float64:
		amount := int(typed)
		return &amount
	case map[string]any:
		return extractAmount(typed)
	default:
		return nil
	}
}

func buildDerivedPriceDiscountLabel(originalAmount int, currentAmount int, currency string) string {
	if originalAmount <= 0 || currentAmount < 0 || originalAmount <= currentAmount {
		return ""
	}
	discountAmount := originalAmount - currentAmount
	discountPercent := int((float64(discountAmount)/float64(originalAmount))*100 + 0.5)
	if formattedOriginal := formatAmount(&originalAmount, currency); formattedOriginal != nil {
		if discountPercent > 0 {
			return fmt.Sprintf("%d%% off (was %s)", discountPercent, *formattedOriginal)
		}
		return fmt.Sprintf("discounted from %s", *formattedOriginal)
	}
	if discountPercent > 0 {
		return fmt.Sprintf("%d%% off", discountPercent)
	}
	return "discounted"
}

func extractOptionGroupIDs(node map[string]any) []string {
	if ids := toSlice(node["option_group_ids"]); ids != nil {
		out := make([]string, 0, len(ids))
		for _, value := range ids {
			if value == nil {
				continue
			}
			out = append(out, stringFromAny(value))
		}
		return out
	}

	groups := toSlice(node["option_groups"])
	if groups != nil {
		ids := make([]string, 0, len(groups))
		for _, group := range groups {
			groupMap := toMap(group)
			if groupMap == nil {
				continue
			}
			id := groupMap["group_id"]
			if id == nil {
				id = groupMap["id"]
			}
			if id == nil {
				continue
			}
			ids = append(ids, stringFromAny(id))
		}
		return ids
	}

	options := toSlice(node["options"])
	if options == nil {
		return []string{}
	}
	ids := make([]string, 0, len(options))
	for _, option := range options {
		optionMap := toMap(option)
		if optionMap == nil {
			continue
		}
		id := optionMap["option_id"]
		if id == nil {
			id = optionMap["id"]
		}
		if id == nil {
			continue
		}
		ids = append(ids, stringFromAny(id))
	}
	return ids
}

func extractOptionGroups(node any) []map[string]any {
	groups := []map[string]any{}
	for _, obj := range walkObjects(node) {
		groupList := toSlice(obj["option_groups"])
		if groupList == nil {
			continue
		}
		for _, value := range groupList {
			group := toMap(value)
			if group == nil {
				continue
			}
			id := group["group_id"]
			if id == nil {
				id = group["id"]
			}
			name, ok := group["name"].(string)
			if !ok {
				name, ok = group["title"].(string)
			}
			if !ok || id == nil {
				continue
			}
			groups = append(groups, map[string]any{
				"group_id": stringFromAny(id),
				"name":     name,
				"required": boolValue(group["required"]),
				"min":      intValue(group["min"]),
				"max":      intValue(group["max"]),
			})
		}
	}
	byID := map[string]map[string]any{}
	for _, group := range groups {
		byID[group["group_id"].(string)] = group
	}
	out := make([]map[string]any, 0, len(byID))
	for _, group := range byID {
		out = append(out, group)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["group_id"].(string) < out[j]["group_id"].(string)
	})
	return out
}

func extractUpsellItems(node any) []map[string]any {
	candidateKeys := []string{"upsell_items", "related_items", "recommended_items"}
	upsell := []map[string]any{}
	for _, obj := range walkObjects(node) {
		for _, key := range candidateKeys {
			values := toSlice(obj[key])
			if values == nil {
				continue
			}
			for _, rawItem := range values {
				item := toMap(rawItem)
				if item == nil {
					continue
				}
				itemID := item["item_id"]
				if itemID == nil {
					itemID = item["id"]
				}
				name, ok := item["name"].(string)
				if !ok {
					name, ok = item["title"].(string)
				}
				if itemID == nil || !ok {
					continue
				}
				amount := extractAmount(item)
				currency := extractCurrency(item)
				var formatted any
				if value := formatAmount(amount, currency); value != nil {
					formatted = *value
				}
				var amountValue any
				if amount != nil {
					amountValue = *amount
				}
				upsell = append(upsell, map[string]any{
					"item_id": stringFromAny(itemID),
					"name":    name,
					"price": map[string]any{
						"amount":           amountValue,
						"formatted_amount": formatted,
					},
				})
			}
		}
	}
	byID := map[string]map[string]any{}
	for _, item := range upsell {
		byID[item["item_id"].(string)] = item
	}
	out := make([]map[string]any, 0, len(byID))
	for _, item := range byID {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["item_id"].(string) < out[j]["item_id"].(string)
	})
	return out
}

func boolValue(v any) bool {
	if value, ok := v.(bool); ok {
		return value
	}
	return false
}

func intValue(v any) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func hasAnyKeys(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func isOptionLikeObject(obj map[string]any) bool {
	if _, hasOptionID := obj["option_id"]; hasOptionID {
		return true
	}
	if _, hasValues := obj["values"]; hasValues {
		if !hasAnyKeys(
			obj,
			"item_id",
			"options",
			"option_groups",
			"option_group_ids",
			"is_cutlery",
			"allowed_delivery_methods",
			"description",
			"disabled_info",
			"vat_percentage",
		) {
			return true
		}
	}
	if _, hasMultiChoice := obj["multi_choice_config"]; hasMultiChoice {
		if !hasAnyKeys(
			obj,
			"item_id",
			"options",
			"option_groups",
			"option_group_ids",
			"is_cutlery",
			"allowed_delivery_methods",
			"description",
			"disabled_info",
			"vat_percentage",
		) {
			return true
		}
	}
	return false
}

// ExtractMenuItems walks payload and normalizes menu-like entries.
func ExtractMenuItems(payload map[string]any, venueID string, venueSlug string) []map[string]any {
	items := []map[string]any{}
	seen := map[string]struct{}{}
	itemCategoryMap := categoryByItemID(payload)

	for _, obj := range walkObjects(payload) {
		itemID := obj["item_id"]
		if itemID == nil {
			itemID = obj["id"]
		}
		name, ok := obj["name"].(string)
		if !ok {
			name, ok = obj["title"].(string)
		}
		if itemID == nil || !ok {
			continue
		}

		signalKeys := []string{"option_group_ids", "option_groups", "base_price", "price", "is_sold_out", "sold_out", "item_id"}
		if hasAnyKeys(obj, "options") {
			signalKeys = append(signalKeys, "options")
		}
		if !hasAnyKeys(obj, signalKeys...) {
			continue
		}
		if isOptionLikeObject(obj) {
			continue
		}

		resolvedItemID := stringFromAny(itemID)
		resolvedVenueID := stringFromAny(coalesce(obj["venue_id"], venueID))
		resolvedVenueSlug := stringFromAny(coalesce(obj["venue_slug"], venueSlug))
		dedupeKey := strings.Join([]string{resolvedItemID, name, resolvedVenueID}, "|")
		if _, ok := seen[dedupeKey]; ok {
			continue
		}
		seen[dedupeKey] = struct{}{}

		amount := extractAmount(obj)
		currency := extractCurrency(obj)
		originalAmount := extractOriginalAmount(obj)
		categoryName := stringFromAny(coalesce(
			obj["category_name"],
			obj["category"],
			obj["section_name"],
			itemCategoryMap[resolvedItemID],
			"uncategorized",
		))
		isSoldOut := boolValue(coalesce(obj["is_sold_out"], obj["sold_out"]))

		var formatted any
		if value := formatAmount(amount, currency); value != nil {
			formatted = *value
		}
		var amountValue any
		if amount != nil {
			amountValue = *amount
		}
		var originalAmountValue any
		var originalFormatted any
		if originalAmount != nil {
			originalAmountValue = *originalAmount
			if value := formatAmount(originalAmount, currency); value != nil {
				originalFormatted = *value
			}
		}

		description := ""
		if value, ok := obj["description"].(string); ok {
			description = value
		}

		discounts := extractDiscountLabels(obj)
		if amount != nil && originalAmount != nil {
			if derived := strings.TrimSpace(buildDerivedPriceDiscountLabel(*originalAmount, *amount, currency)); derived != "" {
				exists := false
				for _, rawLabel := range discounts {
					if strings.EqualFold(strings.TrimSpace(stringFromAny(rawLabel)), derived) {
						exists = true
						break
					}
				}
				if !exists {
					discounts = append(discounts, derived)
				}
			}
		}

		items = append(items, map[string]any{
			"item_id":     resolvedItemID,
			"venue_id":    resolvedVenueID,
			"venue_slug":  resolvedVenueSlug,
			"name":        name,
			"description": description,
			"base_price": map[string]any{
				"amount":           amountValue,
				"currency":         emptyToNil(currency),
				"formatted_amount": formatted,
			},
			"original_price": map[string]any{
				"amount":           originalAmountValue,
				"currency":         emptyToNil(currency),
				"formatted_amount": originalFormatted,
			},
			"option_group_ids": extractOptionGroupIDs(obj),
			"category":         categoryName,
			"is_sold_out":      isSoldOut,
			"discounts":        discounts,
		})
	}

	return items
}

func extractDiscountLabels(node map[string]any) []string {
	out := []string{}
	seen := map[string]struct{}{}
	appendLabel := func(raw string) {
		normalized := strings.TrimSpace(raw)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	for _, key := range []string{
		"promotions",
		"promotion",
		"discounts",
		"discount",
		"offers",
		"offer",
		"campaigns",
	} {
		appendDiscountLabels(node[key], appendLabel)
	}
	for _, key := range []string{"discount_text", "promotion_text", "offer_text"} {
		appendLabel(stringFromAny(node[key]))
	}

	for _, rawBadge := range toSlice(node["badges"]) {
		badge := toMap(rawBadge)
		if badge == nil {
			continue
		}
		variant := strings.ToLower(strings.TrimSpace(stringFromAny(badge["variant"])))
		if strings.Contains(variant, "discount") || strings.Contains(variant, "promotion") {
			appendLabel(firstDiscountText(badge))
		}
	}
	return out
}

func appendDiscountLabels(value any, appendLabel func(string)) {
	switch typed := value.(type) {
	case string:
		appendLabel(typed)
	case []any:
		for _, nested := range typed {
			appendDiscountLabels(nested, appendLabel)
		}
	case map[string]any:
		appendLabel(firstDiscountText(typed))
		for _, key := range []string{"items", "values", "promotions", "discounts", "offers", "labels"} {
			appendDiscountLabels(typed[key], appendLabel)
		}
	case map[string]string:
		appendLabel(firstDiscountTextFromStringMap(typed))
	}
}

func firstDiscountText(payload map[string]any) string {
	for _, key := range []string{"text", "title", "name", "label", "description"} {
		value := strings.TrimSpace(stringFromAny(payload[key]))
		if value != "" {
			return value
		}
	}
	return ""
}

func firstDiscountTextFromStringMap(payload map[string]string) string {
	for _, key := range []string{"text", "title", "name", "label", "description"} {
		value := strings.TrimSpace(payload[key])
		if value != "" {
			return value
		}
	}
	return ""
}

func categoryByItemID(payload map[string]any) map[string]any {
	out := map[string]any{}
	categories := toSlice(payload["categories"])
	for _, rawCategory := range categories {
		category := toMap(rawCategory)
		if category == nil {
			continue
		}
		categoryName := strings.TrimSpace(stringFromAny(coalesce(category["name"], category["slug"], category["id"])))
		if categoryName == "" {
			continue
		}
		for _, rawItemID := range toSlice(category["item_ids"]) {
			itemID := strings.TrimSpace(stringFromAny(rawItemID))
			if itemID == "" {
				continue
			}
			if _, exists := out[itemID]; exists {
				continue
			}
			out[itemID] = categoryName
		}
	}
	return out
}

func emptyToNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
