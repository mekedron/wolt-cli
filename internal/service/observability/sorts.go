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

// ParseVenueSort parses venue sort value.
func ParseVenueSort(value string) (VenueSort, error) {
	v := VenueSort(strings.ToLower(strings.TrimSpace(value)))
	if v == "" {
		return VenueSortRecommended, nil
	}
	switch v {
	case VenueSortRecommended, VenueSortDistance, VenueSortRating, VenueSortDeliveryPrice, VenueSortDeliveryTime:
		return v, nil
	default:
		return "", fmt.Errorf("invalid venue sort %q", value)
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

// ItemSort controls menu-item ordering.
type ItemSort string

const (
	ItemSortRelevance ItemSort = "relevance"
	ItemSortPrice     ItemSort = "price"
	ItemSortName      ItemSort = "name"
)

// ParseItemSort parses item sort value.
func ParseItemSort(value string) (ItemSort, error) {
	v := ItemSort(strings.ToLower(strings.TrimSpace(value)))
	if v == "" {
		return ItemSortRelevance, nil
	}
	switch v {
	case ItemSortRelevance, ItemSortPrice, ItemSortName:
		return v, nil
	default:
		return "", fmt.Errorf("invalid item sort %q", value)
	}
}
