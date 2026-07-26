package checkoutpayload

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

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
	if tip < 0 {
		return nil, nil, fmt.Errorf("tip must be zero or greater")
	}
	deliveryMode = strings.ToLower(strings.TrimSpace(deliveryMode))
	if deliveryMode == "" {
		deliveryMode = "standard"
	}
	if deliveryMode != "standard" && deliveryMode != "priority" {
		return nil, nil, fmt.Errorf("unsupported --delivery-mode %q", deliveryMode)
	}

	venue := payloadutil.Map(basket["venue"])
	venueID := strings.TrimSpace(payloadutil.String(venue["id"]))
	warnings := []string{}
	if !looksLikeObjectID(venueID) {
		return nil, warnings, fmt.Errorf("unable to resolve canonical venue id for checkout preview")
	}
	itemDetails := map[string]map[string]any{}
	categoryIDsByItemID := map[string]string{}
	assortmentPayload := map[string]any{}
	staticPayload := map[string]any{}

	venueSlug := resolveBasketVenueSlug(venue)
	if venueSlug != "" && venuePageStatic != nil {
		if payload, err := venuePageStatic(ctx, venueSlug); err == nil {
			staticPayload = payload
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		}
	}
	currency := resolveCheckoutCurrency(basket, staticPayload)
	if currency == "" {
		return nil, warnings, fmt.Errorf("unable to resolve venue currency for checkout preview")
	}
	country := strings.TrimSpace(payloadutil.String(venue["country"]))
	if country == "" {
		staticVenue := payloadutil.Map(staticPayload["venue"])
		country = strings.TrimSpace(payloadutil.String(staticVenue["country"]))
	}

	if venueSlug != "" && wolt != nil {
		if payload, err := wolt.AssortmentByVenueSlug(ctx, venueSlug); err == nil {
			assortmentPayload = payload
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		} else {
			warnings = append(warnings, fmt.Sprintf("unable to load venue assortment payload for category mapping (slug=%s)", venueSlug))
		}
	}

	basketItems, err := payloadutil.BasketItemRowsForMutation(basket)
	if err != nil {
		return nil, warnings, fmt.Errorf("basket cannot be used for checkout preview: %w", err)
	}
	if len(basketItems) == 0 {
		return nil, warnings, fmt.Errorf("basket has no items for checkout preview")
	}
	menuItems := make([]any, 0, len(basketItems))
	for index, item := range basketItems {
		itemID := strings.TrimSpace(payloadutil.String(item["id"]))
		if itemID == "" {
			return nil, warnings, fmt.Errorf("unable to resolve item id for basket item at index %d", index)
		}
		count := payloadutil.Int(item["count"])
		if count <= 0 {
			count = 1
		}
		replacement, replacementErr := payloadutil.BuildBasketUpsertItem(item, count)
		if replacementErr != nil {
			return nil, warnings, fmt.Errorf("basket cannot be used for checkout preview: %w", replacementErr)
		}
		price := payloadutil.MinorAmount(replacement["price"])

		detail := map[string]any{}
		if wolt != nil {
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
			return nil, warnings, fmt.Errorf("unable to resolve category_id for basket item %q", itemID)
		}
		categoryIDs := resolveCheckoutCategoryIDs(item, categoryID)
		valuePrices := buildOptionValuePriceIndex(detail)
		options := buildCheckoutOptions(replacement["options"], valuePrices)
		endAmount, ok := payloadutil.CheckedMultiplyInt(count, price)
		if !ok {
			return nil, warnings, fmt.Errorf("basket item %q total exceeds the supported integer range", itemID)
		}

		menuItems = append(menuItems, map[string]any{
			"id":                                itemID,
			"venue_id":                          venueID,
			"count":                             count,
			"base_price":                        price,
			"end_amount":                        endAmount,
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

// resolveCheckoutCurrency never guesses a market-wide default. Sending EUR for
// a GEL/SEK/DKK basket can mis-price or invalidate the request, so checkout
// fails closed when neither the basket nor venue payload states a currency.
func resolveCheckoutCurrency(basket map[string]any, venuePayload map[string]any) string {
	if currency := payloadutil.CurrencyFromBasket(basket); currency != "" {
		return currency
	}
	return payloadutil.CurrencyFromVenuePayload(venuePayload)
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
	if id := strings.TrimSpace(fallback[strings.ToLower(strings.TrimSpace(itemID))]); id != "" {
		return id
	}
	return ""
}

func resolveCheckoutCategoryIDFromDetails(detail map[string]any, itemID string) string {
	if strings.TrimSpace(itemID) == "" {
		return ""
	}
	categoryIDsByItemID := buildCheckoutCategoryIDIndex(detail)
	if id := strings.TrimSpace(categoryIDsByItemID[strings.ToLower(strings.TrimSpace(itemID))]); id != "" {
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
	var walk func(any, bool)
	walk = func(node any, categoryContainer bool) {
		switch value := node.(type) {
		case map[string]any:
			if categoryContainer {
				collectCheckoutCategoryMappings(value, index)
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
						recordCheckoutCategoryMapping(index, itemID, categoryID)
					}
				}
			}
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				nested := value[key]
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "category":
					walk(nested, true)
				case "categories", "subcategories":
					for _, categoryNode := range payloadutil.Slice(nested) {
						walk(categoryNode, true)
					}
				default:
					walk(nested, false)
				}
			}
		case []any:
			for _, nested := range value {
				walk(nested, categoryContainer)
			}
		}
	}
	walk(payload, false)
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
		recordCheckoutCategoryMapping(index, itemID, categoryID)
	}
	for _, itemNode := range payloadutil.Slice(category["items"]) {
		itemID := strings.TrimSpace(payloadutil.String(itemNode))
		if item := payloadutil.Map(itemNode); item != nil {
			itemID = strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(item["item_id"], item["id"])))
		}
		if itemID == "" {
			continue
		}
		recordCheckoutCategoryMapping(index, itemID, categoryID)
	}
}

