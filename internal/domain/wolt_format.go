package domain

import (
	"fmt"
	"strings"
	"time"
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
	if !v.Delivers {
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

// Format returns a HH:MM rendering for opening time values.
func (t Times) Format() string {
	ms, ok := t.Value["$date"]
	if !ok {
		return "-"
	}
	tm := time.UnixMilli(ms).UTC()
	return tm.Format("15:04")
}

// FormatDescription renders short restaurant description.
func (r Restaurant) FormatDescription() string {
	if len(r.ShortDescription) == 0 {
		return "-"
	}
	desc := r.ShortDescription[0].Value
	if len(desc) > 60 {
		return desc[:60] + "..."
	}
	return desc
}

// FormatOpeningTime renders today's opening window.
func (r Restaurant) FormatOpeningTime() string {
	if len(r.OpeningTimes) == 0 {
		return "-"
	}
	weekday := strings.ToLower(time.Now().Weekday().String())
	values := r.OpeningTimes[weekday]
	if len(values) == 0 {
		return "-"
	}
	open := "-"
	close := "-"
	for _, value := range values {
		switch strings.ToLower(value.Type) {
		case "open":
			open = value.Format()
		case "close":
			close = value.Format()
		}
	}
	if open == "-" && close == "-" {
		return "-"
	}
	return fmt.Sprintf("%s - %s", open, close)
}

// FormatDeliveryTime renders delivery estimate.
func (r Restaurant) FormatDeliveryTime() string {
	if r.Estimates == nil || r.Estimates.Total.Mean == nil {
		return "-"
	}
	return fmt.Sprintf("%d minutes", *r.Estimates.Total.Mean)
}

// FormatPhone renders phone number.
func (r Restaurant) FormatPhone() string {
	if r.Phone == "" {
		return "-"
	}
	if len(r.Phone) <= 3 {
		return r.Phone
	}
	return r.Phone[:3] + " " + r.Phone[3:]
}

// FormatRating renders detailed restaurant rating.
func (r Restaurant) FormatRating() string {
	if r.Rating == nil {
		return "(No rating)"
	}
	return fmt.Sprintf("%s (%.1f / %d reviews)", r.Rating.Text, r.Rating.Score, r.Rating.Volume)
}

// FormatTags renders restaurant tags.
func (r Restaurant) FormatTags() string {
	if len(r.FoodTags) == 0 {
		return "-"
	}
	return strings.Join(capitalizeWords(r.FoodTags), ", ")
}

// FormatPaymentMethods renders payment methods.
func (r Restaurant) FormatPaymentMethods() string {
	return strings.Join(capitalizeWords(r.AllowedPaymentMethods), ", ")
}

// FormatDeliveryMethods renders delivery methods.
func (r Restaurant) FormatDeliveryMethods() string {
	if len(r.DeliveryMethods) == 0 {
		return "(No delivery)"
	}
	return strings.Join(capitalizeWords(r.DeliveryMethods), ", ")
}
