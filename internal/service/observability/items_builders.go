package observability

import (
	"math"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// BuildVenueMenu builds normalized venue menu payload.
func BuildVenueMenu(
	venueID string,
	payloads []map[string]any,
	category string,
	includeOptions bool,
	limit *int,
	contexts ...ItemVenueContext,
) (map[string]any, []string) {
	warnings := []string{}
	menuItems := []map[string]any{}
	isWoltPlus := false
	context := firstItemVenueContext(contexts)
	contextPayloads := make([]map[string]any, 0, len(payloads)+len(context.MetadataPayloads))
	contextPayloads = append(contextPayloads, payloads...)
	contextPayloads = append(contextPayloads, context.MetadataPayloads...)
	itemContext := resolveItemVenueContext(ItemVenueContext{VenueID: venueID}, contextPayloads, context)
	fallbackCurrency := firstNonEmptyValue(resolvePayloadCurrency(contextPayloads), itemContext.Currency)
	campaignDiscounts := collectCampaignItemDiscounts(contextPayloads)

	for _, payload := range payloads {
		menuItems = append(
			menuItems,
			ExtractMenuItems(payload, itemContext.VenueID, itemContext.VenueSlug)...,
		)
	}
	for _, payload := range contextPayloads {
		if !isWoltPlus && payloadVenueWoltPlus(payload) {
			isWoltPlus = true
		}
	}
	menuItems = dedupeMenuItemsByID(menuItems)
	enrichItemCategories(menuItems, contextPayloads)
	menuItems = filterItemsByCategory(menuItems, category)

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
		row := buildItemRow(item, itemContext, basePrice, originalPrice)
		row["discounts"] = discountLabels
		if intValue(originalPrice["amount"]) <= 0 {
			delete(row, "original_price")
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

	data := map[string]any{
		"venue_id":   itemContext.VenueID,
		"wolt_plus":  isWoltPlus,
		"categories": categories,
		"items":      rows,
	}
	if itemContext.VenueSlug != "" {
		data["venue_slug"] = itemContext.VenueSlug
	}
	if itemContext.CanonicalURL != "" {
		data["canonical_url"] = itemContext.CanonicalURL
	}
	if fallbackCurrency != "" {
		data["currency"] = fallbackCurrency
	}
	return data, warnings
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

func filterItemsByCategory(items []map[string]any, category string) []map[string]any {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		return items
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		for _, field := range []string{"category", "category_id", "category_slug", "category_name"} {
			if strings.EqualFold(strings.TrimSpace(stringFromAny(item[field])), category) {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
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
		if rawAmount, exists := normalized["amount"]; exists && rawAmount != nil {
			amount := intValue(rawAmount)
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
	if intValue(originalPrice["amount"]) > currentAmount {
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

// ItemVenueContext carries venue facts already resolved by the caller. The
// values are fallbacks: item and venue payload fields remain authoritative,
// and CanonicalURL is never synthesized from a slug.
type ItemVenueContext struct {
	VenueID               string
	VenueSlug             string
	CanonicalURL          string
	Currency              string
	IncludeOptionGroupIDs bool
	// AvailabilityVerified tells item-detail builders whether an exact current
	// assortment lookup succeeded. A false value makes stale page availability
	// explicitly unknown instead of defaulting it to available.
	AvailabilityVerified *bool
	// MetadataPayloads contribute venue-level context without contributing
	// item rows. This keeps endpoint-specific selections authoritative.
	MetadataPayloads []map[string]any
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
	contexts ...ItemVenueContext,
) (map[string]any, []string) {
	warnings := []string{}
	menuItems := []map[string]any{}
	context := firstItemVenueContext(contexts)
	contextPayloads := make([]map[string]any, 0, len(payloads)+len(context.MetadataPayloads))
	contextPayloads = append(contextPayloads, payloads...)
	contextPayloads = append(contextPayloads, context.MetadataPayloads...)
	itemContext := resolveItemVenueContext(ItemVenueContext{}, contextPayloads, context)
	fallbackCurrency := firstNonEmptyValue(resolvePayloadCurrency(contextPayloads), itemContext.Currency)

	for _, payload := range payloads {
		menuItems = append(menuItems, ExtractMenuItems(payload, itemContext.VenueID, itemContext.VenueSlug)...)
	}
	enrichItemCategories(menuItems, contextPayloads)
	menuItems = filterItemsByCategory(menuItems, category)

	if len(menuItems) == 0 && len(fallbackItems) > 0 {
		warnings = append(warnings, "item-level search is unavailable upstream; returning venue-level placeholders")
		loweredQuery := strings.ToLower(strings.TrimSpace(query))
		for _, item := range fallbackItems {
			if item.Venue == nil {
				continue
			}
			if !strings.Contains(strings.ToLower(item.Title), loweredQuery) {
				continue
			}
			menuItems = append(menuItems, map[string]any{
				"item_id":    item.TrackID,
				"venue_id":   discoveryVenueID(item),
				"venue_slug": item.Venue.Slug,
				"name":       item.Title,
				"base_price": map[string]any{
					"amount":           nil,
					"currency":         emptyToNil(item.Venue.Currency),
					"formatted_amount": nil,
				},
				"category":     "venue",
				"is_available": true,
				"is_sold_out":  false,
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
	if total == 0 {
		warnings = append(warnings, "no items matched this venue search query")
	}
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
		rows = append(rows, buildItemRow(item, itemContext, basePrice, originalPrice))
	}

	data := map[string]any{
		"query": query,
		"total": total,
		"items": rows,
	}
	if itemContext.VenueID != "" {
		data["venue_id"] = itemContext.VenueID
	}
	if itemContext.VenueSlug != "" {
		data["venue_slug"] = itemContext.VenueSlug
	}
	if itemContext.CanonicalURL != "" {
		data["canonical_url"] = itemContext.CanonicalURL
	}
	if fallbackCurrency != "" {
		data["currency"] = fallbackCurrency
	}
	return data, warnings
}

// BuildItemDetail returns normalized item details for the item show command.
func BuildItemDetail(
	itemID string,
	venueID string,
	payload map[string]any,
	includeUpsell bool,
	contexts ...ItemVenueContext,
) (map[string]any, []string) {
	warnings := []string{}
	context := firstItemVenueContext(contexts)
	itemContext := resolveItemVenueContext(ItemVenueContext{VenueID: venueID}, []map[string]any{payload}, context)
	menuItems := ExtractMenuItems(payload, itemContext.VenueID, itemContext.VenueSlug)
	fallbackCurrency := firstNonEmptyValue(resolvePayloadCurrency([]map[string]any{payload}), itemContext.Currency)

	var sourceItem map[string]any
	for _, item := range menuItems {
		if strings.EqualFold(
			strings.TrimSpace(stringFromAny(item["item_id"])),
			strings.TrimSpace(itemID),
		) {
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
		"venue_id":      firstNonEmptyValue(itemContext.VenueID, venueID),
		"name":          sourceItem["name"],
		"description":   coalesce(sourceItem["description"], ""),
		"price":         price,
		"currency":      price["currency"],
		"option_groups": extractOptionGroups(payload, itemID),
		"upsell_items":  upsellItems,
	}
	if itemContext.VenueSlug != "" {
		data["venue_slug"] = itemContext.VenueSlug
	}
	if itemContext.CanonicalURL != "" {
		data["canonical_url"] = itemContext.CanonicalURL
	}
	copyOptionalItemField(data, sourceItem, "category_id")
	copyOptionalItemField(data, sourceItem, "category_name")
	if category := strings.TrimSpace(stringFromAny(sourceItem["category"])); category != "" && category != "uncategorized" {
		data["category"] = category
	}
	copyItemLanguageVariants(data, sourceItem)
	CopyItemMetadata(data, sourceItem)
	if context.AvailabilityVerified != nil {
		data["availability_verified"] = *context.AvailabilityVerified
		if !*context.AvailabilityVerified {
			data["is_available"] = nil
			data["is_sold_out"] = nil
			data["unavailable_reason"] = nil
			data["purchasable_balance"] = nil
		}
	}
	return data, warnings
}

func resolveItemVenueContext(
	fallback ItemVenueContext,
	payloads []map[string]any,
	context ItemVenueContext,
) ItemVenueContext {
	out := fallback
	out.VenueID = firstNonEmptyValue(context.VenueID, out.VenueID)
	out.VenueSlug = firstNonEmptyValue(context.VenueSlug, out.VenueSlug)
	out.CanonicalURL = firstNonEmptyValue(context.CanonicalURL, out.CanonicalURL)
	out.Currency = firstNonEmptyValue(context.Currency, out.Currency)
	out.IncludeOptionGroupIDs = context.IncludeOptionGroupIDs
	identity := ExtractVenueIdentity(
		VenueIdentity{
			ID:           out.VenueID,
			Slug:         out.VenueSlug,
			CanonicalURL: out.CanonicalURL,
		},
		payloads...,
	)
	out.VenueID = identity.ID
	out.VenueSlug = identity.Slug
	out.CanonicalURL = identity.CanonicalURL
	if out.Currency == "" {
		out.Currency = resolvePayloadCurrency(payloads)
	}
	return out
}

func firstItemVenueContext(contexts []ItemVenueContext) ItemVenueContext {
	if len(contexts) == 0 {
		return ItemVenueContext{}
	}
	return contexts[0]
}

func buildItemRow(
	item map[string]any,
	context ItemVenueContext,
	basePrice map[string]any,
	originalPrice map[string]any,
) map[string]any {
	row := map[string]any{
		"item_id":        item["item_id"],
		"venue_id":       firstNonEmptyValue(stringFromAny(item["venue_id"]), context.VenueID),
		"venue_slug":     firstNonEmptyValue(stringFromAny(item["venue_slug"]), context.VenueSlug),
		"name":           item["name"],
		"description":    coalesce(item["description"], ""),
		"category":       item["category"],
		"price":          basePrice,
		"base_price":     basePrice,
		"original_price": originalPrice,
		"discounts":      item["discounts"],
		"currency":       basePrice["currency"],
		"is_available":   boolValue(item["is_available"]),
		"is_sold_out":    boolValue(item["is_sold_out"]),
	}
	copyOptionalItemField(row, item, "category_id")
	copyOptionalItemField(row, item, "category_name")
	copyItemLanguageVariants(row, item)
	if context.IncludeOptionGroupIDs {
		row["option_group_ids"] = item["option_group_ids"]
	}
	if context.CanonicalURL != "" {
		row["canonical_url"] = context.CanonicalURL
	}
	CopyItemMetadata(row, item)
	return row
}

func copyOptionalItemField(target map[string]any, source map[string]any, key string) {
	value, exists := source[key]
	if !exists || strings.TrimSpace(stringFromAny(value)) == "" {
		return
	}
	target[key] = value
}

// CurrentItemMetadataKeys lists the availability and product-metadata fields
// that every normalized item row carries. Row builders outside this package
// use it so the CLI and MCP surfaces cannot drift apart, or from the documented
// output contract.
var CurrentItemMetadataKeys = []string{
	"is_available",
	"unavailable_reason",
	"is_sold_out",
	"purchasable_balance",
	"image_url",
	"image_urls",
	"image_blurhash",
	"unit_info",
	"unit_price",
	"sell_by_weight_config",
}

// CopyItemMetadata copies every present CurrentItemMetadataKeys entry from
// source onto target.
func CopyItemMetadata(target map[string]any, source map[string]any) {
	for _, key := range CurrentItemMetadataKeys {
		if value, exists := source[key]; exists {
			target[key] = value
		}
	}
}
