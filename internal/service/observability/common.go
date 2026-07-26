package observability

import (
	"fmt"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(text string) string {
	normalized := slugPattern.ReplaceAllString(strings.ToLower(text), "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func formatAmount(amount *int, currency string) *string {
	if amount == nil || strings.TrimSpace(currency) == "" {
		return nil
	}
	v := fmt.Sprintf("%s %.2f", currency, float64(*amount)/100)
	return &v
}
