package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

var objectIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

func looksLikeObjectID(value string) bool {
	return objectIDPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

// venueRef captures the parts of a venue identity that downstream API calls
// might need. After resolution, ID is always populated when possible.
type venueRef struct {
	Input string
	ID    string
	Slug  string
}

// normalizeVenueInput strips a full Wolt URL down to its last path segment, so
// callers can pass either slug, raw ID, or a copy-pasted browser URL.
func normalizeVenueInput(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return value
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		return part
	}
	return value
}

// resolveVenueRef turns whatever the LLM passed (slug / object ID / URL) into a
// {ID, Slug} pair. Both directions go through the venue page, whose `slug` path
// segment accepts either identifier.
func (tc *ToolCtx) resolveVenueRef(ctx context.Context, raw string) (venueRef, error) {
	input := normalizeVenueInput(raw)
	if input == "" {
		return venueRef{}, fmt.Errorf("venue identifier is required (slug, id, or wolt.com URL)")
	}
	ref := venueRef{Input: raw}
	if looksLikeObjectID(input) {
		ref.ID = input
		ref.Slug = tc.resolveVenueSlugFromID(ctx, input)
		return ref, nil
	}
	ref.Slug = input
	ref.ID = input
	if tc.wolt == nil {
		return ref, nil
	}
	if id := tc.resolveVenueIDFromSlug(ctx, input); id != "" {
		ref.ID = id
	}
	return ref, nil
}

// resolveVenueIDFromSlug turns a venue slug into its Mongo ObjectID. It tries
// the static venue page first, then falls back to the dynamic venue page. Wolt
// now 404s the static `pages/venue/slug/<slug>/static` endpoint for the vast
// majority of venues, so the dynamic page is the reliable slug→id source.
// Without it, cart mutations would post the slug as `venue_id` and the Wolt
// backend silently creates a non-persisting "phantom" basket (issue #19).
// Returns "" when neither endpoint yields a real id.
func (tc *ToolCtx) resolveVenueIDFromSlug(ctx context.Context, slug string) string {
	if tc.wolt == nil {
		return ""
	}
	if payload, err := tc.wolt.VenuePageStatic(ctx, slug); err == nil {
		if id := strings.TrimSpace(venueIDFromPayload(payload)); id != "" {
			return id
		}
	}
	payload, err := tc.wolt.VenuePageDynamic(ctx, slug, woltgateway.VenuePageDynamicOptions{})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(venueIDFromPayload(payload))
}

// resolveVenueSlugFromID turns a venue ObjectID into its slug. The static venue
// page serves either identifier from its `slug` path segment, so it doubles as
// the id→slug lookup; `restaurant-api/v3/venues/<id>` is retired upstream and
// answers HTTP 410 for every client, so it is not a source. A slug is mandatory
// for the assortment reads that gate cart adds and checkout preview on current
// item availability — those endpoints are slug-keyed only. Returns "" when the
// page is unavailable or carries no slug.
func (tc *ToolCtx) resolveVenueSlugFromID(ctx context.Context, venueID string) string {
	if tc.wolt == nil {
		return ""
	}
	payload, err := tc.wolt.VenuePageStatic(ctx, venueID)
	if err != nil {
		return ""
	}
	return venueSlugFromPayload(payload)
}

func venueIDFromPayload(payload map[string]any) string {
	venue := asMap(payload["venue"])
	if venue == nil {
		venue = asMap(payload["venue_raw"])
	}
	return strings.TrimSpace(asString(coalesceAny(
		venue["id"],
		payload["venue_id"],
		payload["id"],
	)))
}

func venueSlugFromPayload(payload map[string]any) string {
	venue := asMap(payload["venue"])
	if venue == nil {
		venue = asMap(payload["venue_raw"])
	}
	return strings.TrimSpace(asString(coalesceAny(
		venue["slug"],
		venue["venue_slug"],
		venue["public_slug"],
		venue["url_slug"],
		payload["venue_slug"],
		payload["slug"],
	)))
}
