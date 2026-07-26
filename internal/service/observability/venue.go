package observability

import (
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func limitSlice[T any](in []T, limit *int) []T {
	if limit == nil {
		return in
	}
	if *limit < 0 {
		return []T{}
	}
	if *limit >= len(in) {
		return in
	}
	return in[:*limit]
}

func deliveryFeeMap(amount *int, currency string) map[string]any {
	formatted := formatAmount(amount, currency)
	var formattedValue any
	if formatted != nil {
		formattedValue = *formatted
	}
	var amountValue any
	if amount != nil {
		amountValue = *amount
	}
	return map[string]any{
		"amount":           amountValue,
		"formatted_amount": formattedValue,
	}
}

// BuildDiscoveryFeed normalizes front-page sections.
func BuildDiscoveryFeed(sections []domain.Section, city string, limit *int, woltPlusOnly bool) map[string]any {
	resolvedSections := limitSlice(sections, limit)
	sectionRows := make([]map[string]any, 0, len(resolvedSections))

	for _, section := range resolvedSections {
		sectionItems := limitSlice(section.Items, limit)
		title := section.Title
		if title == "" {
			title = section.Name
		}

		// Sections where every item lacks a venue block (curated brand
		// carousels, restaurant-category tiles, hero banners) classify
		// as "brands" and emit a one-line summary. Mixed sections never
		// occur in the upstream payload today; if they do, the venue
		// items survive and the venueless ones are still dropped.
		if !sectionHasAnyVenue(sectionItems) {
			brands := buildBrandSummary(sectionItems)
			if woltPlusOnly || len(brands) == 0 {
				continue
			}
			sectionRows = append(sectionRows, map[string]any{
				"name":   section.Name,
				"title":  title,
				"kind":   "brands",
				"items":  []map[string]any{},
				"brands": brands,
			})
			continue
		}

		rows := make([]map[string]any, 0, len(sectionItems))
		for _, item := range sectionItems {
			if item.Venue == nil {
				continue
			}
			isWoltPlus := venueWoltPlus(item.Venue)
			if woltPlusOnly && !isWoltPlus {
				continue
			}
			var ratingValue any
			if item.Venue.Rating != nil {
				ratingValue = item.Venue.Rating.Score
			}
			var priceRangeValue any
			if item.Venue.PriceRange > 0 {
				priceRangeValue = item.Venue.PriceRange
			}
			row := map[string]any{
				"venue_id":          discoveryVenueID(item),
				"slug":              item.Venue.Slug,
				"name":              item.Title,
				"tagline":           item.Venue.Tagline(),
				"top_offer":         venueTopOffer(item.Venue),
				"rating":            ratingValue,
				"delivery_estimate": item.Venue.FormatEstimateRange(),
				"delivery_fee":      deliveryFeeMap(item.Venue.DeliveryPriceInt, item.Venue.Currency),
				"price_range":       priceRangeValue,
				"price_range_scale": priceRangeScale(item.Venue.PriceRange),
				"promotions":        venuePromotionTexts(item.Venue),
				"badges":            venueBadgeGlyphs(item.Venue),
				"menu_highlights":   venueMenuHighlights(item.Venue),
				"wolt_plus":         isWoltPlus,
			}
			addDiscoveryAvailability(row, item)
			rows = append(rows, row)
		}
		if woltPlusOnly && len(rows) == 0 {
			continue
		}
		sectionRows = append(sectionRows, map[string]any{
			"name":  section.Name,
			"title": title,
			"kind":  "venues",
			"items": rows,
		})
	}

	resolvedCity := strings.TrimSpace(city)
	if resolvedCity == "" {
		resolvedCity = "unknown"
	}

	return map[string]any{"city": resolvedCity, "wolt_plus_only": woltPlusOnly, "sections": sectionRows}
}

func sectionHasAnyVenue(items []domain.Item) bool {
	for _, item := range items {
		if item.Venue != nil {
			return true
		}
	}
	return false
}

// buildBrandSummary collapses non-venue items (brand carousels,
// category tiles, banner lists) into a stable { name, slug } shape.
// `slug` is the item's link target — it may be a curated list ID
// (e.g. "woltmarket-popular-brands:helsinki") rather than a venue
// slug, but it's a stable identifier the caller can act on.
func buildBrandSummary(items []domain.Item) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		name := strings.TrimSpace(item.Title)
		slug := strings.TrimSpace(item.Link.Target)
		if name == "" && slug == "" {
			continue
		}
		key := name + "|" + slug
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, map[string]any{
			"name": name,
			"slug": slug,
		})
	}
	return out
}

