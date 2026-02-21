package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

type discoverFeedSort string

const (
	discoverFeedSortRecommended discoverFeedSort = "recommended"
	discoverFeedSortRating      discoverFeedSort = "rating"
	discoverFeedSortDeliveryFee discoverFeedSort = "delivery_fee"
	discoverFeedSortDelivery    discoverFeedSort = "delivery_time"
	discoverFeedSortName        discoverFeedSort = "name"
)

func newDiscoverCommand(deps Dependencies) *cobra.Command {
	discover := &cobra.Command{
		Use:   "discover",
		Short: "Read discovery feed and browse categories.",
	}
	discover.AddCommand(newDiscoverFeedCommand(deps))
	discover.AddCommand(newDiscoverCategoriesCommand(deps))
	return discover
}

func newDiscoverFeedCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool
	var woltPlus bool
	var query string
	var sortValue string
	var minRating float64
	var minRatingSet bool
	var maxDeliveryFee int
	var maxDeliveryFeeSet bool
	var promotionsOnly bool
	var limit int
	var limitSet bool
	var offset int
	var offsetSet bool
	var page int
	var pageSet bool
	var fast bool

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Show discovery feed sections and venues.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
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
			locationAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)

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
				&locationAuth,
				cmd,
			)
			if err != nil {
				return err
			}

			frontPage, err := deps.Wolt.FrontPage(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			warnings := []string{}
			sections, err := extractDiscoverSectionsFromFrontPage(frontPage)
			if err != nil {
				sections, err = deps.Wolt.Sections(cmd.Context(), location)
				if err != nil {
					return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
				}
				warnings = append(warnings, "front page sections missing; fallback endpoint used")
			}

			city := asString(asMap(frontPage["city_data"])["name"])
			if city == "" {
				city = asString(frontPage["city"])
			}
			sortMode, err := parseDiscoverFeedSort(sortValue)
			if err != nil {
				return err
			}
			if minRatingSet && minRating < 0 {
				return fmt.Errorf("--min-rating must be >= 0")
			}
			if maxDeliveryFeeSet && maxDeliveryFee < 0 {
				return fmt.Errorf("--max-delivery-fee must be >= 0")
			}
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			resolvedOffset, err := resolvePageOffset(limit, limitSet, offset, offsetSet, page, pageSet)
			if err != nil {
				return err
			}
			data := observability.BuildDiscoveryFeed(sections, city, nil, woltPlus)
			if strings.TrimSpace(query) != "" {
				filterDiscoverFeedByQuery(data, query)
				data["query"] = strings.TrimSpace(query)
			}
			filterDiscoverFeedRows(
				data,
				venueRowFilters{
					MinRatingSet:      minRatingSet,
					MinRating:         minRating,
					MaxDeliveryFeeSet: maxDeliveryFeeSet,
					MaxDeliveryFee:    maxDeliveryFee,
					PromotionsOnly:    promotionsOnly,
				},
			)
			sortDiscoverFeedRows(data, sortMode)
			data["sort"] = string(sortMode)
			paginateDiscoveryFeedRows(data, limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}

			if fast {
				data["enrichment_mode"] = "fast"
				warnings = append(warnings, "fast mode skips per-venue promotion and Wolt+ enrichment")
			} else {
				data["enrichment_mode"] = "full"
				promotionAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
				enrichDiscoverFeedRowsWithDynamicPromotions(
					cmd.Context(),
					deps,
					data,
					nil,
					promotionAuth,
				)
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildDiscoveryFeedTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for location lookup. Provide together with --lon.")
	cmd.Flags().Lookup("lat").NoOptDefVal = "0"
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for location lookup. Provide together with --lat.")
	cmd.Flags().Lookup("lon").NoOptDefVal = "0"
	cmd.Flags().BoolVar(&woltPlus, "wolt-plus", false, "Only include Wolt+ venues in the feed.")
	cmd.Flags().StringVar(&query, "query", "", "Filter venues by name or slug")
	cmd.Flags().StringVar(&sortValue, "sort", string(discoverFeedSortRecommended), "Sort strategy: recommended, rating, delivery_fee, delivery_time, name")
	cmd.Flags().Float64Var(&minRating, "min-rating", 0, "Minimum venue rating score (for example 8.5)")
	cmd.Flags().IntVar(&maxDeliveryFee, "max-delivery-fee", 0, "Maximum delivery fee in minor units (for example 500 = EUR 5.00)")
	cmd.Flags().BoolVar(&promotionsOnly, "promotions-only", false, "Only include venues with promotion labels")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned venues across sections")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned venues across sections")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (requires --limit; cannot be combined with --offset)")
	cmd.Flags().BoolVar(&fast, "fast", false, "Skip extra venue enrichment requests (faster, fewer discounts)")
	addGlobalFlags(cmd, &flags)

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		pageSet = cmd.Flags().Changed("page")
		minRatingSet = cmd.Flags().Changed("min-rating")
		maxDeliveryFeeSet = cmd.Flags().Changed("max-delivery-fee")
		return nil
	}

	return cmd
}

