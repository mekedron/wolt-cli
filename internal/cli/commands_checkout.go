package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/checkoutpayload"
	"github.com/mekedron/wolt-cli/internal/service/deliveryselection"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
	"github.com/spf13/cobra"
)

func newCheckoutCommand(deps Dependencies) *cobra.Command {
	checkout := newCheckoutPreviewCommand(deps)
	checkout.Use = "checkout"
	checkout.Short = "Preview checkout pricing without placing an order."
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
			if tip < 0 {
				return emitError(
					cmd,
					format,
					profileName,
					flags.Locale,
					flags.Output,
					"WOLT_INVALID_ARGUMENT",
					"--tip must be zero or greater",
				)
			}

			auth, err := loadRequiredAuth(cmd.Context(), deps, flags, format, cmd)
			if err != nil {
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
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
			}
			basket, basketSelection, selectionWarnings, selectionErr := selectBasketForMutationWithMeta(page, venueID)
			if selectionErr != nil {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_CHECKOUT_PAYLOAD_ERROR",
					"Basket state could not be verified for checkout preview: "+selectionErr.Error(),
				)
			}
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
			basketVenue := asMap(basket["venue"])
			basketIdentity := payloadutil.ExtractBasketVenueIdentity(basket)
			if basketIdentity.Conflict {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_VENUE_CONFLICT",
					"Checkout was not previewed because the basket contains conflicting canonical venue ids.",
				)
			}
			identityInput := fallbackString(
				basketIdentity.ID,
				fallbackString(basketIdentity.Slug, strings.TrimSpace(venueID)),
			)
			resolvedIdentity, resolveErr := resolveVenueReference(cmd.Context(), deps, identityInput)
			if resolveErr != nil {
				return resolveErr
			}
			resolvedVenueID := fallbackString(basketIdentity.ID, resolvedIdentity.VenueID)
			venueSlug := fallbackString(basketIdentity.Slug, resolvedIdentity.VenueSlug)
			if venueSlug == "" && strings.TrimSpace(venueID) != "" && !looksLikeObjectID(venueID) {
				venueSlug = normalizeVenueInput(venueID)
			}
			if !looksLikeObjectID(resolvedVenueID) || venueSlug == "" {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_ITEM_AVAILABILITY_UNKNOWN",
					"Checkout was not previewed because the canonical venue id and slug could not both be resolved.",
				)
			}
			if basketVenue == nil {
				basketVenue = map[string]any{}
				basket["venue"] = basketVenue
			}
			basketVenue["id"] = resolvedVenueID
			basketVenue["slug"] = venueSlug

			basketItemIDs := catalogitem.BasketItemIDs(basket)
			currentItems, validationWarnings, validationErr := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return requestAssortmentItemsPayload(
						cmd.Context(),
						deps,
						venueSlug,
						basketItemIDs,
						authCtx,
					)
				},
			)
			if validationErr != nil {
				return emitUpstreamError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					flags.Verbose,
					validationErr,
					validationWarnings...,
				)
			}
			availabilityIssues := catalogitem.ValidateItemIDs(currentItems, basketItemIDs)
			if len(availabilityIssues) > 0 {
				return emitErrorWithWarnings(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_CART_ITEMS_UNAVAILABLE",
					"Checkout was not previewed because the basket contains unavailable items: "+
						catalogitem.FormatValidationIssues(availabilityIssues),
					validationWarnings,
				)
			}

			checkoutPayload, checkoutWarnings, err := checkoutpayload.Build(
				cmd.Context(),
				deps.Wolt,
				func(ctx context.Context, slug string) (map[string]any, error) {
					return cachedVenuePageStatic(ctx, deps, slug)
				},
				basket,
				location,
				deliveryMode,
				tip,
				promoCode,
				checkoutpayload.WithCurrentCatalog(currentItems),
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
				combined := append(append([]string{}, authWarnings...), checkoutAuthWarnings...)
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, combined...)
			}
			requestedDeliveryMode := strings.ToLower(strings.TrimSpace(deliveryMode))
			if requestedDeliveryMode == "" {
				requestedDeliveryMode = "standard"
			}
			deliveryState := deliveryselection.Parse(payload)
			appliedDeliveryMode, appliedDeliveryConfig, modeConfirmed := deliveryState.Resolve(requestedDeliveryMode)
			if !modeConfirmed {
				checkoutWarnings = append(checkoutWarnings, authWarnings...)
				checkoutWarnings = append(checkoutWarnings, checkoutAuthWarnings...)
				checkoutWarnings = append(checkoutWarnings, selectionWarnings...)
				return emitErrorWithWarnings(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_DELIVERY_MODE_UNAVAILABLE",
					fmt.Sprintf(
						"Wolt did not offer %s delivery for this order; available: %s.",
						requestedDeliveryMode,
						strings.Join(deliveryState.AvailableModes, ", "),
					),
					checkoutWarnings,
				)
			}

			payableAmount := asInt(payload["payable_amount"])
			payableFormatted := asString(asMap(asMap(payload["payment_breakdown"])["total"])["formatted_amount"])
			if payableFormatted == "" {
				payableFormatted = findTotalFormattedAmount(payload)
			}
			if payableFormatted == "" {
				purchasePlan := asMap(checkoutPayload["purchase_plan"])
				purchaseVenue := asMap(purchasePlan["venue"])
				payableFormatted = formatMinorAmount(payableAmount, asString(purchaseVenue["currency"]))
			}
			data := map[string]any{
				"basket_id":                payloadutil.BasketID(basket),
				"venue_id":                 resolvedVenueID,
				"venue_name":               asString(asMap(basket["venue"])["name"]),
				"venue_slug":               venueSlug,
				"selection":                basketSelection,
				"requested_delivery_mode":  requestedDeliveryMode,
				"applied_delivery_mode":    appliedDeliveryMode,
				"available_delivery_modes": deliveryState.AvailableModes,
				"selected_delivery_config": appliedDeliveryConfig,
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

	cmd.Flags().StringVar(&deliveryMode, "delivery-mode", "standard", "Delivery mode: standard or priority.")
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

func looksLikeObjectID(value string) bool {
	return domain.IsObjectID(value)
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
		{"Requested delivery", fallbackString(asString(data["requested_delivery_mode"]), "-")},
		{"Applied delivery", fallbackString(asString(data["applied_delivery_mode"]), "-")},
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
