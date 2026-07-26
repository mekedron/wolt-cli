package domain

import "strings"

// IsObjectID reports whether value is a canonical 24-character hexadecimal
// identifier used by Wolt venue and item resources.
func IsObjectID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 24 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') &&
			(char < 'a' || char > 'f') &&
			(char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// NormalizeObjectID returns a canonical Wolt resource id or an empty string
// when the input is a slug, tracking target, or another non-id value.
func NormalizeObjectID(value any) string {
	normalized := strings.TrimSpace(NormalizeID(value))
	if !IsObjectID(normalized) {
		return ""
	}
	return normalized
}
