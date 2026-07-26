package observability

import (
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// BuildVenueDetailFromPayload builds venue detail without relying on the
// retired restaurant-api /v3/venues endpoint. Static data supplies stable
// identity/hours; dynamic data supplies location-aware ordering state.
func BuildVenueDetailFromPayload(
	fallback VenueIdentity,
	staticPayload map[string]any,
	dynamicPayload map[string]any,
	searchCandidate map[string]any,
	location *domain.Location,
	include map[string]struct{},
) (map[string]any, []string, error) {
	staticVenue := venuePayload(staticPayload)
	dynamicVenue := venuePayload(dynamicPayload)
	if staticVenue == nil && dynamicVenue == nil && searchCandidate == nil {
		return nil, nil, fmt.Errorf("venue payload is unavailable")
	}

	identity := ExtractVenueIdentity(fallback, staticPayload, dynamicPayload, searchCandidate)
	if identity.ID == "" && identity.Slug == "" {
		return nil, nil, fmt.Errorf("venue payload does not include an id or slug")
	}

	staticVenueRaw := toMap(staticPayload["venue_raw"])
	dynamicVenueRaw := toMap(dynamicPayload["venue_raw"])
	currency := firstNonEmptyValue(
		stringFromAny(staticVenue["currency"]),
		stringFromAny(staticVenueRaw["currency"]),
		stringFromAny(dynamicVenue["currency"]),
		stringFromAny(dynamicVenueRaw["currency"]),
		stringFromAny(searchCandidate["currency"]),
	)
	orderMinimum := intPointerFromAny(coalesceValue(
		staticPayload["order_minimum"],
		staticVenue["order_minimum"],
		toMap(toMap(dynamicPayload["venue_raw"])["delivery_specs"])["order_minimum_no_surcharge"],
	))
	ratingPayload := toMap(staticVenue["rating"])
	rating := coalesceValue(
		ratingPayload["score_raw"],
		ratingPayload["score"],
		staticVenue["rating_score"],
		dynamicVenue["rating"],
		searchCandidate["rating"],
	)

	data := map[string]any{
		"venue_id": identity.ID,
		"slug":     identity.Slug,
		"name": firstNonEmptyValue(
			stringFromAny(staticVenue["name"]),
			stringFromAny(dynamicVenue["name"]),
			stringFromAny(searchCandidate["name"]),
		),
		"address": firstNonEmptyValue(
			stringFromAny(staticVenue["address"]),
			stringFromAny(staticVenue["street_address"]),
			stringFromAny(staticVenueRaw["address"]),
			stringFromAny(staticVenueRaw["street_address"]),
			stringFromAny(dynamicVenue["address"]),
			stringFromAny(dynamicVenue["street_address"]),
			stringFromAny(dynamicVenueRaw["address"]),
			stringFromAny(dynamicVenueRaw["street_address"]),
			stringFromAny(searchCandidate["address"]),
		),
		"currency": currency,
		"timezone": firstNonEmptyValue(
			stringFromAny(staticVenue["timezone"]),
			stringFromAny(dynamicVenue["timezone"]),
		),
		"rating": rating,
		"delivery_methods": firstNonEmptyNormalizedStrings(
			staticVenue["delivery_methods"],
			staticVenueRaw["delivery_methods"],
			dynamicVenue["delivery_methods"],
			dynamicVenueRaw["delivery_methods"],
			searchCandidate["delivery_methods"],
		),
		"order_minimum": moneyMap(orderMinimum, currency),
		"availability":  BuildVenueAvailability(staticPayload, dynamicPayload, searchCandidate, location),
	}
	if identity.CanonicalURL != "" {
		data["canonical_url"] = identity.CanonicalURL
	}

	if _, ok := include["hours"]; ok {
		data["opening_windows"] = openingWindowsFromVenuePayload(staticPayload)
	}
	if _, ok := include["tags"]; ok {
		data["tags"] = normalizedStrings(coalesceValue(
			toMap(staticPayload["venue_raw"])["food_tags"],
			staticVenue["tags"],
			staticPayload["tags"],
		))
	}
	if _, ok := include["rating"]; ok && rating != nil {
		data["rating_details"] = map[string]any{
			"score":  rating,
			"text":   coalesceValue(ratingPayload["text"], ratingPayload["label"]),
			"volume": coalesceValue(ratingPayload["volume"], ratingPayload["count"]),
		}
	}
	if _, ok := include["fees"]; ok {
		data["delivery_fee"] = dynamicDeliveryFee(dynamicPayload, currency)
		data["fee_details"] = buildFeeDetails(dynamicPayload, orderMinimum, currency)
	}
	if _, ok := include["promotions"]; ok {
		data["promotions"] = buildVenuePromotionDetails(dynamicPayload)
	}
	data["delivery_options"] = buildDeliveryOptions(dynamicPayload)

	warnings := []string{}
	if dynamicPayload == nil {
		warnings = append(warnings, "location-aware ordering availability is unavailable")
	}
	if orderMinimum == nil {
		warnings = append(warnings, "order minimum is unavailable and returned as null")
	}
	return data, warnings, nil
}

// BuildVenueHoursFromPayload derives hours and the venue timezone from the
// supported static venue payload.
func BuildVenueHoursFromPayload(
	fallback VenueIdentity,
	staticPayload map[string]any,
	timezone string,
) (map[string]any, []string, error) {
	venue := venuePayload(staticPayload)
	identity := ExtractVenueIdentity(fallback, staticPayload)
	if venue == nil && identity.ID == "" && identity.Slug == "" {
		return nil, nil, fmt.Errorf("venue hours payload is unavailable")
	}
	venueTimezone := strings.TrimSpace(stringFromAny(venue["timezone"]))
	windows := openingWindowsFromVenuePayload(staticPayload)
	resolvedTimezone, warnings := resolveVenueHoursTimezone(venueTimezone, timezone)
	if len(windows) == 0 {
		warnings = append(warnings, "opening hours are unavailable in the venue payload")
	}
	data := map[string]any{
		"venue_id":         identity.ID,
		"slug":             identity.Slug,
		"timezone":         resolvedTimezone,
		"opening_windows":  windows,
		"delivery_windows": []any{},
	}
	if identity.CanonicalURL != "" {
		data["canonical_url"] = identity.CanonicalURL
	}
	return data, warnings, nil
}

func resolveVenueHoursTimezone(venueTimezone string, requestedTimezone string) (any, []string) {
	venueTimezone = strings.TrimSpace(venueTimezone)
	requestedTimezone = strings.TrimSpace(requestedTimezone)
	if venueTimezone != "" {
		if requestedTimezone != "" && !strings.EqualFold(requestedTimezone, venueTimezone) {
			return venueTimezone, []string{
				fmt.Sprintf(
					"timezone override %q was not applied because upstream hours are venue-local and do not include dates needed for a safe conversion",
					requestedTimezone,
				),
			}
		}
		return venueTimezone, nil
	}
	if requestedTimezone != "" {
		return requestedTimezone, []string{
			fmt.Sprintf(
				"venue timezone is unavailable; caller-provided timezone %q was used without validating or converting venue-local hours",
				requestedTimezone,
			),
		}
	}
	return nil, []string{"venue timezone is unavailable; timezone remains unknown"}
}

// BuildVenueAvailability exposes the normalized location-aware order state for
// callers that already resolved the venue separately.
func BuildVenueAvailability(
	staticPayload map[string]any,
	dynamicPayload map[string]any,
	searchCandidate map[string]any,
	location *domain.Location,
) map[string]any {
	venue := venuePayload(dynamicPayload)
	deliveryOpen := toMap(venue["delivery_open_status"])
	openStatus := toMap(venue["open_status"])

	orderNow, orderNowKnown := boolFromAny(deliveryOpen["is_open"])
	if !orderNowKnown {
		orderNow, orderNowKnown = boolFromAny(venue["online"])
	}
	if !orderNowKnown {
		orderNow, orderNowKnown = boolFromAny(searchCandidate["order_now_available"])
	}

	scheduledOrder, scheduledKnown := scheduledDeliveryAvailable(venue)
	if !scheduledKnown {
		scheduledOrder, scheduledKnown = boolFromAny(searchCandidate["scheduled_order_available"])
	}

	deliversToLocation, deliveryKnown := false, false
	if location != nil {
		if scheduledKnown && scheduledOrder {
			// A location-aware scheduled home-delivery slot is stronger evidence of
			// service-area coverage than a generic "delivers" flag, which some Wolt
			// search payloads set to false while a venue is closed for order-now.
			deliversToLocation, deliveryKnown = true, true
		} else {
			deliversToLocation, deliveryKnown = boolFromAny(venue["delivers"])
			if !deliveryKnown {
				deliversToLocation, deliveryKnown = boolFromAny(searchCandidate["delivers_to_location"])
			}
			if !deliveryKnown {
				deliversToLocation, deliveryKnown = deliveryAvailableAtLocation(staticPayload, dynamicPayload, location)
			}
		}
	}
	out := map[string]any{
		"order_now_available":       nullableBool(orderNow, orderNowKnown),
		"scheduled_order_available": nullableBool(scheduledOrder, scheduledKnown),
		"scheduled_only":            nullableBool(scheduledOrder && !orderNow, scheduledKnown && orderNowKnown),
		"store_open_now":            nullableBoolValue(openStatus["is_open"]),
		"delivers_to_location":      nullableBool(deliversToLocation, deliveryKnown),
		"next_opening_at": emptyToNil(firstNonEmptyValue(
			stringFromAny(deliveryOpen["next_open"]),
			stringFromAny(openStatus["next_open"]),
			stringFromAny(searchCandidate["next_opening_at"]),
		)),
		"next_closing_at": emptyToNil(firstNonEmptyValue(
			stringFromAny(deliveryOpen["next_close"]),
			stringFromAny(openStatus["next_close"]),
		)),
		"status_text": emptyToNil(firstNonEmptyValue(
			stringFromAny(deliveryOpen["value"]),
			stringFromAny(openStatus["value"]),
			stringFromAny(searchCandidate["status_text"]),
		)),
		"telemetry_status": emptyToNil(stringFromAny(searchCandidate["telemetry_status"])),
	}
	if venue != nil {
		out["selected_delivery_method"] = stringFromAny(toMap(venue["header"])["delivery_method_default"])
	}
	return out
}

func scheduledDeliveryAvailable(venue map[string]any) (bool, bool) {
	if venue == nil {
		return false, false
	}
	known := false
	for _, raw := range toSlice(venue["delivery_configs"]) {
		config := toMap(raw)
		if !strings.EqualFold(stringFromAny(config["method"]), "homedelivery") ||
			!strings.EqualFold(stringFromAny(config["schedule"]), "time_slot") {
			continue
		}
		known = true
		if hasTimeSlots(config["tso_schedule"]) {
			return true, true
		}
	}
	for _, raw := range toSlice(toMap(venue["header"])["delivery_method_statuses"]) {
		status := toMap(raw)
		if !strings.EqualFold(stringFromAny(status["delivery_method"]), "DELIVERY_SCHEDULED") {
			continue
		}
		known = true
		if enabled, ok := boolFromAny(toMap(status["call_to_action"])["enabled"]); ok && enabled {
			return true, true
		}
	}
	return false, known
}

func hasTimeSlots(value any) bool {
	for _, rawDay := range toSlice(value) {
		if len(toSlice(toMap(rawDay)["time_slots"])) > 0 {
			return true
		}
	}
	return false
}

func buildDeliveryOptions(dynamicPayload map[string]any) []map[string]any {
	venue := venuePayload(dynamicPayload)
	out := []map[string]any{}
	for _, raw := range toSlice(venue["delivery_configs"]) {
		config := toMap(raw)
		row := map[string]any{
			"method":   stringFromAny(config["method"]),
			"schedule": stringFromAny(config["schedule"]),
			"label":    stringFromAny(config["label"]),
			"estimate": config["estimate"],
			"price":    config["price"],
		}
		if firstSlot := firstAvailableTimeSlot(config["tso_schedule"]); firstSlot != nil {
			row["first_available_time_slot"] = firstSlot
		}
		out = append(out, row)
	}
	return out
}

func firstAvailableTimeSlot(value any) map[string]any {
	for _, rawDay := range toSlice(value) {
		day := toMap(rawDay)
		slots := toSlice(day["time_slots"])
		if len(slots) == 0 {
			continue
		}
		slot := toMap(slots[0])
		return map[string]any{
			"day":             stringFromAny(day["day"]),
			"time_slot_start": slot["time_slot_start"],
			"time_slot_end":   slot["time_slot_end"],
			"formatted":       slot["time_slot_formatted"],
		}
	}
	return nil
}

func dynamicDeliveryFee(dynamicPayload map[string]any, currency string) map[string]any {
	for _, raw := range toSlice(venuePayload(dynamicPayload)["delivery_configs"]) {
		config := toMap(raw)
		if !strings.EqualFold(stringFromAny(config["method"]), "homedelivery") ||
			!strings.EqualFold(stringFromAny(config["schedule"]), "standard") {
			continue
		}
		price := toMap(toMap(config["price"])["price"])
		if amount := intPointerFromAny(price["amount"]); amount != nil {
			return moneyMap(amount, currency)
		}
	}
	specs := toMap(toMap(dynamicPayload["venue_raw"])["delivery_specs"])
	return moneyMap(intPointerFromAny(specs["original_delivery_price"]), currency)
}

func buildFeeDetails(dynamicPayload map[string]any, orderMinimum *int, currency string) map[string]any {
	specs := toMap(toMap(dynamicPayload["venue_raw"])["delivery_specs"])
	return map[string]any{
		"order_minimum":                   moneyMap(orderMinimum, currency),
		"order_minimum_possible":          moneyMap(intPointerFromAny(specs["order_minimum_possible"]), currency),
		"order_minimum_without_surcharge": moneyMap(intPointerFromAny(specs["order_minimum_no_surcharge"]), currency),
		"service_fee":                     nil,
		"minimum_order_surcharge":         nil,
		"service_fee_source":              "checkout_preview",
		"minimum_order_surcharge_source":  "checkout_preview",
	}
}

func buildVenuePromotionDetails(dynamicPayload map[string]any) []map[string]any {
	out := []map[string]any{}
	seenText := map[string]struct{}{}
	for _, raw := range toSlice(toMap(dynamicPayload["venue_raw"])["discounts"]) {
		discount := toMap(raw)
		conditions := toMap(discount["conditions"])
		effects := toMap(discount["effects"])
		title := firstNonEmptyValue(
			stringFromAny(toMap(discount["description"])["title"]),
			promotionLabelFromMap(toMap(discount["banner"])),
		)
		if normalized := strings.ToLower(strings.TrimSpace(title)); normalized != "" {
			seenText[normalized] = struct{}{}
		}
		out = append(out, map[string]any{
			"id":                   stringFromAny(discount["id"]),
			"text":                 title,
			"conditions":           conditions,
			"conditions_available": len(conditions) > 0,
			"effects":              effects,
		})
	}
	for _, label := range ExtractVenuePromotionLabels(dynamicPayload) {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized == "" {
			continue
		}
		if _, exists := seenText[normalized]; exists {
			continue
		}
		seenText[normalized] = struct{}{}
		out = append(out, map[string]any{
			"id":                   "",
			"text":                 strings.TrimSpace(label),
			"conditions":           nil,
			"conditions_available": false,
			"effects":              nil,
		})
	}
	return out
}

func openingWindowsFromVenuePayload(payload map[string]any) []map[string]string {
	openingTimes := toMap(toMap(payload["venue_raw"])["opening_times"])
	if openingTimes == nil {
		openingTimes = toMap(venuePayload(payload)["opening_times"])
	}
	if openingTimes == nil {
		return []map[string]string{}
	}
	weekdayOrder := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	out := make([]map[string]string, 0, len(weekdayOrder))
	for _, weekday := range weekdayOrder {
		openValue := ""
		for _, raw := range toSlice(openingTimes[weekday]) {
			entry := toMap(raw)
			seconds := intPointerFromAny(entry["value"])
			if seconds == nil || *seconds < 0 {
				continue
			}
			value := fmt.Sprintf("%02d:%02d", *seconds/3600, (*seconds%3600)/60)
			switch strings.ToLower(stringFromAny(entry["type"])) {
			case "open":
				openValue = value
			case "close":
				if openValue != "" {
					out = append(out, map[string]string{"day": weekday, "open": openValue, "close": value})
					openValue = ""
				}
			}
		}
		if openValue != "" {
			out = append(out, map[string]string{"day": weekday, "open": openValue, "close": "-"})
		}
	}
	return out
}

func venuePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	if venue := toMap(payload["venue"]); venue != nil {
		return venue
	}
	if venue := toMap(payload["venue_raw"]); venue != nil {
		return venue
	}
	if stringFromAny(payload["venue_id"]) != "" || stringFromAny(payload["slug"]) != "" {
		return payload
	}
	return nil
}