func extractDiscoverSectionsFromFrontPage(page map[string]any) ([]domain.Section, error) {
	sectionsRaw, ok := page["sections"]
	if !ok {
		return nil, fmt.Errorf("missing sections in front page payload")
	}
	encoded, err := json.Marshal(sectionsRaw)
	if err != nil {
		return nil, fmt.Errorf("encode sections: %w", err)
	}
	sections := []domain.Section{}
	if err := json.Unmarshal(encoded, &sections); err != nil {
		return nil, fmt.Errorf("decode sections: %w", err)
	}
	return sections, nil
}

func paginateDiscoveryFeedRows(data map[string]any, limit *int, offset int) {
	if data == nil {
		return
	}
	if offset < 0 {
		offset = 0
	}
	sections := asSlice(data["sections"])
	total := 0
	for _, sectionValue := range sections {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		total += len(asSlice(section["items"]))
	}

	start := offset
	if start > total {
		start = total
	}
	end := total
	if limit != nil {
		if *limit < 0 {
			end = start
		} else if start+*limit < end {
			end = start + *limit
		}
	}

	cursor := 0
	sectionRows := make([]any, 0, len(sections))
	for _, sectionValue := range sections {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		items := asSlice(section["items"])
		selected := make([]any, 0, len(items))
		for _, item := range items {
			if cursor >= start && cursor < end {
				selected = append(selected, item)
			}
			cursor++
		}
		if len(selected) == 0 {
			continue
		}
		sectionCopy := map[string]any{}
		for k, v := range section {
			sectionCopy[k] = v
		}
		sectionCopy["items"] = selected
		sectionRows = append(sectionRows, sectionCopy)
	}
	data["sections"] = sectionRows
	data["total"] = total
	data["count"] = end - start
	data["offset"] = offset
	if limit != nil {
		data["limit"] = *limit
	}
	setTotalPages(data, total, limit)
	if end < total {
		data["next_offset"] = end
	} else {
		delete(data, "next_offset")
	}
}

func parseDiscoverFeedSort(raw string) (discoverFeedSort, error) {
	value := discoverFeedSort(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return discoverFeedSortRecommended, nil
	}
	switch value {
	case discoverFeedSortRecommended, discoverFeedSortRating, discoverFeedSortDeliveryFee, discoverFeedSortDelivery, discoverFeedSortName:
		return value, nil
	default:
		return "", fmt.Errorf("invalid --sort value %q; expected one of: recommended, rating, delivery_fee, delivery_time, name", raw)
	}
}

func filterDiscoverFeedByQuery(data map[string]any, query string) {
	if data == nil {
		return
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return
	}
	filteredSections := make([]any, 0, len(asSlice(data["sections"])))
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		items := asSlice(section["items"])
		filteredItems := make([]any, 0, len(items))
		for _, itemValue := range items {
			item := asMap(itemValue)
			if item == nil {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(asString(item["name"])))
			slug := strings.ToLower(strings.TrimSpace(asString(item["slug"])))
			if strings.Contains(name, normalized) || strings.Contains(slug, normalized) {
				filteredItems = append(filteredItems, item)
			}
		}
		if len(filteredItems) == 0 {
			continue
		}
		sectionCopy := map[string]any{}
		for k, v := range section {
			sectionCopy[k] = v
		}
		sectionCopy["items"] = filteredItems
		filteredSections = append(filteredSections, sectionCopy)
	}
	data["sections"] = filteredSections
}

