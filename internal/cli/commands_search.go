package cli

import (
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

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
	var enrich bool
	var showHighlights bool

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
			// Promotion + Wolt+ enrichment hits per-venue endpoints serially and is
			// slow on a full feed. Default to the fast path; the caller opts in
			// with --enrich (or implicitly via --promotions-only, which needs the
			// labels to apply correctly).
			if enrich || promotionsOnly {
				promotionAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
				enrichVenueSearchRowsWithDynamicPromotions(
					cmd.Context(),
					deps,
					data,
					nil,
					promotionAuth,
				)
			}

			if format == output.FormatTable {
				effectiveHighlights := showHighlights
				if !cmd.Flags().Changed("show-highlights") {
					effectiveHighlights = anyVenueRowHasHighlights(asSlice(data["items"]))
				}
				return writeTable(cmd, buildVenueSearchTable(data, effectiveHighlights), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query (optional; omit to list venues)")
	cmd.Flags().StringVar(
		&sortValue,
		"sort",
		string(observability.VenueSortRecommended),
		"Sort: recommended, distance, rating, delivery_time/delivery/"+
			"delivery-time, or delivery_price/fee/delivery-price",
	)
	cmd.Flags().StringVar(&typeValue, "type", "", "Venue type")
	cmd.Flags().StringVar(&category, "category", "", "Category slug")
	cmd.Flags().BoolVar(&openNow, "open-now", false, "Only include currently open venues")
	cmd.Flags().BoolVar(&woltPlus, "wolt-plus", false, "Only include Wolt+ venues")
	cmd.Flags().Float64Var(&minRating, "min-rating", 0, "Minimum venue rating score (for example 8.5)")
	cmd.Flags().IntVar(&maxDeliveryFee, "max-delivery-fee", 0, "Maximum delivery fee in minor units (for example 500 = EUR 5.00)")
	cmd.Flags().BoolVar(&promotionsOnly, "promotions-only", false, "Only include venues with promotion labels (implies --enrich).")
	cmd.Flags().BoolVar(&enrich, "enrich", false, "Fetch per-venue promotion banners and Wolt+ status (slower; off by default).")
	cmd.Flags().BoolVar(&showHighlights, "show-highlights", false, "Append a Highlights column with venue_preview_items. Default: auto (show only when at least one row has data).")
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

func buildVenueSearchTable(data map[string]any, showHighlights bool) string {
	headers := []string{"Venue", "Slug", "Tagline", "Top offer", "Rating", "Delivery", "Fee", "Wolt+"}
	if showHighlights {
		headers = append(headers, "Highlights")
	}
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
		name := formatBadgePrefix(asSlice(item["badges"])) + asString(item["name"])
		row := []string{
			name,
			fallbackString(asString(item["slug"]), "-"),
			truncateForTable(asString(item["tagline"]), 32),
			truncateForTable(asString(item["top_offer"]), 26),
			rating,
			asString(item["delivery_estimate"]),
			fee,
			boolToYesNo(asBool(item["wolt_plus"])),
		}
		if showHighlights {
			row = append(row, formatHighlightsCell(asSlice(item["menu_highlights"]), 32))
		}
		rows = append(rows, row)
	}
	return output.RenderTable("Venue search: "+asString(data["query"]), headers, rows)
}

func truncateForTable(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if max <= 1 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-1]) + "…"
}

func boolToYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
