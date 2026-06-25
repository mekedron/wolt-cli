package cli

import (
	"context"
	"regexp"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/checkoutpayload"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

var objectIDPattern = regexp.MustCompile(`^[a-f0-9]{24}$`)

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
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
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

func looksLikeObjectID(value string) bool {
	return objectIDPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
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
