package observability

import (
	"fmt"
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
			rows = append(rows, map[string]any{
				"venue_id":          domain.NormalizeID(coalesce(item.Venue.ID, item.Link.Target)),
				"slug":              item.Venue.Slug,
				"name":              item.Title,
				"rating":            ratingValue,
				"delivery_estimate": item.Venue.FormatEstimateRange(),
				"delivery_fee":      deliveryFeeMap(item.Venue.DeliveryPriceInt, item.Venue.Currency),
				"price_range":       priceRangeValue,
				"price_range_scale": priceRangeScale(item.Venue.PriceRange),
				"promotions":        venuePromotionTexts(item.Venue),
				"wolt_plus":         isWoltPlus,
			})
		}
		if woltPlusOnly && len(rows) == 0 {
			continue
		}
		title := section.Title
		if title == "" {
			title = section.Name
		}
		sectionRows = append(sectionRows, map[string]any{
			"name":  section.Name,
			"title": title,
			"items": rows,
		})
	}

	resolvedCity := strings.TrimSpace(city)
	if resolvedCity == "" {
		resolvedCity = "unknown"
	}

	return map[string]any{"city": resolvedCity, "wolt_plus_only": woltPlusOnly, "sections": sectionRows}
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
			return filtered[i].Venue.Estimate < filtered[j].Venue.Estimate
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
			left := 0
			right := 0
			if filtered[i].Venue.DeliveryPriceInt != nil {
				left = *filtered[i].Venue.DeliveryPriceInt
			}
			if filtered[j].Venue.DeliveryPriceInt != nil {
				right = *filtered[j].Venue.DeliveryPriceInt
			}
			return left < right
		})
	case VenueSortDeliveryTime:
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].Venue.Estimate < filtered[j].Venue.Estimate
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
		rows = append(rows, map[string]any{
			"venue_id":          domain.NormalizeID(coalesce(item.Venue.ID, item.Link.Target)),
			"slug":              item.Venue.Slug,
			"name":              item.Title,
			"address":           item.Venue.Address,
			"rating":            ratingValue,
			"delivery_estimate": item.Venue.FormatEstimateRange(),
			"delivery_fee":      deliveryFeeMap(item.Venue.DeliveryPriceInt, item.Venue.Currency),
			"price_range":       priceRangeValue,
			"price_range_scale": priceRangeScale(item.Venue.PriceRange),
			"promotions":        venuePromotionTexts(item.Venue),
			"wolt_plus":         venueWoltPlus(item.Venue),
		})
	}

	return map[string]any{
		"query": query,
		"total": total,
		"items": rows,
	}, warnings
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

// BuildVenueDetail normalizes venue detail payload.
func BuildVenueDetail(item *domain.Item, restaurant *domain.Restaurant, include map[string]struct{}) (map[string]any, []string, error) {
	if item == nil || item.Venue == nil {
		return nil, nil, fmt.Errorf("item does not include venue details")
	}
	if restaurant == nil {
		return nil, nil, fmt.Errorf("restaurant cannot be nil")
	}

	warnings := []string{"order minimum is unavailable in basic mode and returned as null"}

	var ratingValue any
	if restaurant.Rating != nil {
		ratingValue = restaurant.Rating.Score
	} else if item.Venue.Rating != nil {
		ratingValue = item.Venue.Rating.Score
	}

	data := map[string]any{
		"venue_id":         domain.NormalizeID(coalesce(restaurant.ID, item.Venue.ID, item.Link.Target)),
		"slug":             stringValue(coalesce(restaurant.Slug, item.Venue.Slug)),
		"name":             item.Title,
		"address":          restaurant.Address,
		"currency":         restaurant.Currency,
		"rating":           ratingValue,
		"delivery_methods": restaurant.DeliveryMethods,
		"order_minimum": map[string]any{
			"amount":           nil,
			"formatted_amount": nil,
		},
	}

	if _, ok := include["hours"]; ok {
		data["opening_windows"] = openingWindows(restaurant)
	}
	if _, ok := include["tags"]; ok {
		data["tags"] = restaurant.FoodTags
	}
	if _, ok := include["rating"]; ok && restaurant.Rating != nil {
		data["rating_details"] = map[string]any{
			"score":  restaurant.Rating.Score,
			"text":   restaurant.Rating.Text,
			"volume": restaurant.Rating.Volume,
		}
	}
	if _, ok := include["fees"]; ok {
		data["delivery_fee"] = deliveryFeeMap(item.Venue.DeliveryPriceInt, item.Venue.Currency)
	}

	return data, warnings, nil
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

// BuildVenueHours renders venue opening windows.
func BuildVenueHours(restaurant *domain.Restaurant, timezone string) map[string]any {
	resolvedTimezone := strings.TrimSpace(timezone)
	if resolvedTimezone == "" {
		resolvedTimezone = strings.TrimSpace(restaurant.TimezoneName)
	}
	if resolvedTimezone == "" {
		resolvedTimezone = "UTC"
	}
	return map[string]any{
		"venue_id":         domain.NormalizeID(restaurant.ID),
		"timezone":         resolvedTimezone,
		"opening_windows":  openingWindows(restaurant),
		"delivery_windows": []any{},
	}
}
