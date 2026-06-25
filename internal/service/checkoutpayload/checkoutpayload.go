package checkoutpayload

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

var objectIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

type VenuePageStaticFunc func(context.Context, string) (map[string]any, error)

// Build constructs the current Wolt web checkout preview payload.
func Build(
	ctx context.Context,
	wolt woltgateway.API,
	venuePageStatic VenuePageStaticFunc,
	basket map[string]any,
	location domain.Location,
	deliveryMode string,
	tip int,
	promoCode string,
) (map[string]any, []string, error) {
	deliveryMode = strings.ToLower(strings.TrimSpace(deliveryMode))
	if deliveryMode == "" {
		deliveryMode = "standard"
	}
	if deliveryMode != "standard" && deliveryMode != "priority" && deliveryMode != "schedule" {
		return nil, nil, fmt.Errorf("unsupported --delivery-mode %q", deliveryMode)
	}

	venue := payloadutil.Map(basket["venue"])
	venueID := strings.TrimSpace(payloadutil.String(venue["id"]))
	currency := payloadutil.InferCurrency(payloadutil.String(basket["total"]))
	if currency == "" {
		currency = "EUR"
	}
	country := strings.TrimSpace(payloadutil.String(venue["country"]))
	warnings := []string{}
	itemDetails := map[string]map[string]any{}
	categoryIDsByItemID := map[string]string{}
	assortmentPayload := map[string]any{}

	venueSlug := resolveBasketVenueSlug(venue)
	if venueSlug != "" && wolt != nil {
		if payload, err := wolt.AssortmentByVenueSlug(ctx, venueSlug); err == nil {
			assortmentPayload = payload
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		} else {
			warnings = append(warnings, fmt.Sprintf("unable to load venue assortment payload for category mapping (slug=%s)", venueSlug))
		}
		if payload, err := venuePageStatic(ctx, venueSlug); err == nil {
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		}
	}

	menuItems := make([]any, 0, len(payloadutil.Slice(basket["items"])))
	for _, value := range payloadutil.Slice(basket["items"]) {
		item := payloadutil.Map(value)
		itemID := strings.TrimSpace(payloadutil.String(item["id"]))
		count := payloadutil.Int(item["count"])
		if count <= 0 {
			count = 1
		}
		price := payloadutil.Int(item["price"])
		if price <= 0 {
			return nil, warnings, fmt.Errorf("unable to resolve base_price for basket item %q", itemID)
		}

		detail := map[string]any{}
		if itemID != "" && wolt != nil {
			if cached, ok := itemDetails[itemID]; ok {
				detail = cached
			} else if payload, err := wolt.VenueItemPage(ctx, venueID, itemID); err == nil {
				detail = payload
				itemDetails[itemID] = payload
				mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
			} else if len(assortmentPayload) > 0 {
				detail = assortmentPayload
				itemDetails[itemID] = assortmentPayload
			} else {
				warnings = append(warnings, fmt.Sprintf("unable to enrich checkout payload for item %s; using basket defaults", itemID))
			}
		}

		categoryID := resolveCheckoutCategoryID(item, detail, itemID, categoryIDsByItemID)
		if categoryID == "" {
			if looksLikeObjectID(itemID) {
				categoryID = itemID
				warnings = append(warnings, fmt.Sprintf("unable to resolve category_id for item %s; falling back to item id", itemID))
			} else {
				return nil, warnings, fmt.Errorf("unable to resolve category_id for basket item %q", itemID)
			}
		}
		categoryIDs := resolveCheckoutCategoryIDs(item, categoryID)
		valuePrices := buildOptionValuePriceIndex(detail)
		options := buildCheckoutOptions(item["options"], valuePrices)

		menuItems = append(menuItems, map[string]any{
			"id":                                itemID,
			"venue_id":                          venueID,
			"count":                             count,
			"base_price":                        price,
			"end_amount":                        count * price,
			"is_weighted_item":                  false,
			"category_id":                       categoryID,
			"category_ids":                      categoryIDs,
			"alcohol_permille":                  payloadutil.Int(payloadutil.CoalesceAny(item["alcohol_permille"], 0)),
			"exclude_from_credits":              payloadutil.Bool(payloadutil.CoalesceAny(item["exclude_from_credits"], false)),
			"exclude_from_discounts":            payloadutil.Bool(payloadutil.CoalesceAny(item["exclude_from_discounts"], false)),
			"exclude_from_discounts_min_basket": payloadutil.Bool(payloadutil.CoalesceAny(item["exclude_from_discounts_min_basket"], false)),
			"restrictions":                      payloadutil.CoalesceAny(item["restrictions"], []any{}),
			"age_limit":                         payloadutil.CoalesceAny(item["age_limit"], nil),
			"options":                           options,
		})
	}

	promoDiscountIDs := []any{}
	if strings.TrimSpace(promoCode) != "" {
		promoDiscountIDs = append(promoDiscountIDs, strings.TrimSpace(promoCode))
	}

	return map[string]any{
		"purchase_plan": map[string]any{
			"venue": map[string]any{
				"id":       venueID,
				"currency": currency,
				"country":  country,
			},
			"delivery_method":           "homedelivery",
			"menu_items":                menuItems,
			"use_promo_discount_ids":    promoDiscountIDs,
			"courier_tip":               tip,
			"use_cash":                  false,
			"use_credits_and_tokens":    false,
			"use_loyalty_points_amount": 0,
			"use_promo_surcharge_ids":   []any{},
			"payment_methods":           []any{},
			"is_priority_delivery":      deliveryMode == "priority",
			"delivery": map[string]any{
				"delivery_coordinates": map[string]any{
					"latitude":  location.Lat,
					"longitude": location.Lon,
				},
			},
		},
	}, warnings, nil
}