// BuildCategoryList extracts category slugs from section tags.
func BuildCategoryList(sections []domain.Section) map[string]any {
	categories := map[string]map[string]string{}
	for _, section := range sections {
		for _, item := range section.Items {
			if item.Venue == nil {
				continue
			}
			for _, tag := range item.Venue.Tags {
				slug := slugify(tag)
				categories[slug] = map[string]string{
					"id":   slug,
					"name": capitalize(tag),
					"slug": slug,
				}
			}
		}
	}

	rows := make([]map[string]string, 0, len(categories))
	for _, value := range categories {
		rows = append(rows, value)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["name"] < rows[j]["name"]
	})
	return map[string]any{"categories": rows}
}

func capitalize(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + strings.ToLower(value[1:])
}

func coalesce(values ...any) any {
	for _, value := range values {
		switch t := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(t) == "" {
				continue
			}
			return t
		default:
			return t
		}
	}
	return nil
}

// BuildVenueSearchResult applies filters/sorts over discovery items.
func BuildVenueSearchResult(
	items []domain.Item,
	query string,
	sortMode VenueSort,
	venueType *VenueType,
	category string,
	openNow bool,
	woltPlus bool,
	limit *int,
	offset int,
) (map[string]any, []string) {
	warnings := []string{}
	loweredQuery := strings.ToLower(strings.TrimSpace(query))
	loweredCategory := strings.ToLower(strings.TrimSpace(category))

	filtered := make([]domain.Item, 0, len(items))
	for _, item := range items {
		if item.Venue == nil {
			continue
		}
		if loweredQuery != "" {
			match := strings.Contains(strings.ToLower(item.Title), loweredQuery) ||
				strings.Contains(strings.ToLower(item.Venue.Address), loweredQuery)
			if !match {
				for _, tag := range item.Venue.Tags {
					if strings.Contains(strings.ToLower(tag), loweredQuery) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	if venueType != nil {
		out := make([]domain.Item, 0, len(filtered))
		for _, item := range filtered {
			productLine := item.Venue.ProductLine
			if strings.TrimSpace(productLine) == "" {
				productLine = "restaurant"
			}
			if productLine == string(*venueType) {
				out = append(out, item)
			}
		}
		filtered = out
	}

	if loweredCategory != "" {
		out := make([]domain.Item, 0, len(filtered))
		for _, item := range filtered {
			match := false
			for _, tag := range item.Venue.Tags {
				if strings.Contains(strings.ToLower(tag), loweredCategory) {
					match = true
					break
				}
			}
			if match {
				out = append(out, item)
			}
		}
		filtered = out
	}

	if openNow {
		out := make([]domain.Item, 0, len(filtered))
		for _, item := range filtered {
			if item.Venue.Online != nil && *item.Venue.Online {
				out = append(out, item)
			}
		}
		filtered = out
	}

	if woltPlus {
		out := make([]domain.Item, 0, len(filtered))
		for _, item := range filtered {
			if item.Venue.ShowWoltPlus {
				out = append(out, item)
			}
		}
		filtered = out
	}

	switch sortMode {
	case VenueSortDistance:
		warnings = append(warnings, "distance sort is approximated with delivery estimate in basic mode")
		sort.SliceStable(filtered, func(i, j int) bool {
			return optionalEstimateLess(filtered[i].Venue.Estimate, filtered[j].Venue.Estimate)
		})
	case VenueSortRating:
		sort.SliceStable(filtered, func(i, j int) bool {
			left := 0.0
			right := 0.0
			if filtered[i].Venue.Rating != nil {
				left = filtered[i].Venue.Rating.Score
			}
			if filtered[j].Venue.Rating != nil {
				right = filtered[j].Venue.Rating.Score
			}
			return left > right
		})
	case VenueSortDeliveryPrice:
		sort.SliceStable(filtered, func(i, j int) bool {
			return optionalIntLess(
				filtered[i].Venue.DeliveryPriceInt,
				filtered[j].Venue.DeliveryPriceInt,
			)
		})
	case VenueSortDeliveryTime:
		sort.SliceStable(filtered, func(i, j int) bool {
			return optionalEstimateLess(filtered[i].Venue.Estimate, filtered[j].Venue.Estimate)
		})
	}

	total := len(filtered)
	if offset > 0 {
		if offset >= len(filtered) {
			filtered = []domain.Item{}
		} else {
			filtered = filtered[offset:]
		}
	}
	filtered = limitSlice(filtered, limit)

	rows := make([]map[string]any, 0, len(filtered))
	for _, item := range filtered {
		var ratingValue any
		if item.Venue.Rating != nil {
			ratingValue = item.Venue.Rating.Score
		}
		var priceRangeValue any
		if item.Venue.PriceRange > 0 {
			priceRangeValue = item.Venue.PriceRange
		}
		row := map[string]any{
			"venue_id":          discoveryVenueID(item),
			"slug":              item.Venue.Slug,
			"name":              item.Title,
			"tagline":           item.Venue.Tagline(),
			"top_offer":         venueTopOffer(item.Venue),
			"address":           item.Venue.Address,
			"rating":            ratingValue,
			"delivery_estimate": item.Venue.FormatEstimateRange(),
			"delivery_fee":      deliveryFeeMap(item.Venue.DeliveryPriceInt, item.Venue.Currency),
			"price_range":       priceRangeValue,
			"price_range_scale": priceRangeScale(item.Venue.PriceRange),
			"promotions":        venuePromotionTexts(item.Venue),
			"badges":            venueBadgeGlyphs(item.Venue),
			"menu_highlights":   venueMenuHighlights(item.Venue),
			"wolt_plus":         venueWoltPlus(item.Venue),
		}
		addDiscoveryAvailability(row, item)
		rows = append(rows, row)
	}

	return map[string]any{
		"query": query,
		"total": total,
		"items": rows,
	}, warnings
}

func discoveryVenueID(item domain.Item) string {
	if item.Venue != nil {
		if venueID := strings.TrimSpace(domain.NormalizeID(item.Venue.ID)); venueID != "" {
			return venueID
		}
	}
	return domain.NormalizeObjectID(item.Link.Target)
}

// addDiscoveryAvailability preserves location-aware ordering signals that
// Wolt embeds in front-page venue rows. Discovery is still non-exhaustive, but
// callers can distinguish immediate and scheduled ordering for rows the feed
// does return. Unknown scheduled state remains null rather than false.
func addDiscoveryAvailability(row map[string]any, item domain.Item) {
	if row == nil || item.Venue == nil {
		return
	}

	var orderNow any
	if item.Venue.Online != nil {
		orderNow = *item.Venue.Online
	}
	telemetryStatus := firstNonEmptyValue(
		stringFromAny(item.Venue.Status["telemetry_status"]),
		stringFromAny(item.OverlayV2["telemetry_status"]),
	)
	scheduledOrder, scheduledPickup := scheduledAvailabilityFromTelemetryStatus(telemetryStatus)
	var scheduledOnly any
	if scheduled, known := scheduledOrder.(bool); known && item.Venue.Online != nil {
		scheduledOnly = scheduled && !*item.Venue.Online
	}

	row["order_now_available"] = orderNow
	// `online` means order-now availability, not necessarily that the
	// physical store is open. Exact venue detail uses the separate upstream
	// open_status signal for store_open_now.
	row["store_open_now"] = nil
	row["scheduled_order_available"] = scheduledOrder
	row["scheduled_pickup_available"] = scheduledPickup
	row["scheduled_only"] = scheduledOnly
	var deliversToLocation any
	if scheduled, known := scheduledOrder.(bool); known && scheduled {
		// Location-aware scheduled-order telemetry is more specific than the
		// generic discovery delivers flag, which can be false while a venue is
		// closed but still accepts scheduled home-delivery orders.
		deliversToLocation = true
	} else if item.Venue.Delivers != nil {
		deliversToLocation = *item.Venue.Delivers
	}
	row["delivers_to_location"] = deliversToLocation
	row["next_opening_at"] = emptyToNil(firstNonEmptyValue(
		stringFromAny(item.Venue.Status["next_open"]),
		stringFromAny(item.OverlayV2["next_open"]),
	))
	row["status_text"] = emptyToNil(firstNonEmptyValue(
		stringFromAny(item.Venue.Status["primary_text"]),
		stringFromAny(item.Venue.Status["value"]),
		stringFromAny(item.OverlayV2["primary_text"]),
	))
	row["telemetry_status"] = emptyToNil(telemetryStatus)
}

func scheduledAvailabilityFromTelemetryStatus(status string) (any, any) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch {
	case strings.HasPrefix(normalized, "scheduled_order"):
		return true, nil
	case strings.HasPrefix(normalized, "scheduled_pickup"):
		return nil, true
	default:
		return nil, nil
	}
}

func optionalEstimateLess(left float64, right float64) bool {
	leftKnown := left > 0
	rightKnown := right > 0
	if leftKnown != rightKnown {
		return leftKnown
	}
	return leftKnown && left < right
}

func optionalIntLess(left *int, right *int) bool {
	if (left != nil) != (right != nil) {
		return left != nil
	}
	return left != nil && *left < *right
}

func priceRangeScale(level int) string {
	if level <= 0 {
		return "-"
	}
	if level > 5 {
		level = 5
	}
	return strings.Repeat("$", level)
}

// venueTopOffer returns the most user-facing promotion text for a venue,
// preferring "discount"-variant promos over generic ones. Returns "" when
// the venue has no promo worth highlighting.
func venueTopOffer(venue *domain.Venue) string {
	if venue == nil {
		return ""
	}
	candidates := []map[string]any{}
	for _, raw := range venue.Promotions {
		if m, ok := raw.(map[string]any); ok {
			candidates = append(candidates, m)
		}
	}
	for _, raw := range venue.PromotionsForTelemetry {
		if m, ok := raw.(map[string]any); ok {
			candidates = append(candidates, m)
		}
	}
	pickDiscount := func() string {
		for _, m := range candidates {
			variant := strings.ToLower(strings.TrimSpace(stringValue(m["variant"])))
			if strings.Contains(variant, "discount") || strings.Contains(variant, "promotion") {
				if text := firstNonEmptyString(m, "text", "title", "name", "label", "description"); text != "" && !isWoltPlusText(text) {
					return text
				}
			}
		}
		return ""
	}
	if top := pickDiscount(); top != "" {
		return top
	}
	for _, m := range candidates {
		if text := firstNonEmptyString(m, "text", "title", "name", "label", "description"); text != "" && !isWoltPlusText(text) {
			return text
		}
	}
	for _, raw := range venue.Promotions {
		if s, ok := raw.(string); ok {
			text := strings.TrimSpace(s)
			if text != "" && !isWoltPlusText(text) {
				return text
			}
		}
	}
	for _, badge := range venue.Badges {
		text := strings.TrimSpace(badge.Text)
		variant := strings.ToLower(strings.TrimSpace(badge.Variant))
		if text == "" || isWoltPlusText(text) {
			continue
		}
		if strings.Contains(variant, "discount") || strings.Contains(variant, "promotion") {
			return text
		}
	}
	return ""
}

// venueMenuHighlights flattens venue_preview_items into a stable
// { name, formatted_price } shape. Upstream only populates this for
// sponsored / featured rows; returns an empty slice otherwise.
func venueMenuHighlights(venue *domain.Venue) []map[string]any {
	out := []map[string]any{}
	if venue == nil {
		return out
	}
	for _, raw := range venue.PreviewItems {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := firstNonEmptyString(entry, "name", "title", "label")
		price := firstNonEmptyString(entry, "formatted_price", "price_formatted", "formatted_amount", "price")
		if name == "" && price == "" {
			continue
		}
		out = append(out, map[string]any{
			"name":            name,
			"formatted_price": price,
		})
	}
	return out
}

// venueBadgeGlyphs maps the BadgesV2 array onto the stable
// { icon, variant, text } output shape. Reads from BadgesV2 only —
// the legacy Badges array omits icon and lives in the `promotions[]`
// derivation already.
func venueBadgeGlyphs(venue *domain.Venue) []map[string]any {
	out := []map[string]any{}
	if venue == nil {
		return out
	}
	for _, badge := range venue.BadgesV2 {
		text := strings.TrimSpace(badge.Text)
		icon := strings.TrimSpace(badge.Icon)
		variant := strings.TrimSpace(badge.Variant)
		if text == "" && icon == "" {
			continue
		}
		out = append(out, map[string]any{
			"icon":    icon,
			"variant": variant,
			"text":    text,
		})
	}
	return out
}

func venuePromotionTexts(venue *domain.Venue) []string {
	if venue == nil {
		return []string{}
	}
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

	for _, promotionRaw := range venue.Promotions {
		switch promotion := promotionRaw.(type) {
		case string:
			appendLabel(promotion)
		case map[string]any:
			appendLabel(firstNonEmptyString(promotion, "text", "title", "name", "label", "description"))
		case map[string]string:
			appendLabel(firstNonEmptyStringFromStringMap(promotion, "text", "title", "name", "label", "description"))
		}
	}

	for _, badge := range venue.Badges {
		variant := strings.ToLower(strings.TrimSpace(badge.Variant))
		if strings.Contains(variant, "discount") || strings.Contains(variant, "promotion") {
			appendLabel(badge.Text)
		}
	}

	return out
}

func venueWoltPlus(venue *domain.Venue) bool {
	if venue == nil {
		return false
	}
	if venue.ShowWoltPlus {
		return true
	}
	if isWoltPlusText(venue.Icon) {
		return true
	}
	for _, tag := range venue.Tags {
		if isWoltPlusText(tag) {
			return true
		}
	}
	for _, badge := range venue.Badges {
		if isWoltPlusText(badge.Text) || isWoltPlusText(badge.Variant) {
			return true
		}
	}
	for _, promotion := range venuePromotionTexts(venue) {
		if isWoltPlusText(promotion) {
			return true
		}
	}
	return false
}

func isWoltPlusText(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "wolt+", "wolt plus", "wolt-plus", "wolt_plus":
		return true
	default:
		return strings.Contains(normalized, "wolt+") || strings.Contains(normalized, "wolt plus") || strings.Contains(normalized, "wolt-plus") || strings.Contains(normalized, "wolt_plus")
	}
}

func firstNonEmptyString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			resolved := strings.TrimSpace(stringValue(value))
			if resolved != "" {
				return resolved
			}
		}
	}
	return ""
}

func firstNonEmptyStringFromStringMap(payload map[string]string, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			resolved := strings.TrimSpace(value)
			if resolved != "" {
				return resolved
			}
		}
	}
	return ""
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
