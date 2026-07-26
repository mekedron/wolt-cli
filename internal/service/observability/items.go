package observability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func walkObjects(node any) []map[string]any {
	objects := []map[string]any{}
	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			objects = append(objects, v)
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(v[key])
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
	ids := []string{}
	seen := map[string]struct{}{}
	appendID := func(raw any) {
		id := strings.TrimSpace(stringFromAny(raw))
		key := strings.ToLower(id)
		if id == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		ids = append(ids, id)
	}
	for _, value := range toSlice(node["option_group_ids"]) {
		appendID(value)
	}
	for _, alias := range []string{"option_groups", "options"} {
		for _, rawGroup := range toSlice(node[alias]) {
			group := toMap(rawGroup)
			if group == nil {
				continue
			}
			appendID(coalesce(group["group_id"], group["option_id"], group["id"]))
		}
	}
	return ids
}

func extractOptionGroups(node any, itemID string) []map[string]any {
	payload := toMap(node)
	if payload == nil {
		return []map[string]any{}
	}
	target := catalogitem.Find(payload, itemID)
	if target == nil {
		rootID := strings.TrimSpace(stringFromAny(coalesce(payload["item_id"], payload["id"])))
		if strings.EqualFold(rootID, strings.TrimSpace(itemID)) {
			target = payload
		}
	}
	if target == nil {
		return []map[string]any{}
	}

	referenced := map[string]struct{}{}
	for _, groupID := range extractOptionGroupIDs(target) {
		referenced[strings.ToLower(groupID)] = struct{}{}
	}
	groups := payloadutil.MergeOptionGroups(
		toSlice(target["options"]),
		toSlice(target["option_groups"]),
	)
	rootGroups := payloadutil.MergeOptionGroups(
		toSlice(payload["options"]),
		toSlice(payload["option_groups"]),
	)
	for _, rawGroup := range rootGroups {
		groupID := rawOptionGroupID(rawGroup)
		if _, wanted := referenced[strings.ToLower(groupID)]; !wanted {
			continue
		}
		groups = payloadutil.MergeOptionGroups(groups, []any{rawGroup})
	}
	specs := payloadutil.ExtractOptionSpecs(map[string]any{"option_groups": groups})
	groupIDs := make([]string, 0, len(specs))
	for groupID := range specs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	out := make([]map[string]any, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		spec := specs[groupID]
		valueIDs := make([]string, 0, len(spec.Values))
		for valueID := range spec.Values {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)
		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			valueSpec := spec.Values[valueID]
			value := map[string]any{
				"value_id": valueID,
				"name":     valueSpec.Name,
			}
			if valueSpec.HasPrice {
				value["price"] = valueSpec.Price
			}
			values = append(values, value)
		}
		out = append(out, map[string]any{
			"group_id": groupID,
			"name":     spec.Name,
			"required": spec.Required,
			"min":      spec.MinSelect,
			"max":      spec.MaxSelect,
			"values":   values,
		})
	}
	return out
}

