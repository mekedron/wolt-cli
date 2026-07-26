package domain

import (
	"fmt"
	"strings"
)

// NormalizeID normalizes mixed payload id values.
func NormalizeID(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any:
		if oid, ok := t["$oid"].(string); ok {
			return oid
		}
		return fmt.Sprint(t)
	default:
		return fmt.Sprint(t)
	}
}

func capitalizeWords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		out = append(out, strings.ToUpper(v[:1])+strings.ToLower(v[1:]))
	}
	return out
}

// FormatBadges renders venue badges for legacy tables.
func (v Venue) FormatBadges() string {
	if len(v.Badges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(v.Badges))
	for _, badge := range v.Badges {
		if badge.Text != "" {
			parts = append(parts, badge.Text)
		}
	}
	return strings.Join(parts, ", ")
}

// FormatTags renders venue tags for legacy tables.
func (v Venue) FormatTags() string {
	return strings.Join(capitalizeWords(v.Tags), ", ")
}

// FormatEstimateRange renders delivery estimate.
func (v Venue) FormatEstimateRange() string {
	if strings.TrimSpace(v.EstimateRange) == "" {
		return "-"
	}
	return strings.ReplaceAll(v.EstimateRange, "-", " - ") + " min"
}

// FormatDeliveryPrice renders delivery cost.
func (v Venue) FormatDeliveryPrice() string {
	if v.DeliveryPriceInt == nil {
		return "-"
	}
	if v.Delivers != nil && !*v.Delivers {
		return "(No delivery)"
	}
	return fmt.Sprintf("%.2f %s", float64(*v.DeliveryPriceInt)/100, v.Currency)
}

// FormatRating renders venue rating.
func (v Venue) FormatRating() string {
	if v.Rating == nil {
		return "(No rating)"
	}
	return fmt.Sprintf("%.1f", v.Rating.Score)
}

// FormatPriceRange renders price range.
func (v Venue) FormatPriceRange() string {
	if v.PriceRange <= 0 {
		return "-"
	}
	return strings.Repeat("$", v.PriceRange)
}

// FormatTitle renders item title and badges.
func (i Item) FormatTitle() string {
	if i.Venue == nil || len(i.Venue.Badges) == 0 {
		return i.Title
	}
	return fmt.Sprintf("%s (%s)", i.Title, i.Venue.FormatBadges())
}