func resolveCheckoutCategoryID(item map[string]any, detail map[string]any, itemID string, fallback map[string]string) string {
	if id := strings.TrimSpace(payloadutil.String(item["category_id"])); id != "" {
		return id
	}
	if category := payloadutil.Map(item["category"]); category != nil {
		if id := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(category["id"], category["_id"]))); id != "" {
			return id
		}
	}
	if categoryIDs := payloadutil.Slice(item["category_ids"]); len(categoryIDs) > 0 {
		if id := strings.TrimSpace(payloadutil.String(categoryIDs[0])); id != "" {
			return id
		}
	}
	if detailCategory := resolveCheckoutCategoryIDFromItemLikePayload(detail); detailCategory != "" {
		return detailCategory
	}
	if id := resolveCheckoutCategoryIDFromDetails(detail, itemID); id != "" {
		return id
	}
	if id := strings.TrimSpace(fallback[itemID]); id != "" {
		return id
	}
	return ""
}

func resolveCheckoutCategoryIDFromDetails(detail map[string]any, itemID string) string {
	if strings.TrimSpace(itemID) == "" {
		return ""
	}
	categoryIDsByItemID := buildCheckoutCategoryIDIndex(detail)
	if id := strings.TrimSpace(categoryIDsByItemID[itemID]); id != "" {
		return id
	}
	return ""
}

func resolveCheckoutCategoryIDFromItemLikePayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if id := strings.TrimSpace(payloadutil.String(payload["category_id"])); id != "" {
		return id
	}
	if category := payloadutil.Map(payload["category"]); category != nil {
		if id := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(category["id"], category["_id"]))); id != "" {
			return id
		}
	}
	if categoryIDs := payloadutil.Slice(payload["category_ids"]); len(categoryIDs) > 0 {
		if id := strings.TrimSpace(payloadutil.String(categoryIDs[0])); id != "" {
			return id
		}
	}
	return ""
}

func resolveBasketVenueSlug(venue map[string]any) string {
	if venue == nil {
		return ""
	}
	candidates := []any{
		venue["slug"],
		venue["venue_slug"],
		venue["public_slug"],
		venue["url_slug"],
	}
	for _, candidate := range candidates {
		if slug := strings.TrimSpace(payloadutil.String(candidate)); slug != "" {
			return slug
		}
	}
	return ""
}