func normalizedStrings(value any) []string {
	out := []string{}
	for _, raw := range toSlice(value) {
		text := strings.TrimSpace(stringFromAny(raw))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstNonEmptyNormalizedStrings(values ...any) []string {
	for _, value := range values {
		if normalized := normalizedStrings(value); len(normalized) > 0 {
			return normalized
		}
	}
	return []string{}
}

func moneyMap(amount *int, currency string) map[string]any {
	var rawAmount any
	var formatted any
	if amount != nil {
		rawAmount = *amount
		if value := formatAmount(amount, currency); value != nil {
			formatted = *value
		}
	}
	return map[string]any{"amount": rawAmount, "currency": strings.TrimSpace(currency), "formatted_amount": formatted}
}

func intPointerFromAny(value any) *int {
	switch typed := value.(type) {
	case int:
		out := typed
		return &out
	case int64:
		out := int(typed)
		return &out
	case float64:
		out := int(typed)
		return &out
	case float32:
		out := int(typed)
		return &out
	default:
		return nil
	}
}

func boolFromAny(value any) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func nullableBool(value bool, known bool) any {
	if !known {
		return nil
	}
	return value
}

func nullableBoolValue(value any) any {
	resolved, ok := boolFromAny(value)
	return nullableBool(resolved, ok)
}

func coalesceValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
