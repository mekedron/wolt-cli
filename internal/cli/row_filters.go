package cli

import (
	"fmt"
	"sort"
	"strings"
)

type itemRowSort string

const (
	itemRowSortRecommended itemRowSort = "recommended"
	itemRowSortPrice       itemRowSort = "price"
	itemRowSortName        itemRowSort = "name"
)

type venueRowFilters struct {
	MinRatingSet      bool
	MinRating         float64
	MaxDeliveryFeeSet bool
	MaxDeliveryFee    int
	PromotionsOnly    bool
}

type itemRowFilters struct {
	MinPriceSet   bool
	MinPrice      int
	MaxPriceSet   bool
	MaxPrice      int
	HideSoldOut   bool
	DiscountsOnly bool
}

func applyVenueRowFilters(rows []any, filters venueRowFilters) []any {
	if len(rows) == 0 {
		return rows
	}
	filtered := make([]any, 0, len(rows))
	for _, rowValue := range rows {
		row := asMap(rowValue)
		if row == nil {
			continue
		}
		if filters.MinRatingSet && venueRowRating(row) < filters.MinRating {
			continue
		}
		if filters.MaxDeliveryFeeSet && asInt(asMap(row["delivery_fee"])["amount"]) > filters.MaxDeliveryFee {
			continue
		}
		if filters.PromotionsOnly && len(asSlice(row["promotions"])) == 0 {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func applyItemRowFilters(rows []any, filters itemRowFilters) []any {
	if len(rows) == 0 {
		return rows
	}
	filtered := make([]any, 0, len(rows))
	for _, rowValue := range rows {
		row := asMap(rowValue)
		if row == nil {
			continue
		}
		if filters.HideSoldOut && asBool(row["is_sold_out"]) {
			continue
		}
		if filters.DiscountsOnly && !itemHasDiscount(row) {
			continue
		}
		amount := asInt(asMap(row["base_price"])["amount"])
		if filters.MinPriceSet && amount < filters.MinPrice {
			continue
		}
		if filters.MaxPriceSet && amount > filters.MaxPrice {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func itemHasDiscount(row map[string]any) bool {
	if row == nil {
		return false
	}
	if len(asSlice(row["discounts"])) > 0 {
		return true
	}
	original := asMap(row["original_price"])
	base := asMap(row["base_price"])
	if original == nil || base == nil {
		return false
	}
	return asInt(original["amount"]) > 0 && asInt(base["amount"]) > 0 && asInt(original["amount"]) > asInt(base["amount"])
}

func parseItemRowSort(raw string) (itemRowSort, error) {
	value := itemRowSort(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return itemRowSortRecommended, nil
	}
	switch value {
	case itemRowSortRecommended, itemRowSortPrice, itemRowSortName:
		return value, nil
	default:
		return "", fmt.Errorf("invalid --sort value %q; expected one of: recommended, price, name", raw)
	}
}

func sortItemRows(rows []any, sortMode itemRowSort) {
	if len(rows) == 0 || sortMode == itemRowSortRecommended {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := asMap(rows[i])
		right := asMap(rows[j])
		switch sortMode {
		case itemRowSortPrice:
			return asInt(asMap(left["base_price"])["amount"]) < asInt(asMap(right["base_price"])["amount"])
		case itemRowSortName:
			return strings.ToLower(strings.TrimSpace(asString(left["name"]))) < strings.ToLower(strings.TrimSpace(asString(right["name"])))
		default:
			return false
		}
	})
}

func venueRowRating(row map[string]any) float64 {
	if row == nil {
		return 0
	}
	switch value := row["rating"].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}
