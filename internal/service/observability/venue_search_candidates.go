package observability

import (
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// ExtractVenueSearchCandidates normalizes the venue section returned by the
// exhaustive search endpoint. Unlike discovery/front-page feeds, this endpoint
// includes closed and scheduled-order venues.
func ExtractVenueSearchCandidates(payload map[string]any) []map[string]any {
	out := []map[string]any{}
	seen := map[string]struct{}{}
	for _, rawSection := range toSlice(payload["sections"]) {
		section := toMap(rawSection)
		for _, rawItem := range toSlice(section["items"]) {
			item := toMap(rawItem)
			venue := toMap(item["venue"])
			if venue == nil {
				continue
			}
			id := strings.TrimSpace(stringFromAny(venue["id"]))
			slug := strings.TrimSpace(stringFromAny(venue["slug"]))
			if id == "" && slug == "" {
				continue
			}
			key := id
			if key == "" {
				key = slug
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			status := toMap(venue["status"])
			overlay := toMap(item["overlay_v2"])
			telemetryStatus := firstNonEmptyValue(
				stringFromAny(status["telemetry_status"]),
				stringFromAny(overlay["telemetry_status"]),
			)
			var orderNow any
			if value, known := boolFromAny(venue["online"]); known {
				orderNow = value
			}
			scheduledOrder, scheduledPickup := scheduledAvailabilityFromTelemetryStatus(telemetryStatus)
			canonicalURL := domain.CanonicalVenueURL(
				stringFromAny(toMap(item["link"])["target"]),
				slug,
			)
			if canonicalURL == "" {
				canonicalURL = findVenueURL(item, slug)
			}
			var deliversToLocation any
			if value, known := boolFromAny(venue["delivers"]); known {
				deliversToLocation = value
			}

			out = append(out, map[string]any{
				"venue_id":                   id,
				"venue_slug":                 slug,
				"slug":                       slug,
				"canonical_url":              canonicalURL,
				"name":                       firstNonEmptyValue(stringFromAny(venue["name"]), stringFromAny(item["title"])),
				"address":                    stringFromAny(venue["address"]),
				"currency":                   stringFromAny(venue["currency"]),
				"order_now_available":        orderNow,
				"scheduled_order_available":  scheduledOrder,
				"scheduled_pickup_available": scheduledPickup,
				"delivers_to_location":       deliversToLocation,
				"next_opening_at": emptyToNil(firstNonEmptyValue(
					stringFromAny(status["next_open"]),
					stringFromAny(overlay["next_open"]),
				)),
				"telemetry_status": telemetryStatus,
				"status_text": firstNonEmptyValue(
					stringFromAny(status["primary_text"]),
					stringFromAny(overlay["primary_text"]),
					stringFromAny(toMap(venue["estimate_box"])["subtitle"]),
				),
			})
		}
	}
	return out
}
