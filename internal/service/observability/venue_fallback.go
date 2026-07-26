package observability

import (
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// The rich `restaurant-api/v3/venues/<id>` document that BuildVenueDetail and
// BuildVenueHours consume is retired upstream and answers HTTP 410 for every
// client. The builders in this file reconstruct the same output shape from the
// static venue page (`order-xp/web/v1/pages/venue/slug/<slug-or-id>/static`),
// which is always reachable. Callers pair them with the rich builders: rich
// document when it arrives, these when it does not. Each returns warnings
// naming the degraded fields so consumers can tell partial data from complete.

// weekdayOrder fixes the order of opening windows so table and JSON output stay
// stable across runs regardless of upstream map iteration.
var weekdayOrder = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

// BuildVenueDetailFallback mirrors BuildVenueDetail's output from the static
// venue page. item may be nil — it only enriches fields the static payload does
// not carry (delivery fee, and a title when the page omits the name).
func BuildVenueDetailFallback(
	slug string,
	venueID string,
	item *domain.Item,
	staticPayload map[string]any,
	include map[string]struct{},
) (map[string]any, []string) {
	venuePayload := payloadutil.Map(staticPayload["venue"])
	if venuePayload == nil {
		venuePayload = payloadutil.Map(staticPayload["venue_raw"])
	}

	name := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		fallbackItemTitle(item),
		venuePayload["name"],
		staticPayload["name"],
		SluggifiedTitle(slug),
	)))
	address := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		venuePayload["address"],
		venuePayload["street_address"],
	)))
	currency := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		venuePayload["currency"],
		fallbackItemCurrency(item),
		staticPayload["currency"],
	)))
	// score_raw is numeric, matching the rich path's restaurant.Rating.Score so
	// table and JSON output stay identical across both sources.
	ratingPayload := payloadutil.Map(venuePayload["rating"])
	rating := payloadutil.CoalesceAny(ratingPayload["score_raw"], ratingPayload["score"], fallbackItemRating(item))

	deliveryMethods := payloadutil.Slice(venuePayload["delivery_methods"])
	if deliveryMethods == nil {
		deliveryMethods = []any{}
	}

	orderMinimum := map[string]any{
		"amount":           nil,
		"formatted_amount": nil,
	}
	orderMinimumResolved := false
	if raw := payloadutil.CoalesceAny(staticPayload["order_minimum"], venuePayload["order_minimum"]); raw != nil {
		amount := payloadutil.Int(raw)
		orderMinimum["amount"] = amount
		if formatted := payloadutil.FormatMinorAmount(amount, currency); formatted != "" {
			orderMinimum["formatted_amount"] = formatted
		}
		orderMinimumResolved = true
	}

	data := map[string]any{
		"venue_id":         venueID,
		"slug":             strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(venuePayload["slug"], slug))),
		"name":             name,
		"address":          address,
		"currency":         currency,
		"rating":           rating,
		"delivery_methods": deliveryMethods,
		"order_minimum":    orderMinimum,
	}

	warnings := []string{
		"restaurant detail endpoint unavailable; showing venue details from static payload",
	}

	// "hours" is served by the dedicated hours builder, which reads the same
	// static payload; venue detail advertises the key without duplicating it.
	if _, ok := include["hours"]; ok {
		data["opening_windows"] = []any{}
	}
	if _, ok := include["tags"]; ok {
		data["tags"] = fallbackTags(venuePayload, staticPayload)
	}
	if _, ok := include["rating"]; ok && rating != nil {
		data["rating_details"] = map[string]any{
			"score":  rating,
			"text":   nil,
			"volume": nil,
		}
	}
	if _, ok := include["fees"]; ok {
		amount := fallbackItemDeliveryFee(item)
		formatted := any(nil)
		if amount != nil {
			if text := payloadutil.FormatMinorAmount(*amount, currency); text != "" {
				formatted = text
			}
		}
		data["delivery_fee"] = map[string]any{
			"amount":           fallbackAmountValue(amount),
			"formatted_amount": formatted,
		}
	}

	if !orderMinimumResolved {
		warnings = append(warnings, "order minimum is unavailable in basic mode and returned as null")
	}
	return data, warnings
}