func recordCheckoutCategoryMapping(index map[string]string, itemID, categoryID string) {
	itemKey := strings.ToLower(strings.TrimSpace(itemID))
	categoryID = strings.TrimSpace(categoryID)
	if itemKey == "" || categoryID == "" {
		return
	}
	if _, exists := index[itemKey]; exists {
		return
	}
	index[itemKey] = categoryID
}

func mergeCheckoutCategoryIndexes(target map[string]string, source map[string]string) {
	if target == nil || len(source) == 0 {
		return
	}
	itemIDs := make([]string, 0, len(source))
	for itemID := range source {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)
	for _, itemID := range itemIDs {
		recordCheckoutCategoryMapping(target, itemID, source[itemID])
	}
}

func looksLikeObjectID(value string) bool {
	return domain.IsObjectID(value)
}

func resolveCheckoutCategoryIDs(item map[string]any, categoryID string) []any {
	canonicalID := strings.TrimSpace(categoryID)
	rawIDs := make([]string, 0, len(payloadutil.Slice(item["category_ids"])))
	seen := map[string]struct{}{}
	canonicalPresent := false
	for _, rawID := range payloadutil.Slice(item["category_ids"]) {
		id := strings.TrimSpace(payloadutil.String(rawID))
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if canonicalID != "" && strings.EqualFold(id, canonicalID) {
			canonicalPresent = true
		}
		rawIDs = append(rawIDs, id)
	}

	if canonicalID == "" {
		categoryIDs := make([]any, 0, len(rawIDs))
		for _, id := range rawIDs {
			categoryIDs = append(categoryIDs, id)
		}
		return categoryIDs
	}
	if !canonicalPresent {
		// A category_ids array that omits the separately resolved category_id
		// is stale or belongs to a different item snapshot. Do not send
		// contradictory category identities to checkout.
		return []any{canonicalID}
	}

	categoryIDs := make([]any, 0, len(rawIDs))
	categoryIDs = append(categoryIDs, canonicalID)
	for _, id := range rawIDs {
		if !strings.EqualFold(id, canonicalID) {
			categoryIDs = append(categoryIDs, id)
		}
	}
	return categoryIDs
}

func buildOptionValuePriceIndex(detail map[string]any) map[string]int {
	index := map[string]int{}
	for _, spec := range payloadutil.ExtractOptionSpecs(detail) {
		for valueID, value := range spec.Values {
			valueID = strings.TrimSpace(valueID)
			if valueID == "" || !value.HasPrice {
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
			price := payloadutil.MinorAmount(value["price"])
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
