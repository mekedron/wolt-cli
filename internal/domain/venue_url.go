package domain

import (
	"net/url"
	"strings"
)

// IsWoltURL reports whether raw is an absolute HTTP(S) URL on Wolt's public
// web host. Venue and item references must not reinterpret third-party URLs as
// Wolt slugs or item IDs.
func IsWoltURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "http") &&
		!strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	return host == "wolt.com" || host == "www.wolt.com"
}

// CanonicalVenueURL normalizes a Wolt venue or nested item/category URL to the
// venue root. expectedSlug, when non-empty, prevents a URL for another venue
// from replacing already-resolved identity.
func CanonicalVenueURL(raw string, expectedSlug string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !IsWoltURL(raw) {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	slug, index, ok := venueSlugFromPath(parts)
	if !ok {
		return ""
	}
	if strings.TrimSpace(expectedSlug) != "" &&
		!strings.EqualFold(slug, strings.TrimSpace(expectedSlug)) {
		return ""
	}
	parsed.Scheme = "https"
	parsed.User = nil
	parsed.Host = "wolt.com"
	parsed.Path = "/" + strings.Join(parts[:index+2], "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

// VenueSlugFromReference returns the venue identity from a slug, id, or URL.
// Nested Wolt item and category URLs resolve to the segment immediately after
// venue/restaurant rather than to their final resource segment.
func VenueSlugFromReference(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		if strings.Contains(value, "://") {
			return ""
		}
		return value
	}
	if !IsWoltURL(value) {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if slug, _, ok := venueSlugFromPath(parts); ok {
		return slug
	}
	for index := len(parts) - 1; index >= 0; index-- {
		if part := strings.TrimSpace(parts[index]); part != "" {
			return part
		}
	}
	return ""
}

func venueSlugFromPath(parts []string) (string, int, bool) {
	for index := range parts {
		if !strings.EqualFold(parts[index], "venue") &&
			!strings.EqualFold(parts[index], "restaurant") {
			continue
		}
		if index+1 >= len(parts) {
			return "", 0, false
		}
		slug := strings.TrimSpace(parts[index+1])
		if slug == "" {
			return "", 0, false
		}
		return slug, index, true
	}
	return "", 0, false
}