// BuildVenueHoursFallback mirrors BuildVenueHours' output from the static venue
// page. The static page carries no timezone of its own for this shape, so the
// caller's timezone wins and UTC is the last resort.
func BuildVenueHoursFallback(venueID string, timezone string, staticPayload map[string]any) (map[string]any, []string) {
	resolvedTimezone := strings.TrimSpace(timezone)
	if resolvedTimezone == "" {
		venuePayload := payloadutil.Map(staticPayload["venue"])
		resolvedTimezone = strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
			venuePayload["timezone"],
			venuePayload["timezone_name"],
		)))
	}
	if resolvedTimezone == "" {
		resolvedTimezone = "UTC"
	}
	windows := OpeningWindowsFromStaticPayload(staticPayload)
	data := map[string]any{
		"venue_id":         venueID,
		"timezone":         resolvedTimezone,
		"opening_windows":  windows,
		"delivery_windows": []any{},
	}
	warnings := []string{}
	if len(windows) == 0 {
		warnings = append(warnings, "restaurant detail endpoint unavailable; opening hours are unavailable in fallback mode")
	} else {
		warnings = append(warnings, "restaurant detail endpoint unavailable; opening hours derived from static venue payload")
	}
	return data, warnings
}

// OpeningWindowsFromStaticPayload lifts opening hours out of the static venue
// payload's opening_times map. Each weekday entry is a list of
// { type: "open"|"close", value: int } where value is seconds since midnight.
// Returns an empty slice when no weekday carries a usable window, so callers can
// distinguish "closed all week" from "hours unknown".
func OpeningWindowsFromStaticPayload(payload map[string]any) []any {
	openingTimes := payloadutil.Map(payloadutil.Map(payload["venue_raw"])["opening_times"])
	if openingTimes == nil {
		openingTimes = payloadutil.Map(payloadutil.Map(payload["venue"])["opening_times"])
	}
	if openingTimes == nil {
		return []any{}
	}

	windows := make([]any, 0, len(weekdayOrder))
	hasAny := false
	for _, weekday := range weekdayOrder {
		entries := payloadutil.Slice(openingTimes[weekday])
		openValue := "-"
		closeValue := "-"
		for _, raw := range entries {
			entry := payloadutil.Map(raw)
			if entry == nil {
				continue
			}
			if entry["value"] == nil {
				continue
			}
			secs := payloadutil.Int(entry["value"])
			hhmm := fmt.Sprintf("%02d:%02d", secs/3600, (secs%3600)/60)
			switch strings.ToLower(payloadutil.String(entry["type"])) {
			case "open":
				openValue = hhmm
			case "close":
				closeValue = hhmm
			}
		}
		if openValue != "-" || closeValue != "-" {
			hasAny = true
		}
		windows = append(windows, map[string]any{
			"day":   weekday,
			"open":  openValue,
			"close": closeValue,
		})
	}
	if !hasAny {
		return []any{}
	}
	return windows
}

// SluggifiedTitle turns a venue slug into a human-readable name, for the case
// where no payload carries one.
func SluggifiedTitle(slug string) string {
	parts := strings.Split(strings.TrimSpace(slug), "-")
	resolved := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		resolved = append(resolved, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
	}
	if len(resolved) == 0 {
		return strings.TrimSpace(slug)
	}
	return strings.Join(resolved, " ")
}

func fallbackTags(venuePayload map[string]any, staticPayload map[string]any) []any {
	tags := payloadutil.Slice(venuePayload["tags"])
	if len(tags) == 0 {
		tags = payloadutil.Slice(staticPayload["tags"])
	}
	resolved := make([]any, 0, len(tags))
	for _, value := range tags {
		tag := strings.TrimSpace(payloadutil.String(value))
		if tag == "" {
			continue
		}
		resolved = append(resolved, tag)
	}
	return resolved
}

func fallbackItemTitle(item *domain.Item) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.Title)
}

func fallbackItemCurrency(item *domain.Item) string {
	if item == nil || item.Venue == nil {
		return ""
	}
	return strings.TrimSpace(item.Venue.Currency)
}

func fallbackItemDeliveryFee(item *domain.Item) *int {
	if item == nil || item.Venue == nil {
		return nil
	}
	return item.Venue.DeliveryPriceInt
}

func fallbackItemRating(item *domain.Item) any {
	if item == nil || item.Venue == nil || item.Venue.Rating == nil {
		return nil
	}
	return item.Venue.Rating.Score
}

func fallbackAmountValue(amount *int) any {
	if amount == nil {
		return nil
	}
	return *amount
}
