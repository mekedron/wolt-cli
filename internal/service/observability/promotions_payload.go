package observability

import "strings"

type campaignItemDiscount struct {
	labels      []string
	maxFraction float64
}

// ExtractVenueWoltPlus reports whether payload marks venue as Wolt+.
func ExtractVenueWoltPlus(payload map[string]any) bool {
	candidates := []any{
		payload["show_wolt_plus"],
		payload["wolt_plus"],
		payload["is_wolt_plus"],
	}

	venue := toMap(payload["venue"])
	if venue != nil {
		candidates = append(candidates,
			venue["show_wolt_plus"],
			venue["wolt_plus"],
			venue["is_wolt_plus"],
		)
	}

	venueRaw := toMap(payload["venue_raw"])
	if venueRaw != nil {
		candidates = append(candidates,
			venueRaw["show_wolt_plus"],
			venueRaw["wolt_plus"],
			venueRaw["is_wolt_plus"],
		)
	}

	for _, candidate := range candidates {
		if boolValue(candidate) {
			return true
		}
	}

	return false
}

// ExtractVenuePromotionLabels returns unique promotion labels from dynamic/static venue payloads.
func ExtractVenuePromotionLabels(payload map[string]any) []string {
	out := []string{}
	seen := map[string]struct{}{}
	appendLabel := func(raw string) {
		label := strings.TrimSpace(raw)
		if label == "" {
			return
		}
		if _, exists := seen[label]; exists {
			return
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}

	appendDiscountLabels(payload["promotions"], appendLabel)
	appendDiscountLabels(payload["discounts"], appendLabel)
	appendDiscountLabels(payload["offers"], appendLabel)

	venue := toMap(payload["venue"])
	if venue != nil {
		appendDiscountLabels(venue["promotions"], appendLabel)
		appendDiscountLabels(venue["discounts"], appendLabel)
		appendDiscountLabels(venue["offers"], appendLabel)
		for _, rawBanner := range toSlice(venue["banners"]) {
			banner := toMap(rawBanner)
			if banner == nil {
				continue
			}
			appendLabel(promotionLabelFromMap(banner))
			appendLabel(promotionLabelFromMap(toMap(banner["discount"])))
		}
		offerAssistant := toMap(venue["offer_assistant"])
		for _, rawTracker := range toSlice(offerAssistant["offer_trackers"]) {
			tracker := toMap(rawTracker)
			if tracker == nil {
				continue
			}
			appendLabel(strings.TrimSpace(stringFromAny(tracker["title"])))
		}
	}

	venueRaw := toMap(payload["venue_raw"])
	if venueRaw != nil {
		for _, rawDiscount := range toSlice(venueRaw["discounts"]) {
			discount := toMap(rawDiscount)
			if discount == nil {
				continue
			}
			appendLabel(promotionLabelFromMap(discount))
			appendLabel(strings.TrimSpace(stringFromAny(toMap(discount["banner"])["formatted_text"])))
			appendLabel(strings.TrimSpace(stringFromAny(toMap(discount["description"])["title"])))
		}
	}

	return out
}

func campaignItemDiscountsByItemID(payload map[string]any) map[string]campaignItemDiscount {
	out := map[string]campaignItemDiscount{}
	venueRaw := toMap(payload["venue_raw"])
	if venueRaw == nil {
		return out
	}

	for _, rawDiscount := range toSlice(venueRaw["discounts"]) {
		discount := toMap(rawDiscount)
		if discount == nil {
			continue
		}
		effects := toMap(discount["effects"])
		itemDiscount := toMap(effects["item_discount"])
		if itemDiscount == nil {
			continue
		}
		include := toMap(itemDiscount["include"])
		itemIDs := toSlice(include["items"])
		if len(itemIDs) == 0 {
			continue
		}

		labels := discountLabelsFromCampaign(discount)
		fraction := normalizedDiscountFraction(itemDiscount["fraction"])
		for _, rawItemID := range itemIDs {
			itemID := strings.TrimSpace(stringFromAny(rawItemID))
			if itemID == "" {
				continue
			}
			entry := out[itemID]
			entry.labels = mergeStringLabels(entry.labels, labels)
			if fraction > entry.maxFraction {
				entry.maxFraction = fraction
			}
			out[itemID] = entry
		}
	}

	return out
}

func collectCampaignItemDiscounts(payloads []map[string]any) map[string]campaignItemDiscount {
	out := map[string]campaignItemDiscount{}
	for _, payload := range payloads {
		for itemID, discount := range campaignItemDiscountsByItemID(payload) {
			entry := out[itemID]
			entry.labels = mergeStringLabels(entry.labels, discount.labels)
			if discount.maxFraction > entry.maxFraction {
				entry.maxFraction = discount.maxFraction
			}
			out[itemID] = entry
		}
	}
	return out
}

func discountLabelsFromCampaign(discount map[string]any) []string {
	labels := []string{}
	labels = mergeStringLabels(labels, []string{
		promotionLabelFromMap(toMap(discount["effect_item_badge"])),
		promotionLabelFromMap(toMap(discount["condition_item_badge"])),
		promotionLabelFromMap(toMap(discount["banner"])),
		strings.TrimSpace(stringFromAny(toMap(discount["description"])["title"])),
	})
	return labels
}

func promotionLabelFromMap(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"formatted_text", "text", "title", "name", "label"} {
		value := strings.TrimSpace(stringFromAny(payload[key]))
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizedDiscountFraction(value any) float64 {
	raw := floatFromAny(value)
	if raw <= 0 {
		return 0
	}
	if raw > 1 && raw <= 100 {
		return raw / 100
	}
	if raw > 1 {
		return 1
	}
	return raw
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func mergeStringLabels(base []string, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	seen := map[string]struct{}{}
	appendLabel := func(raw string) {
		label := strings.TrimSpace(raw)
		if label == "" {
			return
		}
		if _, exists := seen[label]; exists {
			return
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	for _, label := range base {
		appendLabel(label)
	}
	for _, label := range extra {
		appendLabel(label)
	}
	return out
}

func labelsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return mergeStringLabels(nil, typed)
	case []any:
		labels := make([]string, 0, len(typed))
		for _, raw := range typed {
			labels = append(labels, strings.TrimSpace(stringFromAny(raw)))
		}
		return mergeStringLabels(nil, labels)
	default:
		return nil
	}
}
