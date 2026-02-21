package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

var objectIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

func newCheckoutCommand(deps Dependencies) *cobra.Command {
	checkout := &cobra.Command{
		Use:   "checkout",
		Short: "Inspect checkout pricing projections (preview only).",
	}
	checkout.AddCommand(newCheckoutPreviewCommand(deps))
	return checkout
}

func newCheckoutPreviewCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var deliveryMode string
	var tip int
	var promoCode string
	var venueID string
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview checkout rows and payable total (no order placement).",
		Long: "Preview-only checkout estimation.\n\n" +
			"This command does not place orders. Location overrides affect the quote preview only; actual order placement in Wolt uses the delivery address selected in your Wolt account.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)

			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}

			var latPtr *float64
			var lonPtr *float64
			if latSet {
				latPtr = &lat
			}
			if lonSet {
				lonPtr = &lon
			}
			location, profile, err := resolveLocation(
				cmd.Context(),
				deps,
				latPtr,
				lonPtr,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				&auth,
				cmd,
			)
			if err != nil {
				return err
			}

			page, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketsPage(cmd.Context(), location, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			basket, basketSelection, selectionWarnings := selectBasketWithMeta(page, venueID)
			if basket == nil {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_EMPTY_CART",
					"No basket found for checkout preview.",
				)
			}

			checkoutPayload, checkoutWarnings, err := buildCheckoutPayload(
				cmd.Context(),
				deps,
				basket,
				location,
				deliveryMode,
				tip,
				promoCode,
			)
			if err != nil {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_CHECKOUT_PAYLOAD_ERROR",
					err.Error(),
				)
			}
			payload, checkoutAuthWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.CheckoutPreview(cmd.Context(), checkoutPayload, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}

			payableAmount := asInt(payload["payable_amount"])
			payableFormatted := asString(asMap(asMap(payload["payment_breakdown"])["total"])["formatted_amount"])
			if payableFormatted == "" {
				payableFormatted = findTotalFormattedAmount(payload)
			}
			if payableFormatted == "" {
				payableFormatted = formatMinorAmount(payableAmount, inferCurrency(asString(asMap(basket)["total"])))
			}
			data := map[string]any{
				"basket_id":  asString(basket["id"]),
				"venue_id":   asString(asMap(basket["venue"])["id"]),
				"venue_name": asString(asMap(basket["venue"])["name"]),
				"venue_slug": asString(
					coalesceAny(
						asMap(basket["venue"])["slug"],
						asMap(basket["venue"])["venue_slug"],
						asMap(basket["venue"])["public_slug"],
						asMap(basket["venue"])["url_slug"],
					),
				),
				"selection": basketSelection,
				"payable_amount": map[string]any{
					"amount":           payableAmount,
					"formatted_amount": emptyToNil(payableFormatted),
				},
				"checkout_rows":    coalesceAny(payload["checkout_rows"], []any{}),
				"delivery_configs": coalesceAny(payload["delivery_configs"], []any{}),
				"offers":           coalesceAny(payload["offers"], map[string]any{"selectable": []any{}, "applied": []any{}}),
				"tip_config":       coalesceAny(payload["tip_config"], map[string]any{}),
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildCheckoutPreviewTable(data), flags.Output)
			}
			checkoutWarnings = append(checkoutWarnings, authWarnings...)
			checkoutWarnings = append(checkoutWarnings, checkoutAuthWarnings...)
			checkoutWarnings = append(checkoutWarnings, selectionWarnings...)
			env := output.BuildEnvelope(profile, flags.Locale, data, checkoutWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&deliveryMode, "delivery-mode", "standard", "Delivery mode: standard, priority, or schedule.")
	cmd.Flags().IntVar(&tip, "tip", 0, "Tip amount in minor units.")
	cmd.Flags().StringVar(&promoCode, "promo-code", "", "Promo code identifier to forward into checkout discount IDs.")
	cmd.Flags().StringVar(&venueID, "venue-id", "", "Restrict preview to one venue basket.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for checkout preview. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for checkout preview. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func buildCheckoutPayload(
	ctx context.Context,
	deps Dependencies,
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

	venue := asMap(basket["venue"])
	venueID := strings.TrimSpace(asString(venue["id"]))
	currency := inferCurrency(asString(basket["total"]))
	if currency == "" {
		currency = "EUR"
	}
	country := strings.TrimSpace(asString(venue["country"]))
	warnings := []string{}
	itemDetails := map[string]map[string]any{}
	categoryIDsByItemID := map[string]string{}
	assortmentPayload := map[string]any{}

	venueSlug := resolveBasketVenueSlug(venue)
	if venueSlug != "" && deps.Wolt != nil {
		if payload, err := deps.Wolt.AssortmentByVenueSlug(ctx, venueSlug); err == nil {
			assortmentPayload = payload
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		} else {
			warnings = append(warnings, fmt.Sprintf("unable to load venue assortment payload for category mapping (slug=%s)", venueSlug))
		}
		if payload, err := deps.Wolt.VenuePageStatic(ctx, venueSlug); err == nil {
			mergeCheckoutCategoryIndexes(categoryIDsByItemID, buildCheckoutCategoryIDIndex(payload))
		}
	}

	menuItems := make([]any, 0, len(asSlice(basket["items"])))
	for _, value := range asSlice(basket["items"]) {
		item := asMap(value)
		itemID := strings.TrimSpace(asString(item["id"]))
		count := asInt(item["count"])
		if count <= 0 {
			count = 1
		}
		price := asInt(item["price"])
		if price <= 0 {
			return nil, warnings, fmt.Errorf("unable to resolve base_price for basket item %q", itemID)
		}

		detail := map[string]any{}
		if itemID != "" && deps.Wolt != nil {
			if cached, ok := itemDetails[itemID]; ok {
				detail = cached
			} else if payload, err := deps.Wolt.VenueItemPage(ctx, venueID, itemID); err == nil {
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
			"alcohol_permille":                  asInt(coalesceAny(item["alcohol_permille"], 0)),
			"exclude_from_credits":              asBool(coalesceAny(item["exclude_from_credits"], false)),
			"exclude_from_discounts":            asBool(coalesceAny(item["exclude_from_discounts"], false)),
			"exclude_from_discounts_min_basket": asBool(coalesceAny(item["exclude_from_discounts_min_basket"], false)),
			"restrictions":                      coalesceAny(item["restrictions"], []any{}),
			"age_limit":                         coalesceAny(item["age_limit"], nil),
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
	if id := strings.TrimSpace(asString(item["category_id"])); id != "" {
		return id
	}
	if category := asMap(item["category"]); category != nil {
		if id := strings.TrimSpace(asString(coalesceAny(category["id"], category["_id"]))); id != "" {
			return id
		}
	}
	if categoryIDs := asSlice(item["category_ids"]); len(categoryIDs) > 0 {
		if id := strings.TrimSpace(asString(categoryIDs[0])); id != "" {
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
	if id := strings.TrimSpace(asString(payload["category_id"])); id != "" {
		return id
	}
	if category := asMap(payload["category"]); category != nil {
		if id := strings.TrimSpace(asString(coalesceAny(category["id"], category["_id"]))); id != "" {
			return id
		}
	}
	if categoryIDs := asSlice(payload["category_ids"]); len(categoryIDs) > 0 {
		if id := strings.TrimSpace(asString(categoryIDs[0])); id != "" {
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
		if slug := strings.TrimSpace(asString(candidate)); slug != "" {
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
			if categories := asSlice(value["categories"]); len(categories) > 0 {
				for _, categoryNode := range categories {
					collectCheckoutCategoryMappings(categoryNode, index)
				}
			}
			if menuItems := asSlice(value["menu_items"]); len(menuItems) > 0 {
				for _, menuItemNode := range menuItems {
					menuItem := asMap(menuItemNode)
					if menuItem == nil {
						continue
					}
					itemID := strings.TrimSpace(asString(coalesceAny(menuItem["item_id"], menuItem["id"])))
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
	category := asMap(node)
	if category == nil {
		return
	}
	categoryID := strings.TrimSpace(asString(coalesceAny(category["category_id"], category["id"], category["_id"])))
	if categoryID == "" {
		return
	}
	for _, itemNode := range asSlice(category["item_ids"]) {
		itemID := strings.TrimSpace(asString(itemNode))
		if itemID == "" {
			continue
		}
		index[itemID] = categoryID
	}
	for _, itemNode := range asSlice(category["items"]) {
		itemID := strings.TrimSpace(asString(itemNode))
		if item := asMap(itemNode); item != nil {
			itemID = strings.TrimSpace(asString(coalesceAny(item["item_id"], item["id"])))
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
	categoryIDs := asSlice(item["category_ids"])
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
	for _, spec := range extractOptionSpecs(detail) {
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
	options := make([]any, 0, len(asSlice(raw)))
	for _, optionValue := range asSlice(raw) {
		option := asMap(optionValue)
		if option == nil {
			continue
		}

		values := make([]any, 0, len(asSlice(option["values"])))
		for _, selectedValue := range asSlice(option["values"]) {
			value := asMap(selectedValue)
			if value == nil {
				continue
			}
			valueID := strings.TrimSpace(asString(value["id"]))
			if valueID == "" {
				continue
			}
			count := asInt(value["count"])
			if count <= 0 {
				count = 1
			}
			price := asInt(value["price"])
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
			"id":     strings.TrimSpace(asString(option["id"])),
			"values": values,
		})
	}
	return options
}

func findTotalFormattedAmount(payload map[string]any) string {
	for _, value := range asSlice(payload["checkout_rows"]) {
		row := asMap(value)
		if asString(row["template"]) != "price_total_amount_row" {
			continue
		}
		return strings.TrimSpace(asString(asMap(row["price_total_amount"])["formatted_amount"]))
	}
	return ""
}

func buildCheckoutPreviewTable(data map[string]any) string {
	summaryRows := [][]string{
		{"Basket ID", fallbackString(asString(data["basket_id"]), "-")},
		{"Venue ID", fallbackString(asString(data["venue_id"]), "-")},
		{"Venue name", fallbackString(asString(data["venue_name"]), "-")},
		{"Venue slug", fallbackString(asString(data["venue_slug"]), "-")},
		{"Payable total", fallbackString(asString(asMap(data["payable_amount"])["formatted_amount"]), "-")},
	}
	if selection := asMap(data["selection"]); selection != nil {
		summaryRows = append(summaryRows, []string{"Selection mode", fallbackString(asString(selection["selection_mode"]), "-")})
		summaryRows = append(summaryRows, []string{"Baskets available", asString(selection["basket_count"])})
	}
	summary := output.RenderTable("Checkout selection", []string{"Field", "Value"}, summaryRows)

	headers := []string{"Label", "Amount"}
	rows := [][]string{}
	for _, value := range asSlice(data["checkout_rows"]) {
		row := asMap(value)
		template := asString(row["template"])
		switch template {
		case "amount_row":
			rows = append(rows, []string{
				asString(row["label"]),
				fallbackString(asString(asMap(row["amount"])["formatted_amount"]), "-"),
			})
		case "price_total_amount_row":
			rows = append(rows, []string{
				fallbackString(asString(row["label"]), "Total"),
				fallbackString(asString(asMap(row["price_total_amount"])["formatted_amount"]), "-"),
			})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"Total", fallbackString(asString(asMap(data["payable_amount"])["formatted_amount"]), "-")})
	}
	rowsTable := output.RenderTable("Checkout rows", headers, rows)
	return summary + "\n\n" + rowsTable
}
