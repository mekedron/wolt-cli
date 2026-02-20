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
			location, profile, err := resolveProfileLocation(
				cmd.Context(),
				deps,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
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
			data, warnings := observability.BuildVenueSearchResult(
				items,
				query,
				sortMode,
				venueType,
				category,
				openNow,
				woltPlus,
				limitPtr,
				offset,
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
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned rows")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		limitSet = cmd.Flags().Changed("limit")
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

			location, profile, err := resolveProfileLocation(
				cmd.Context(),
				deps,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
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

			data, itemWarnings := observability.BuildItemSearchResult(
				query,
				payloads,
				sortMode,
				category,
				limitPtr,
				offset,
				fallbackItems,
			)
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
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned rows")
	if err := cmd.MarkFlagRequired("query"); err != nil {
		panic(err)
	}
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		limitSet = cmd.Flags().Changed("limit")
	}

	return cmd
}

func buildVenueSearchTable(data map[string]any) string {
	headers := []string{"Venue", "Address", "Rating", "Delivery", "Fee", "Price", "Promotions", "Wolt+"}
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
