package observability

import (
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// VenueIdentity is the canonical identity that can be recovered from Wolt
// venue/search payloads. CanonicalURL is populated only when Wolt returned it;
// callers must not infer branch identity from a display slug alone.
type VenueIdentity struct {
	ID           string
	Slug         string
	CanonicalURL string
}

// ExtractVenueIdentity resolves the identity fields Wolt repeats across its
// static venue page, dynamic venue page, and search result payloads.
func ExtractVenueIdentity(fallback VenueIdentity, payloads ...map[string]any) VenueIdentity {
	out := VenueIdentity{}
	for _, payload := range payloads {
		if payload == nil {
			continue
		}
		candidate := explicitVenueIdentity(payload)
		if out.ID == "" {
			out.ID = candidate.ID
		}
		if out.Slug == "" {
			out.Slug = candidate.Slug
		}
		if out.CanonicalURL == "" {
			out.CanonicalURL = explicitVenueURL(
				payload,
				firstNonEmptyValue(candidate.Slug, out.Slug, fallback.Slug),
			)
		}
	}
	if out.ID == "" {
		out.ID = strings.TrimSpace(fallback.ID)
	}
	if out.Slug == "" {
		out.Slug = strings.TrimSpace(fallback.Slug)
	}
	if out.CanonicalURL == "" {
		out.CanonicalURL = strings.TrimSpace(fallback.CanonicalURL)
	}
	return out
}

func explicitVenueURL(payload map[string]any, slug string) string {
	venue := toMap(payload["venue"])
	venueRaw := toMap(payload["venue_raw"])
	for _, raw := range []any{
		payload["canonical_url"],
		payload["public_url"],
		payload["share_url"],
		venue["canonical_url"],
		venue["public_url"],
		venue["share_url"],
		venueRaw["canonical_url"],
		venueRaw["public_url"],
		venueRaw["share_url"],
	} {
		if canonical := domain.CanonicalVenueURL(stringFromAny(raw), slug); canonical != "" {
			return canonical
		}
	}
	return findVenueURL(payload, slug)
}

// explicitVenueIdentity reads only fields whose placement identifies them as
// venue data. Bare root id/slug fields are accepted only for venue-shaped
// payloads; category and item endpoints use the same generic field names.
func explicitVenueIdentity(payload map[string]any) VenueIdentity {
	if payload == nil {
		return VenueIdentity{}
	}
	venue := toMap(payload["venue"])
	venueRaw := toMap(payload["venue_raw"])
	out := VenueIdentity{
		ID: firstNonEmptyValue(
			stringFromAny(venue["id"]),
			stringFromAny(venue["venue_id"]),
			stringFromAny(venueRaw["id"]),
			stringFromAny(venueRaw["venue_id"]),
			stringFromAny(payload["venue_id"]),
		),
		Slug: firstNonEmptyValue(
			stringFromAny(venue["slug"]),
			stringFromAny(venue["venue_slug"]),
			stringFromAny(venueRaw["slug"]),
			stringFromAny(venueRaw["venue_slug"]),
			stringFromAny(payload["venue_slug"]),
		),
	}
	if rootPayloadIsVenue(payload) {
		out.ID = firstNonEmptyValue(out.ID, stringFromAny(payload["id"]))
		out.Slug = firstNonEmptyValue(out.Slug, stringFromAny(payload["slug"]))
	}
	return out
}

func rootPayloadIsVenue(payload map[string]any) bool {
	for _, key := range []string{
		"address",
		"street_address",
		"timezone",
		"delivery_methods",
		"opening_times",
		"delivery_open_status",
		"open_status",
		"delivery_configs",
		"delivery_geo_range",
	} {
		if _, exists := payload[key]; exists {
			return true
		}
	}
	return false
}

func findVenueURL(value any, slug string) string {
	switch typed := value.(type) {
	case map[string]any:
		preferredKeys := []string{
			"canonical_url",
			"public_url",
			"share_url",
			"venue",
			"venue_raw",
			"link",
			"target",
			"url",
		}
		visited := make(map[string]struct{}, len(preferredKeys))
		for _, key := range preferredKeys {
			child, exists := typed[key]
			if !exists {
				continue
			}
			visited[key] = struct{}{}
			if found := findVenueURL(child, slug); found != "" {
				return found
			}
		}
		remainingKeys := make([]string, 0, len(typed)-len(visited))
		for key := range typed {
			if _, exists := visited[key]; !exists {
				remainingKeys = append(remainingKeys, key)
			}
		}
		sort.Strings(remainingKeys)
		for _, key := range remainingKeys {
			if found := findVenueURL(typed[key], slug); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findVenueURL(child, slug); found != "" {
				return found
			}
		}
	case string:
		if canonical := domain.CanonicalVenueURL(typed, slug); canonical != "" {
			return canonical
		}
	}
	return ""
}