func buildCheckoutCategoryIDIndex(payload map[string]any) map[string]string {
	index := map[string]string{}
	if payload == nil {
		return index
	}
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if categories := payloadutil.Slice(value["categories"]); len(categories) > 0 {
				for _, categoryNode := range categories {
					collectCheckoutCategoryMappings(categoryNode, index)
				}
			}
			if menuItems := payloadutil.Slice(value["menu_items"]); len(menuItems) > 0 {
				for _, menuItemNode := range menuItems {
					menuItem := payloadutil.Map(menuItemNode)
					if menuItem == nil {
						continue
					}
					itemID := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(menuItem["item_id"], menuItem["id"])))
					if itemID == "" {
						continue
					}
					if categoryID := resolveCheckoutCategoryIDFromItemLikePayload(menuItem); categoryID != "" {
						index[itemID] = categoryID
					}
				}
			}
			collectCheckoutCategoryMappings(value, index)
			for _, nested := range value {
				walk(nested)
			}
		case []any:
			for _, nested := range value {
				walk(nested)
			}
		}
	}
	walk(payload)
	return index
}

func collectCheckoutCategoryMappings(node any, index map[string]string) {
	category := payloadutil.Map(node)
	if category == nil {
		return
	}
	categoryID := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(category["category_id"], category["id"], category["_id"])))
	if categoryID == "" {
		return
	}
	for _, itemNode := range payloadutil.Slice(category["item_ids"]) {
		itemID := strings.TrimSpace(payloadutil.String(itemNode))
		if itemID == "" {
			continue
		}
		index[itemID] = categoryID
	}
	for _, itemNode := range payloadutil.Slice(category["items"]) {
		itemID := strings.TrimSpace(payloadutil.String(itemNode))
		if item := payloadutil.Map(itemNode); item != nil {
			itemID = strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(item["item_id"], item["id"])))
		}
		if itemID == "" {
			continue
		}
		index[itemID] = categoryID
	}
}

func mergeCheckoutCategoryIndexes(target map[string]string, source map[string]string) {
	if target == nil || len(source) == 0 {
		return
	}
	for itemID, categoryID := range source {
		itemID = strings.TrimSpace(itemID)
		categoryID = strings.TrimSpace(categoryID)
		if itemID == "" || categoryID == "" {
			continue
		}
		if _, exists := target[itemID]; exists {
			continue
		}
		target[itemID] = categoryID
	}
}

func looksLikeObjectID(value string) bool {
	return objectIDPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

func resolveCheckoutCategoryIDs(item map[string]any, categoryID string) []any {
	categoryIDs := payloadutil.Slice(item["category_ids"])
	if len(categoryIDs) > 0 {
		return categoryIDs
	}
	if strings.TrimSpace(categoryID) == "" {
		return []any{}
	}
	return []any{categoryID}
}

func buildOptionValuePriceIndex(detail map[string]any) map[string]int {
	index := map[string]int{}
	for _, spec := range payloadutil.ExtractOptionSpecs(detail) {
		for valueID, value := range spec.Values {
			valueID = strings.TrimSpace(valueID)
			if valueID == "" {
				continue
			}
			index[valueID] = value.Price
		}
	}
	return index
}

func buildCheckoutOptions(raw any, valuePrices map[string]int) []any {
	options := make([]any, 0, len(payloadutil.Slice(raw)))
	for _, optionValue := range payloadutil.Slice(raw) {
		option := payloadutil.Map(optionValue)
		if option == nil {
			continue
		}

		values := make([]any, 0, len(payloadutil.Slice(option["values"])))
		for _, selectedValue := range payloadutil.Slice(option["values"]) {
			value := payloadutil.Map(selectedValue)
			if value == nil {
				continue
			}
			valueID := strings.TrimSpace(payloadutil.String(value["id"]))
			if valueID == "" {
				continue
			}
			count := payloadutil.Int(value["count"])
			if count <= 0 {
				count = 1
			}
			price := payloadutil.Int(value["price"])
			if inferred, ok := valuePrices[valueID]; ok {
				price = inferred
			}
			values = append(values, map[string]any{
				"id":    valueID,
				"count": count,
				"price": price,
			})
		}

		options = append(options, map[string]any{
			"id":     strings.TrimSpace(payloadutil.String(option["id"])),
			"values": values,
		})
	}
	return options
}
