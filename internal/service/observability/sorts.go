package observability

import (
	"fmt"
	"strings"
)

// VenueSort controls venue ordering.
type VenueSort string

const (
	VenueSortRecommended   VenueSort = "recommended"
	VenueSortDistance      VenueSort = "distance"
	VenueSortRating        VenueSort = "rating"
	VenueSortDeliveryPrice VenueSort = "delivery_price"
	VenueSortDeliveryTime  VenueSort = "delivery_time"
)

var venueSortInputValues = []string{
	string(VenueSortRecommended),
	string(VenueSortDistance),
	string(VenueSortRating),
	string(VenueSortDeliveryPrice),
	string(VenueSortDeliveryTime),
	"delivery",
	"fee",
	"delivery-price",
	"delivery-time",
}

// VenueSortInputValues returns the public venue-sort values accepted by the
// CLI and MCP tools. The short values are compatibility aliases:
// delivery means delivery_time and fee means delivery_price.
func VenueSortInputValues() []string {
	return append([]string(nil), venueSortInputValues...)
}

// ParseVenueSort parses venue sort value. Accepts hyphenated aliases
// ("delivery-time", "delivery-price") and the shorter public aliases
// ("delivery", "fee") alongside the canonical underscored forms.
func ParseVenueSort(value string) (VenueSort, error) {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	switch normalized {
	case "delivery":
		normalized = string(VenueSortDeliveryTime)
	case "fee":
		normalized = string(VenueSortDeliveryPrice)
	}
	v := VenueSort(normalized)
	if v == "" {
		return VenueSortRecommended, nil
	}
	switch v {
	case VenueSortRecommended, VenueSortDistance, VenueSortRating, VenueSortDeliveryPrice, VenueSortDeliveryTime:
		return v, nil
	default:
		return "", fmt.Errorf(
			"invalid venue sort %q; allowed: %s",
			value,
			strings.Join(venueSortInputValues, ", "),
		)
	}
}

// VenueType controls product-line filter.
type VenueType string

const (
	VenueTypeRestaurant VenueType = "restaurant"
	VenueTypeGrocery    VenueType = "grocery"
	VenueTypePharmacy   VenueType = "pharmacy"
	VenueTypeRetail     VenueType = "retail"
)

// ParseVenueType parses venue type value.
func ParseVenueType(value string) (VenueType, error) {
	v := VenueType(strings.ToLower(strings.TrimSpace(value)))
	switch v {
	case VenueTypeRestaurant, VenueTypeGrocery, VenueTypePharmacy, VenueTypeRetail:
		return v, nil
	default:
		return "", fmt.Errorf("invalid venue type %q", value)
	}
}
