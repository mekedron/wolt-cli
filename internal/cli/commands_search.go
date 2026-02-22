package cli

import (
	"fmt"

	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newSearchCommand(deps Dependencies) *cobra.Command {
	search := &cobra.Command{
		Use:   "search",
		Short: "Search venues and menu items by query.",
	}
	search.AddCommand(newSearchVenuesCommand(deps))
	search.AddCommand(newSearchItemsCommand(deps))
	return search
}

func newSearchVenuesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var query string
	var sortValue string
	var typeValue string
	var category string
	var openNow bool
	var woltPlus bool
	var limit int
	var limitSet bool
	var offset int
	var offsetSet bool
	var page int
	var pageSet bool
	var minRating float64
	var minRatingSet bool
	var maxDeliveryFee int
	var maxDeliveryFeeSet bool
	var promotionsOnly bool

	cmd := &cobra.Command{
		Use:   "venues",
		Short: "Search venues by query.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			sortMode, err := observability.ParseVenueSort(sortValue)
			if err != nil {
				return err
			}
			var venueType *observability.VenueType
			if typeValue != "" {
				parsedType, err := observability.ParseVenueType(typeValue)
				if err != nil {
					return err
				}
				venueType = &parsedType
			}
			locationAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			location, profile, err := resolveProfileLocation(
				cmd.Context(),
				deps,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				&locationAuth,
				cmd,
			)
			if err != nil {
				return err
			}
			items, err := deps.Wolt.Items(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			resolvedOffset, err := resolvePageOffset(limit, limitSet, offset, offsetSet, page, pageSet)
			if err != nil {
				return err
			}
			if minRatingSet && minRating < 0 {
				return fmt.Errorf("--min-rating must be >= 0")
			}
			if maxDeliveryFeeSet && maxDeliveryFee < 0 {
				return fmt.Errorf("--max-delivery-fee must be >= 0")
			}
			data, warnings := observability.BuildVenueSearchResult(
				items,
				query,
				sortMode,
				venueType,
				category,
				openNow,
				woltPlus,
				nil,
				0,
			)
			data["items"] = applyVenueRowFilters(
				asSlice(data["items"]),
				venueRowFilters{
					MinRatingSet:      minRatingSet,
					MinRating:         minRating,
					MaxDeliveryFeeSet: maxDeliveryFeeSet,
					MaxDeliveryFee:    maxDeliveryFee,
					PromotionsOnly:    promotionsOnly,
				},
			)
			paginateFlatRows(data, "items", limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}
			promotionAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			enrichVenueSearchRowsWithDynamicPromotions(
				cmd.Context(),
				deps,
				data,
				nil,
				promotionAuth,
			)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueSearchTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query (optional; omit to list venues)")
	cmd.Flags().StringVar(&sortValue, "sort", string(observability.VenueSortRecommended), "Sort strategy")
	cmd.Flags().StringVar(&typeValue, "type", "", "Venue type")
	cmd.Flags().StringVar(&category, "category", "", "Category slug")
	cmd.Flags().BoolVar(&openNow, "open-now", false, "Only include currently open venues")
	cmd.Flags().BoolVar(&woltPlus, "wolt-plus", false, "Only include Wolt+ venues")
	cmd.Flags().Float64Var(&minRating, "min-rating", 0, "Minimum venue rating score (for example 8.5)")
	cmd.Flags().IntVar(&maxDeliveryFee, "max-delivery-fee", 0, "Maximum delivery fee in minor units (for example 500 = EUR 5.00)")
	cmd.Flags().BoolVar(&promotionsOnly, "promotions-only", false, "Only include venues with promotion labels")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned rows")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (requires --limit; cannot be combined with --offset)")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		pageSet = cmd.Flags().Changed("page")
		minRatingSet = cmd.Flags().Changed("min-rating")
		maxDeliveryFeeSet = cmd.Flags().Changed("max-delivery-fee")
	}

	return cmd
}

func newSearchItemsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var query string
	var sortValue string
	var category string
	var limit int
	var limitSet bool
	var offset int
	var offsetSet bool
	var page int
	var pageSet bool
	var minPrice int
	var minPriceSet bool
	var maxPrice int
	var maxPriceSet bool
	var hideSoldOut bool
	var discountsOnly bool

	cmd := &cobra.Command{
		Use:   "items",
		Short: "Search menu items by query.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return fmt.Errorf("%s", requiredArg("--query"))
			}
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			sortMode, err := observability.ParseItemSort(sortValue)
			if err != nil {
				return err
			}

			locationAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			location, profile, err := resolveProfileLocation(
				cmd.Context(),
				deps,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				&locationAuth,
				cmd,
			)
			if err != nil {
				return err
			}

			fallbackItems, err := deps.Wolt.Items(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}

			payloads := []map[string]any{}
			warnings := []string{}
			if payload, err := deps.Wolt.Search(cmd.Context(), location, query); err == nil {
				payloads = append(payloads, payload)
			} else {
				warnings = append(warnings, "search endpoint unavailable; using basic fallback data")
			}

			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			resolvedOffset, err := resolvePageOffset(limit, limitSet, offset, offsetSet, page, pageSet)
			if err != nil {
				return err
			}
			if minPriceSet && minPrice < 0 {
				return fmt.Errorf("--min-price must be >= 0")
			}
			if maxPriceSet && maxPrice < 0 {
				return fmt.Errorf("--max-price must be >= 0")
			}
			if minPriceSet && maxPriceSet && minPrice > maxPrice {
				return fmt.Errorf("--min-price cannot be greater than --max-price")
			}

			data, itemWarnings := observability.BuildItemSearchResult(
				query,
				payloads,
				sortMode,
				category,
				nil,
				0,
				fallbackItems,
			)
			data["items"] = applyItemRowFilters(
				asSlice(data["items"]),
				itemRowFilters{
					MinPriceSet:   minPriceSet,
					MinPrice:      minPrice,
					MaxPriceSet:   maxPriceSet,
					MaxPrice:      maxPrice,
					HideSoldOut:   hideSoldOut,
					DiscountsOnly: discountsOnly,
				},
			)
			paginateFlatRows(data, "items", limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}
			warnings = append(warnings, itemWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildItemSearchTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().StringVar(&sortValue, "sort", string(observability.ItemSortRelevance), "Sort strategy")
	cmd.Flags().StringVar(&category, "category", "", "Category slug")
	cmd.Flags().IntVar(&minPrice, "min-price", 0, "Minimum item base price in minor units")
	cmd.Flags().IntVar(&maxPrice, "max-price", 0, "Maximum item base price in minor units")
	cmd.Flags().BoolVar(&hideSoldOut, "hide-sold-out", false, "Exclude sold-out items")
	cmd.Flags().BoolVar(&discountsOnly, "discounts-only", false, "Only include items with discounts")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned rows")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (requires --limit; cannot be combined with --offset)")
	if err := cmd.MarkFlagRequired("query"); err != nil {
		panic(err)
	}
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		pageSet = cmd.Flags().Changed("page")
		minPriceSet = cmd.Flags().Changed("min-price")
		maxPriceSet = cmd.Flags().Changed("max-price")
	}

	return cmd
}

func buildVenueSearchTable(data map[string]any) string {
	headers := []string{"Venue", "Slug", "Address", "Rating", "Delivery", "Fee", "Price", "Promotions", "Wolt+"}
	rows := [][]string{}
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		rating := asString(item["rating"])
		if rating == "" {
			rating = "-"
		}
		fee := asString(asMap(item["delivery_fee"])["formatted_amount"])
		if fee == "" {
			fee = "-"
		}
		priceRange := asString(item["price_range_scale"])
		if priceRange == "" {
			priceRange = "-"
		}
		promotions := stringsJoin(asSlice(item["promotions"]), ", ")
		if promotions == "" {
			promotions = "-"
		}
		rows = append(rows, []string{
			asString(item["name"]),
			fallbackString(asString(item["slug"]), "-"),
			asString(item["address"]),
			rating,
			asString(item["delivery_estimate"]),
			fee,
			priceRange,
			promotions,
			boolToYesNo(asBool(item["wolt_plus"])),
		})
	}
	return output.RenderTable("Venue search: "+asString(data["query"]), headers, rows)
}

func buildItemSearchTable(data map[string]any) string {
	headers := []string{"Item", "Venue", "Price", "Sold out"}
	rows := [][]string{}
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		price := asString(asMap(item["base_price"])["formatted_amount"])
		if price == "" {
			price = "-"
		}
		venue := asString(item["venue_slug"])
		if venue == "" {
			venue = asString(item["venue_id"])
		}
		if venue == "" {
			venue = "-"
		}
		rows = append(rows, []string{
			asString(item["name"]),
			venue,
			price,
			boolToYesNo(asBool(item["is_sold_out"])),
		})
	}
	return output.RenderTable("Item search: "+asString(data["query"]), headers, rows)
}

func boolToYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