func filterDiscoverFeedRows(data map[string]any, filters venueRowFilters) {
	if data == nil {
		return
	}
	filteredSections := make([]any, 0, len(asSlice(data["sections"])))
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		filteredItems := applyVenueRowFilters(asSlice(section["items"]), filters)
		if len(filteredItems) == 0 {
			continue
		}
		sectionCopy := map[string]any{}
		for k, v := range section {
			sectionCopy[k] = v
		}
		sectionCopy["items"] = filteredItems
		filteredSections = append(filteredSections, sectionCopy)
	}
	data["sections"] = filteredSections
}

func sortDiscoverFeedRows(data map[string]any, sortMode discoverFeedSort) {
	if data == nil || sortMode == discoverFeedSortRecommended {
		return
	}
	less := func(left map[string]any, right map[string]any) bool {
		switch sortMode {
		case discoverFeedSortRating:
			return discoverFeedRating(left) > discoverFeedRating(right)
		case discoverFeedSortDeliveryFee:
			return discoverFeedDeliveryFee(left) < discoverFeedDeliveryFee(right)
		case discoverFeedSortDelivery:
			return discoverFeedDeliveryEstimate(left) < discoverFeedDeliveryEstimate(right)
		case discoverFeedSortName:
			return strings.ToLower(strings.TrimSpace(asString(left["name"]))) < strings.ToLower(strings.TrimSpace(asString(right["name"])))
		default:
			return false
		}
	}
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		items := asSlice(section["items"])
		sort.SliceStable(items, func(i, j int) bool {
			return less(asMap(items[i]), asMap(items[j]))
		})
		section["items"] = items
	}
}

func discoverFeedRating(item map[string]any) float64 {
	if item == nil {
		return 0
	}
	switch value := item["rating"].(type) {
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

func discoverFeedDeliveryFee(item map[string]any) int {
	if item == nil {
		return 0
	}
	return asInt(asMap(item["delivery_fee"])["amount"])
}

func discoverFeedDeliveryEstimate(item map[string]any) int {
	if item == nil {
		return 0
	}
	estimate := strings.TrimSpace(strings.ToLower(asString(item["delivery_estimate"])))
	for _, token := range strings.Fields(estimate) {
		normalized := strings.TrimSpace(strings.Trim(token, "-"))
		if normalized == "" {
			continue
		}
		if value, err := strconv.Atoi(normalized); err == nil {
			return value
		}
	}
	return 0
}

func newDiscoverCategoriesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List available discovery categories.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
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
			locationAuth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
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
				&locationAuth,
				cmd,
			)
			if err != nil {
				return err
			}

			sections, err := deps.Wolt.Sections(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			data := observability.BuildCategoryList(sections)

			if format == output.FormatTable {
				return writeTable(cmd, buildCategoryTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, []string{}, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for location lookup. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for location lookup. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func buildDiscoveryFeedTable(data map[string]any) string {
	headers := []string{"Section", "Venue", "Slug", "Rating", "Delivery estimate", "Delivery fee", "Price", "Promotions", "Wolt+"}
	rows := [][]string{}
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		sectionName := asString(section["title"])
		items := asSlice(section["items"])
		if len(items) == 0 {
			rows = append(rows, []string{sectionName, "-", "-", "-", "-", "-", "-", "-", "-"})
			continue
		}
		for idx, itemValue := range items {
			item := asMap(itemValue)
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
			name := asString(item["name"])
			if idx > 0 {
				sectionName = ""
			}
			rows = append(rows, []string{
				sectionName,
				name,
				fallbackString(asString(item["slug"]), "-"),
				rating,
				asString(item["delivery_estimate"]),
				fee,
				priceRange,
				promotions,
				boolToYesNo(asBool(item["wolt_plus"])),
			})
		}
	}
	title := "Discover feed: " + asString(data["city"])
	if asBool(data["wolt_plus_only"]) {
		title += " (Wolt+ only)"
	}
	return output.RenderTable(title, headers, rows)
}

func buildCategoryTable(data map[string]any) string {
	headers := []string{"Category", "Slug", "ID"}
	rows := [][]string{}
	for _, value := range asSlice(data["categories"]) {
		category := asMap(value)
		rows = append(rows, []string{asString(category["name"]), asString(category["slug"]), asString(category["id"])})
	}
	return output.RenderTable("Discover categories", headers, rows)
}
