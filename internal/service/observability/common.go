package observability

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
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

func openingWindows(restaurant *domain.Restaurant) []map[string]string {
	windows := make([]map[string]string, 0, 7)
	weekdayOrder := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	for _, weekday := range weekdayOrder {
		values := restaurant.OpeningTimes[weekday]
		openValue := "-"
		closeValue := "-"
		for _, value := range values {
			switch strings.ToLower(value.Type) {
			case "open":
				openValue = value.Format()
			case "close":
				closeValue = value.Format()
			}
		}
		windows = append(windows, map[string]string{"day": weekday, "open": openValue, "close": closeValue})
	}
	return windows
}