func rawOptionGroupID(raw any) string {
	group := toMap(raw)
	return strings.TrimSpace(stringFromAny(coalesce(
		group["id"],
		group["group_id"],
		group["option_id"],
	)))
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

		signalKeys := []string{
			"option_group_ids",
			"option_groups",
			"base_price",
			"price",
			"disabled_info",
			"purchasable_balance",
			"item_id",
		}
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
		categoryContext := categoryContextForItem(obj, itemCategoryMap[resolvedItemID])
		availability := catalogitem.ResolveAvailability(obj)
		imageURLs := catalogitem.ImageURLs(obj)
		var imageURL any
		if len(imageURLs) > 0 {
			imageURL = imageURLs[0]
		}

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

		item := map[string]any{
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
			"category":         categoryContext.Label,
			"is_available":     availability.IsAvailable,
			"unavailable_reason": emptyToNil(
				availability.Reason,
			),
			// Deprecated compatibility alias. The value is derived from the
			// current Wolt disabled_info / purchasable_balance schema; raw
			// is_sold_out and sold_out fields are no longer consulted.
			"is_sold_out":           !availability.IsAvailable,
			"purchasable_balance":   availability.PurchasableBalance,
			"image_url":             imageURL,
			"image_urls":            imageURLs,
			"image_blurhash":        emptyToNil(catalogitem.ImageBlurhash(obj)),
			"unit_info":             obj["unit_info"],
			"unit_price":            obj["unit_price"],
			"sell_by_weight_config": obj["sell_by_weight_config"],
			"discounts":             discounts,
		}
		if categoryContext.ID != "" {
			item["category_id"] = categoryContext.ID
		}
		if categoryContext.Name != "" {
			item["category_name"] = categoryContext.Name
		}
		if categoryContext.Slug != "" {
			item["category_slug"] = categoryContext.Slug
		}
		copyItemLanguageVariants(item, obj)
		items = append(items, item)
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

type itemCategoryContext struct {
	ID    string
	Slug  string
	Name  string
	Label string
}

func categoryContextForItem(item map[string]any, fallback itemCategoryContext) itemCategoryContext {
	category := toMap(item["category"])
	categoryID := firstNonEmptyValue(
		stringFromAny(item["category_id"]),
		stringFromAny(item["categoryId"]),
		stringFromAny(category["id"]),
		stringFromAny(category["_id"]),
	)
	if categoryID == "" {
		for _, rawID := range toSlice(item["category_ids"]) {
			if categoryID = strings.TrimSpace(stringFromAny(rawID)); categoryID != "" {
				break
			}
		}
	}
	if categoryID == "" {
		categoryID = fallback.ID
	}

	categorySlug := firstNonEmptyValue(
		stringFromAny(item["category_slug"]),
		stringFromAny(item["categorySlug"]),
		stringFromAny(category["slug"]),
		fallback.Slug,
	)
	categoryName := firstNonEmptyValue(
		stringFromAny(item["category_name"]),
		stringFromAny(item["categoryName"]),
		stringFromAny(category["name"]),
		stringFromAny(category["title"]),
		fallback.Name,
	)
	legacyCategory := ""
	if category == nil {
		legacyCategory = strings.TrimSpace(stringFromAny(item["category"]))
	}
	label := firstNonEmptyValue(
		categoryName,
		legacyCategory,
		stringFromAny(item["section_name"]),
		fallback.Label,
		categoryID,
		"uncategorized",
	)
	return itemCategoryContext{ID: categoryID, Slug: categorySlug, Name: categoryName, Label: label}
}

func categoryByItemID(payload map[string]any) map[string]itemCategoryContext {
	out := map[string]itemCategoryContext{}
	walkPayloadCategories(payload, func(category map[string]any) {
		context := categoryContextFromPayload(category)
		for _, itemID := range categoryItemIDs(category) {
			out[itemID] = mergeCategoryContext(out[itemID], context)
		}
	})
	return out
}

func categoryByID(payloads []map[string]any) map[string]itemCategoryContext {
	out := map[string]itemCategoryContext{}
	for _, payload := range payloads {
		walkPayloadCategories(payload, func(category map[string]any) {
			context := categoryContextFromPayload(category)
			if context.ID == "" {
				return
			}
			out[context.ID] = mergeCategoryContext(out[context.ID], context)
		})
	}
	return out
}

func enrichItemCategories(items []map[string]any, payloads []map[string]any) {
	categories := categoryByID(payloads)
	for _, item := range items {
		categoryID := strings.TrimSpace(stringFromAny(item["category_id"]))
		context, exists := categories[categoryID]
		if categoryID == "" || !exists {
			continue
		}
		categoryName := strings.TrimSpace(stringFromAny(item["category_name"]))
		if categoryName == "" && context.Name != "" {
			categoryName = context.Name
			item["category_name"] = categoryName
		}
		label := strings.TrimSpace(stringFromAny(item["category"]))
		if categoryName != "" &&
			(label == "" || strings.EqualFold(label, categoryID) || strings.EqualFold(label, "uncategorized")) {
			item["category"] = categoryName
		}
		if strings.TrimSpace(stringFromAny(item["category_slug"])) == "" && context.Slug != "" {
			item["category_slug"] = context.Slug
		}
	}
}

func categoryContextFromPayload(category map[string]any) itemCategoryContext {
	categoryID := firstNonEmptyValue(
		stringFromAny(category["category_id"]),
		stringFromAny(category["id"]),
		stringFromAny(category["_id"]),
	)
	categoryName := firstNonEmptyValue(
		stringFromAny(category["category_name"]),
		stringFromAny(category["name"]),
		stringFromAny(category["title"]),
	)
	categorySlug := strings.TrimSpace(stringFromAny(category["slug"]))
	return itemCategoryContext{
		ID:   categoryID,
		Slug: categorySlug,
		Name: categoryName,
		Label: firstNonEmptyValue(
			categoryName,
			categorySlug,
			categoryID,
		),
	}
}

func mergeCategoryContext(existing itemCategoryContext, incoming itemCategoryContext) itemCategoryContext {
	if existing.ID == "" {
		existing.ID = incoming.ID
	}
	if existing.Slug == "" {
		existing.Slug = incoming.Slug
	}
	if existing.Name == "" {
		existing.Name = incoming.Name
	}
	if existing.Label == "" {
		existing.Label = incoming.Label
	}
	return existing
}

func walkPayloadCategories(payload map[string]any, visit func(map[string]any)) {
	var walkCategory func(any)
	walkCategory = func(raw any) {
		category := toMap(raw)
		if category == nil {
			return
		}
		// Descendants remain authoritative when a parent repeats their items.
		for _, key := range []string{"categories", "subcategories"} {
			for _, child := range toSlice(category[key]) {
				walkCategory(child)
			}
		}
		visit(category)
	}

	var scan func(any)
	scan = func(raw any) {
		switch value := raw.(type) {
		case map[string]any:
			// A singular category is the endpoint-selected context and takes
			// precedence over broader category collections. Slice order remains
			// authoritative within each collection.
			if category, exists := value["category"]; exists {
				walkCategory(category)
			}
			for _, key := range []string{"categories", "subcategories"} {
				for _, category := range toSlice(value[key]) {
					walkCategory(category)
				}
			}
			keys := make([]string, 0, len(value))
			for key := range value {
				switch key {
				case "category", "categories", "subcategories":
					continue
				}
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				scan(value[key])
			}
		case []any:
			for _, nested := range value {
				scan(nested)
			}
		}
	}
	scan(payload)
}

func categoryItemIDs(category map[string]any) []string {
	out := []string{}
	seen := map[string]struct{}{}
	appendID := func(raw any) {
		itemID := strings.TrimSpace(stringFromAny(raw))
		if itemID == "" {
			return
		}
		if _, exists := seen[itemID]; exists {
			return
		}
		seen[itemID] = struct{}{}
		out = append(out, itemID)
	}
	for _, rawItemID := range toSlice(category["item_ids"]) {
		appendID(rawItemID)
	}
	for _, rawItem := range toSlice(category["items"]) {
		if item := toMap(rawItem); item != nil {
			appendID(coalesce(item["item_id"], item["id"]))
			continue
		}
		appendID(rawItem)
	}
	return out
}

func copyItemLanguageVariants(target map[string]any, source map[string]any) {
	for key, value := range source {
		if value == nil || !isItemLanguageVariantKey(key) {
			continue
		}
		target[key] = value
	}
}

func isItemLanguageVariantKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "translations",
		"localizations",
		"name_translations",
		"description_translations",
		"original_name",
		"localized_name",
		"name_original",
		"name_localized",
		"original_description",
		"localized_description",
		"description_original",
		"description_localized":
		return true
	}
	for _, prefix := range []string{"name_", "description_"} {
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(normalized, prefix)
		primary := strings.Split(strings.ReplaceAll(suffix, "_", "-"), "-")[0]
		if len(primary) < 2 || len(primary) > 3 {
			return false
		}
		for _, char := range primary {
			if char < 'a' || char > 'z' {
				return false
			}
		}
		return true
	}
	return false
}

// ItemMatchesQuery reports whether an item contains a query in its
// upstream-provided display text or language variants. It never synthesizes
// translations. Dedicated upstream search results are already authoritative
// matches and should not be filtered again with this helper.
func ItemMatchesQuery(item map[string]any, query string) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return true
	}
	for _, key := range []string{"name", "description"} {
		if queryTextContains(item[key], needle) {
			return true
		}
	}
	for key, value := range item {
		if isItemLanguageVariantKey(key) && queryTextContains(value, needle) {
			return true
		}
	}
	return false
}

func queryTextContains(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), needle)
	case map[string]any:
		for _, nested := range typed {
			if queryTextContains(nested, needle) {
				return true
			}
		}
	case map[string]string:
		for _, nested := range typed {
			if queryTextContains(nested, needle) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if queryTextContains(nested, needle) {
				return true
			}
		}
	}
	return false
}

func emptyToNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
