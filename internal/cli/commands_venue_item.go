package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newVenueCommand(deps Dependencies) *cobra.Command {
	venue := &cobra.Command{
		Use:   "venue",
		Short: "Inspect venue details, menus, and opening hours.",
	}
	venue.AddCommand(newVenueShowCommand(deps))
	venue.AddCommand(newVenueCategoriesCommand(deps))
	venue.AddCommand(newVenueSearchCommand(deps))
	venue.AddCommand(newVenueMenuCommand(deps))
	venue.AddCommand(newVenueHoursCommand(deps))
	return venue
}

func newItemCommand(deps Dependencies) *cobra.Command {
	item := &cobra.Command{
		Use:   "item",
		Short: "Inspect a single menu item for a venue.",
	}
	item.AddCommand(newItemShowCommand(deps))
	item.AddCommand(newItemOptionsCommand(deps))
	return item
}

func newVenueShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var include string

	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Show venue details by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
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
			item, venueID, staticPayload, fallbackWarnings, err := resolveVenueBySlug(cmd.Context(), deps, location, slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil || strings.TrimSpace(venueID) == "" {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", slug, profile)
			}
			restaurant, err := deps.Wolt.RestaurantByID(cmd.Context(), venueID)
			if err != nil {
				if isRecoverableRestaurantError(err) {
					data, warnings := buildVenueDetailFallback(slug, venueID, item, staticPayload, splitCSV(include))
					warnings = append(warnings, fallbackWarnings...)
					if format == output.FormatTable {
						return writeTable(cmd, buildVenueDetailTable(data), flags.Output)
					}
					env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
					return writeMachinePayload(cmd, env, format, flags.Output)
				}
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data, warnings, err := observability.BuildVenueDetail(item, restaurant, splitCSV(include))
			if err != nil {
				return err
			}
			warnings = append(warnings, fallbackWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueDetailTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&include, "include", "", "Include sections: hours,tags,rating,fees")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newVenueCategoriesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "categories <slug>",
		Short: "List available venue menu categories by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}

			venueID := strings.TrimSpace(slug)
			staticWarnings := []string{}
			if payload, err := deps.Wolt.VenuePageStatic(cmd.Context(), slug); err == nil {
				if resolvedID := venueIDFromPayload(payload); strings.TrimSpace(resolvedID) != "" {
					venueID = strings.TrimSpace(resolvedID)
				}
			} else {
				staticWarnings = append(staticWarnings, "venue static page endpoint unavailable")
			}

			assortmentPayload, err := deps.Wolt.AssortmentByVenueSlug(cmd.Context(), slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data := buildVenueCategoriesData(venueID, assortmentPayload)
			if format == output.FormatTable {
				return writeTable(cmd, buildVenueCategoriesTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, staticWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildVenueCategoriesData(venueID string, assortmentPayload map[string]any) map[string]any {
	return map[string]any{
		"venue_id":         venueID,
		"loading_strategy": strings.TrimSpace(asString(assortmentPayload["loading_strategy"])),
		"categories":       collectVenueCategoryRows(assortmentPayload),
	}
}

func collectVenueCategoryRows(assortmentPayload map[string]any) []map[string]any {
	rows := []map[string]any{}
	seen := map[string]struct{}{}

	var walk func(category map[string]any, parentSlug string, level int)
	walk = func(category map[string]any, parentSlug string, level int) {
		if category == nil {
			return
		}
		subcategories := asSlice(category["subcategories"])
		slug := strings.TrimSpace(asString(coalesceAny(category["slug"], category["id"])))
		if slug != "" {
			if _, exists := seen[slug]; !exists {
				seen[slug] = struct{}{}
				rows = append(rows, map[string]any{
					"id":              strings.TrimSpace(asString(category["id"])),
					"slug":            slug,
					"name":            strings.TrimSpace(asString(coalesceAny(category["name"], category["title"], slug))),
					"parent_slug":     emptyToNil(strings.TrimSpace(parentSlug)),
					"level":           level,
					"leaf":            len(subcategories) == 0,
					"item_refs_count": len(asSlice(category["item_ids"])),
				})
			}
			parentSlug = slug
		}
		for _, rawSubcategory := range subcategories {
			walk(asMap(rawSubcategory), parentSlug, level+1)
		}
	}

	for _, rawCategory := range asSlice(assortmentPayload["categories"]) {
		walk(asMap(rawCategory), "", 0)
	}
	for _, rawSubcategory := range asSlice(assortmentPayload["subcategories"]) {
		walk(asMap(rawSubcategory), "", 0)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		leftLevel := asInt(rows[i]["level"])
		rightLevel := asInt(rows[j]["level"])
		if leftLevel != rightLevel {
			return leftLevel < rightLevel
		}
		return strings.ToLower(asString(rows[i]["name"])) < strings.ToLower(asString(rows[j]["name"]))
	})
	return rows
}

func newVenueMenuCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var category string
	var fullCatalog bool
	var includeOptions bool
	var sortValue string
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
		Use:   "menu <slug>",
		Short: "Show venue menu by slug.",
		Long: "Show venue menu by slug.\n\n" +
			"For large marketplace assortments, prefer `wolt venue search <slug> --query <text>` " +
			"or use category-first mode (`wolt venue menu <slug> --category <slug>`).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			sortMode, err := parseItemRowSort(sortValue)
			if err != nil {
				return err
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
			venueID := strings.TrimSpace(slug)
			payloads := []map[string]any{}
			warnings := []string{}
			assortmentPayload := map[string]any{}
			if payload, err := deps.Wolt.VenuePageStatic(cmd.Context(), slug); err == nil {
				payloads = append(payloads, payload)
				if resolvedID := venueIDFromPayload(payload); strings.TrimSpace(resolvedID) != "" {
					venueID = strings.TrimSpace(resolvedID)
				}
			} else {
				warnings = append(warnings, "venue static page endpoint unavailable")
			}
			var dynamicLocation *domain.Location
			if trimmed := strings.TrimSpace(flags.Address); trimmed != "" {
				if deps.Location == nil {
					return emitError(
						cmd,
						format,
						profile.Name,
						flags.Locale,
						flags.Output,
						"WOLT_LOCATION_RESOLVE_ERROR",
						"location resolver is not available",
					)
				}
				location, locationErr := deps.Location.Get(cmd.Context(), trimmed)
				if locationErr != nil {
					return emitError(
						cmd,
						format,
						profile.Name,
						flags.Locale,
						flags.Output,
						"WOLT_LOCATION_RESOLVE_ERROR",
						locationErr.Error(),
					)
				}
				dynamicLocation = &location
			} else if location, locationErr := resolveAccountLocation(cmd.Context(), deps, profile, &auth); locationErr == nil {
				dynamicLocation = &location
			}
			dynamicOptions := woltgateway.VenuePageDynamicOptions{
				Location: dynamicLocation,
				Auth:     auth,
			}
			if payload, err := deps.Wolt.VenuePageDynamic(
				cmd.Context(),
				slug,
				dynamicOptions,
			); err == nil {
				payloads = append(payloads, payload)
			} else if isUnauthorized(err) && dynamicOptions.Auth.HasCredentials() {
				dynamicOptions.Auth = woltgateway.AuthContext{}
				if payload, retryErr := deps.Wolt.VenuePageDynamic(cmd.Context(), slug, dynamicOptions); retryErr == nil {
					payloads = append(payloads, payload)
				} else {
					warnings = append(warnings, "venue dynamic page endpoint unavailable")
				}
			} else {
				warnings = append(warnings, "venue dynamic page endpoint unavailable")
			}
			if payload, err := deps.Wolt.AssortmentByVenueSlug(cmd.Context(), slug); err == nil {
				assortmentPayload = payload
				payloads = append(payloads, payload)
			} else {
				warnings = append(warnings, "venue assortment endpoint unavailable")
			}
			categorySlug := strings.TrimSpace(category)
			categoryFilter := categorySlug
			switch {
			case categorySlug != "":
				categoryPayload, err := requestAssortmentCategoryPayload(
					cmd.Context(),
					deps,
					slug,
					categorySlug,
					resolveAssortmentLanguage(flags.Locale),
					auth,
				)
				if err != nil {
					return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
				}
				categoryPayload = hydrateAssortmentCategoryItems(cmd.Context(), deps, slug, categoryPayload, auth)
				payloads = append(payloads, categoryPayload)
				categoryFilter = ""
			case isAssortmentPartial(assortmentPayload) && !fullCatalog:
				return emitError(
					cmd,
					format,
					profile.Name,
					flags.Locale,
					flags.Output,
					"WOLT_INVALID_ARGUMENT",
					fmt.Sprintf(
						"venue assortment is partial for %q; pass --category <slug> (list with \"wolt venue categories %s\"), or use \"wolt venue search %s --query <text>\"",
						slug,
						slug,
						slug,
					),
				)
			case needsVenueContentFallback(assortmentPayload, venueID):
				if isAssortmentPartial(assortmentPayload) && fullCatalog {
					warnings = append(warnings, "full catalog mode enabled for partial assortment; loading all categories (this may be slow)")
					categoryPayloads, categoryWarnings := loadAssortmentCategoryPayloads(
						cmd.Context(),
						deps,
						slug,
						resolveAssortmentLanguage(flags.Locale),
						auth,
						assortmentPayload,
						derefPositiveInt(limitPtr),
					)
					payloads = append(payloads, categoryPayloads...)
					warnings = append(warnings, categoryWarnings...)
				}
				venueContentPayloads, fallbackWarnings := loadVenueContentPayloads(cmd.Context(), deps, slug, auth, 2)
				payloads = append(payloads, venueContentPayloads...)
				warnings = append(warnings, fallbackWarnings...)
			}

			data, menuWarnings := observability.BuildVenueMenu(venueID, payloads, categoryFilter, includeOptions, nil)
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
			sortItemRows(asSlice(data["items"]), sortMode)
			data["sort"] = string(sortMode)
			paginateFlatRows(data, "items", limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}
			warnings = append(warnings, menuWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueMenuTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "Category slug")
	cmd.Flags().BoolVar(&fullCatalog, "full-catalog", false, "Force full cross-category crawl for partial assortments (can be slow).")
	cmd.Flags().BoolVar(&includeOptions, "include-options", false, "Include option group IDs")
	cmd.Flags().StringVar(&sortValue, "sort", string(itemRowSortRecommended), "Sort strategy: recommended, price, name")
	cmd.Flags().IntVar(&minPrice, "min-price", 0, "Minimum item base price in minor units")
	cmd.Flags().IntVar(&maxPrice, "max-price", 0, "Maximum item base price in minor units")
	cmd.Flags().BoolVar(&hideSoldOut, "hide-sold-out", false, "Exclude sold-out items")
	cmd.Flags().BoolVar(&discountsOnly, "discounts-only", false, "Only include items with discounts")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned rows")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (requires --limit; cannot be combined with --offset)")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		pageSet = cmd.Flags().Changed("page")
		minPriceSet = cmd.Flags().Changed("min-price")
		maxPriceSet = cmd.Flags().Changed("max-price")
	}
	return cmd
}

func newVenueSearchCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var query string
	var category string
	var includeOptions bool
	var sortValue string
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
		Use:   "search <slug>",
		Short: "Search items inside a single venue (recommended for large marketplaces).",
		Long: "Search items inside one venue by query.\n\n" +
			"Recommended for large marketplace venues where full menu traversal is heavy.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("%s", requiredArg("--query"))
			}
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			sortMode, err := parseItemRowSort(sortValue)
			if err != nil {
				return err
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

			venueID := strings.TrimSpace(slug)
			warnings := []string{}
			staticPayload := map[string]any{}
			if payload, err := deps.Wolt.VenuePageStatic(cmd.Context(), slug); err == nil {
				staticPayload = payload
				if resolvedID := venueIDFromPayload(payload); strings.TrimSpace(resolvedID) != "" {
					venueID = strings.TrimSpace(resolvedID)
				}
			} else {
				warnings = append(warnings, "venue static page endpoint unavailable")
			}

			searchPayload, err := requestAssortmentItemsSearchPayload(
				cmd.Context(),
				deps,
				slug,
				strings.TrimSpace(query),
				resolveAssortmentLanguage(flags.Locale),
				auth,
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}

			fallbackCurrency := resolveVenueSearchFallbackCurrency(staticPayload, searchPayload)
			data, searchWarnings := buildVenueItemSearchData(
				venueID,
				slug,
				query,
				category,
				searchPayload,
				fallbackCurrency,
				includeOptions,
				nil,
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
			sortItemRows(asSlice(data["items"]), sortMode)
			data["sort"] = string(sortMode)
			paginateFlatRows(data, "items", limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}
			warnings = append(warnings, searchWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueItemSearchTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().StringVar(&category, "category", "", "Category slug filter")
	cmd.Flags().BoolVar(&includeOptions, "include-options", false, "Include option-group IDs")
	cmd.Flags().StringVar(&sortValue, "sort", string(itemRowSortRecommended), "Sort strategy: recommended, price, name")
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

func venueIDFromPayload(payload map[string]any) string {
	venue := asMap(payload["venue"])
	if venue == nil {
		venue = asMap(payload["venue_raw"])
	}
	return strings.TrimSpace(asString(coalesceAny(
		venue["id"],
		payload["venue_id"],
		payload["id"],
	)))
}

func derefPositiveInt(value *int) int {
	if value == nil {
		return 0
	}
	if *value <= 0 {
		return 0
	}
	return *value
}

func isAssortmentPartial(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(asString(payload["loading_strategy"])), "partial")
}

func resolveAssortmentLanguage(locale string) string {
	language := strings.TrimSpace(locale)
	if idx := strings.Index(language, "-"); idx >= 0 {
		language = strings.TrimSpace(language[:idx])
	}
	if language == "" {
		return "en"
	}
	return language
}

func newVenueHoursCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var timezone string

	cmd := &cobra.Command{
		Use:   "hours <slug>",
		Short: "Show venue opening hours by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
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
			item, venueID, staticPayload, fallbackWarnings, err := resolveVenueBySlug(cmd.Context(), deps, location, slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil || strings.TrimSpace(venueID) == "" {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", slug, profile)
			}
			restaurant, err := deps.Wolt.RestaurantByID(cmd.Context(), venueID)
			if err != nil {
				if isRecoverableRestaurantError(err) {
					data, warnings := buildVenueHoursFallback(venueID, timezone, staticPayload)
					warnings = append(warnings, fallbackWarnings...)
					if format == output.FormatTable {
						return writeTable(cmd, buildVenueHoursTable(data), flags.Output)
					}
					env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
					return writeMachinePayload(cmd, env, format, flags.Output)
				}
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data := observability.BuildVenueHours(restaurant, timezone)
			if format == output.FormatTable {
				return writeTable(cmd, buildVenueHoursTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, fallbackWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&timezone, "timezone", "", "Timezone override")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func resolveVenueBySlug(
	ctx context.Context,
	deps Dependencies,
	location domain.Location,
	slug string,
) (*domain.Item, string, map[string]any, []string, error) {
	warnings := []string{}
	staticPayload := map[string]any{}
	item, itemErr := deps.Wolt.ItemBySlug(ctx, location, slug)
	if itemErr == nil && item != nil {
		venueID := strings.TrimSpace(item.Link.Target)
		if item.Venue != nil && strings.TrimSpace(asString(item.Venue.ID)) != "" {
			venueID = strings.TrimSpace(asString(item.Venue.ID))
		}
		if venueID != "" {
			return item, venueID, staticPayload, warnings, nil
		}
	}
	if itemErr != nil {
		warnings = append(warnings, "venue catalog lookup failed; using static venue payload fallback")
	}

	staticPayload, staticErr := deps.Wolt.VenuePageStatic(ctx, slug)
	if staticErr != nil {
		if itemErr != nil {
			return nil, "", map[string]any{}, warnings, itemErr
		}
		return nil, "", map[string]any{}, warnings, staticErr
	}
	venueID := strings.TrimSpace(venueIDFromPayload(staticPayload))
	if venueID == "" {
		return nil, "", staticPayload, warnings, nil
	}
	if item != nil {
		if item.Venue == nil {
			item.Venue = &domain.Venue{}
		}
		if strings.TrimSpace(asString(item.Venue.ID)) == "" {
			item.Venue.ID = venueID
		}
		if strings.TrimSpace(item.Venue.Slug) == "" {
			item.Venue.Slug = strings.TrimSpace(slug)
		}
		if strings.TrimSpace(item.Link.Target) == "" {
			item.Link.Target = venueID
		}
		if strings.TrimSpace(item.Title) == "" {
			item.Title = strings.TrimSpace(asString(coalesceAny(
				asMap(staticPayload["venue"])["name"],
				asMap(staticPayload["venue_raw"])["name"],
				staticPayload["name"],
				sluggifiedTitle(slug),
			)))
		}
		return item, venueID, staticPayload, warnings, nil
	}

	return fallbackVenueItemFromStaticPayload(slug, venueID, staticPayload), venueID, staticPayload, warnings, nil
}

func fallbackVenueItemFromStaticPayload(slug string, venueID string, payload map[string]any) *domain.Item {
	venuePayload := asMap(payload["venue"])
	if venuePayload == nil {
		venuePayload = asMap(payload["venue_raw"])
	}

	venue := &domain.Venue{
		ID:       venueID,
		Slug:     strings.TrimSpace(asString(coalesceAny(venuePayload["slug"], slug))),
		Name:     strings.TrimSpace(asString(venuePayload["name"])),
		Address:  strings.TrimSpace(asString(coalesceAny(venuePayload["address"], venuePayload["street_address"]))),
		Currency: strings.TrimSpace(asString(coalesceAny(venuePayload["currency"], payload["currency"]))),
	}
	if venue.Name == "" {
		venue.Name = sluggifiedTitle(slug)
	}
	if deliveryPrice := asInt(coalesceAny(
		asMap(venuePayload["delivery_fee"])["amount"],
		venuePayload["delivery_price"],
		venuePayload["delivery_price_int"],
	)); deliveryPrice > 0 {
		venue.DeliveryPriceInt = &deliveryPrice
	}

	title := venue.Name
	if title == "" {
		title = sluggifiedTitle(slug)
	}

	return &domain.Item{
		Title: title,
		Link:  domain.Link{Target: venueID},
		Venue: venue,
	}
}

func sluggifiedTitle(slug string) string {
	parts := strings.Split(strings.TrimSpace(slug), "-")
	resolved := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		resolved = append(resolved, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
	}
	if len(resolved) == 0 {
		return strings.TrimSpace(slug)
	}
	return strings.Join(resolved, " ")
}

func isRecoverableRestaurantError(err error) bool {
	var upstreamErr *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	return upstreamErr.StatusCode == 404 || upstreamErr.StatusCode == 410
}

func buildVenueDetailFallback(
	slug string,
	venueID string,
	item *domain.Item,
	staticPayload map[string]any,
	include map[string]struct{},
) (map[string]any, []string) {
	venuePayload := asMap(staticPayload["venue"])
	if venuePayload == nil {
		venuePayload = asMap(staticPayload["venue_raw"])
	}

	name := strings.TrimSpace(asString(coalesceAny(
		itemTitle(item),
		venuePayload["name"],
		staticPayload["name"],
		sluggifiedTitle(slug),
	)))
	address := strings.TrimSpace(asString(coalesceAny(
		venuePayload["address"],
		venuePayload["street_address"],
	)))
	currency := strings.TrimSpace(asString(coalesceAny(
		venuePayload["currency"],
		itemCurrency(item),
		staticPayload["currency"],
	)))
	rating := itemRating(item)

	data := map[string]any{
		"venue_id":         venueID,
		"slug":             strings.TrimSpace(asString(coalesceAny(venuePayload["slug"], slug))),
		"name":             name,
		"address":          address,
		"currency":         currency,
		"rating":           rating,
		"delivery_methods": []any{},
		"order_minimum": map[string]any{
			"amount":           nil,
			"formatted_amount": nil,
		},
	}

	if _, ok := include["hours"]; ok {
		data["opening_windows"] = []any{}
	}
	if _, ok := include["tags"]; ok {
		tags := asSlice(venuePayload["tags"])
		if len(tags) == 0 {
			tags = asSlice(staticPayload["tags"])
		}
		resolvedTags := make([]any, 0, len(tags))
		for _, value := range tags {
			tag := strings.TrimSpace(asString(value))
			if tag == "" {
				continue
			}
			resolvedTags = append(resolvedTags, tag)
		}
		data["tags"] = resolvedTags
	}
	if _, ok := include["rating"]; ok && rating != nil {
		data["rating_details"] = map[string]any{
			"score":  rating,
			"text":   nil,
			"volume": nil,
		}
	}
	if _, ok := include["fees"]; ok {
		amount := itemDeliveryFee(item)
		formatted := any(nil)
		if amount != nil {
			formatted = formatMinorAmount(*amount, currency)
		}
		data["delivery_fee"] = map[string]any{
			"amount":           amountValue(amount),
			"formatted_amount": formatted,
		}
	}

	warnings := []string{
		"restaurant detail endpoint unavailable; showing basic venue details from static payload",
		"order minimum is unavailable in basic mode and returned as null",
	}
	return data, warnings
}

func buildVenueHoursFallback(venueID string, timezone string, _ map[string]any) (map[string]any, []string) {
	resolvedTimezone := strings.TrimSpace(timezone)
	if resolvedTimezone == "" {
		resolvedTimezone = "UTC"
	}
	data := map[string]any{
		"venue_id":         venueID,
		"timezone":         resolvedTimezone,
		"opening_windows":  []any{},
		"delivery_windows": []any{},
	}
	warnings := []string{
		"restaurant detail endpoint unavailable; opening hours are unavailable in fallback mode",
	}
	return data, warnings
}

func itemTitle(item *domain.Item) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.Title)
}

func itemCurrency(item *domain.Item) string {
	if item == nil || item.Venue == nil {
		return ""
	}
	return strings.TrimSpace(item.Venue.Currency)
}

func itemDeliveryFee(item *domain.Item) *int {
	if item == nil || item.Venue == nil {
		return nil
	}
	return item.Venue.DeliveryPriceInt
}

func itemRating(item *domain.Item) any {
	if item == nil || item.Venue == nil || item.Venue.Rating == nil {
		return nil
	}
	return item.Venue.Rating.Score
}

func amountValue(amount *int) any {
	if amount == nil {
		return nil
	}
	return *amount
}

func newItemShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var includeUpsell bool

	cmd := &cobra.Command{
		Use:   "show <venue-slug> <item-id>",
		Short: "Show item details by venue slug and item ID.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueSlug := args[0]
			itemID := args[1]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)

			venueID, payload, warnings := resolveVenueItemPayloadBySlug(cmd.Context(), deps, venueSlug, itemID, auth)
			if !payloadContainsItem(payload, venueID, itemID) {
				return fmt.Errorf(
					"item %q was not found for venue slug %q; run \"wolt venue menu %s --include-options\" to list valid item IDs",
					itemID,
					venueSlug,
					venueSlug,
				)
			}
			data, itemWarnings := observability.BuildItemDetail(itemID, venueID, payload, includeUpsell)
			warnings = append(warnings, itemWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildItemDetailTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().BoolVar(&includeUpsell, "include-upsell", false, "Include upsell items")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newItemOptionsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "options <venue-slug> <item-id>",
		Short: "Show full option groups/values for an item.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueSlug := args[0]
			itemID := args[1]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)

			venueID, payload, warnings := resolveVenueItemPayloadBySlug(cmd.Context(), deps, venueSlug, itemID, auth)
			if !payloadContainsItem(payload, venueID, itemID) {
				return fmt.Errorf(
					"item %q was not found for venue slug %q; run \"wolt venue menu %s --include-options\" to list valid item IDs",
					itemID,
					venueSlug,
					venueSlug,
				)
			}
			itemGroupIDs := itemOptionGroupIDsFromPayload(payload, venueID, itemID)
			data, optionWarnings := buildItemOptionsData(venueID, itemID, payload, itemGroupIDs)
			warnings = append(warnings, optionWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildItemOptionsTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildVenueDetailTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Venue ID", asString(data["venue_id"])},
		{"Slug", asString(data["slug"])},
		{"Address", asString(data["address"])},
		{"Currency", asString(data["currency"])},
		{"Rating", fallbackString(asString(data["rating"]), "-")},
		{"Delivery methods", stringsJoin(asSlice(data["delivery_methods"]), ", ")},
		{"Order minimum", fallbackString(asString(asMap(data["order_minimum"])["formatted_amount"]), "-")},
	}
	optional := []string{"tags", "opening_windows", "rating_details", "delivery_fee"}
	for _, field := range optional {
		if value, ok := data[field]; ok {
			rows = append(rows, []string{field, fmt.Sprintf("%v", value)})
		}
	}
	return output.RenderTable("Venue: "+asString(data["name"]), headers, rows)
}

func resolveVenueItemPayloadBySlug(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	itemID string,
	auth woltgateway.AuthContext,
) (string, map[string]any, []string) {
	venueID := strings.TrimSpace(venueSlug)
	warnings := []string{}
	assortmentPayload := map[string]any{}
	venueContentPayloads := []map[string]any{}
	venueContentLoaded := false
	loadVenueContent := func() {
		if venueContentLoaded {
			return
		}
		venueContentLoaded = true
		payloads, fallbackWarnings := loadVenueContentPayloads(ctx, deps, venueSlug, auth, 2)
		venueContentPayloads = payloads
		warnings = append(warnings, fallbackWarnings...)
	}

	if payload, err := deps.Wolt.VenuePageStatic(ctx, venueSlug); err == nil {
		if resolvedID := venueIDFromPayload(payload); strings.TrimSpace(resolvedID) != "" {
			venueID = strings.TrimSpace(resolvedID)
		}
	} else {
		warnings = append(warnings, "venue static page endpoint unavailable")
	}
	if payload, err := deps.Wolt.AssortmentByVenueSlug(ctx, venueSlug); err == nil {
		assortmentPayload = payload
	} else {
		warnings = append(warnings, "venue assortment endpoint unavailable")
	}
	if needsVenueContentFallback(assortmentPayload, venueID) {
		loadVenueContent()
	}

	payload := map[string]any{}
	if venueID != "" {
		if itemPayload, err := deps.Wolt.VenueItemPage(ctx, venueID, itemID); err == nil {
			payload = itemPayload
			if fallback := buildItemPayloadFromAssortment(assortmentPayload, itemID); fallback != nil {
				payload = mergeItemPayloadFallback(payload, fallback)
			}
			if !payloadContainsItem(payload, venueID, itemID) {
				if fallback := buildItemPayloadFromMenuPayloads(venueContentPayloads, venueID, itemID); fallback != nil {
					payload = mergeItemPayloadFallback(payload, fallback)
					warnings = append(warnings, "item endpoint payload incomplete; used venue content fallback metadata")
				}
			}
		} else {
			warnings = append(warnings, "item endpoint unavailable")
			if fallback := buildItemPayloadFromAssortment(assortmentPayload, itemID); fallback != nil {
				payload = fallback
			}
			if !payloadContainsItem(payload, venueID, itemID) {
				if len(venueContentPayloads) == 0 {
					loadVenueContent()
				}
				if fallback := buildItemPayloadFromMenuPayloads(venueContentPayloads, venueID, itemID); fallback != nil {
					payload = mergeItemPayloadFallback(payload, fallback)
					warnings = append(warnings, "used venue content fallback metadata for item lookup")
				}
			}
		}
	}
	if len(payload) == 0 && len(venueContentPayloads) > 0 {
		payload = venueContentPayloads[0]
	}
	if len(payload) == 0 && len(assortmentPayload) > 0 {
		payload = assortmentPayload
	}
	if len(payload) == 0 {
		warnings = append(warnings, "item payload fallback unavailable")
	}
	return venueID, payload, dedupeStrings(warnings)
}

func itemOptionGroupIDsFromPayload(payload map[string]any, venueID string, itemID string) []string {
	targetItemID := strings.TrimSpace(itemID)
	if targetItemID == "" || payload == nil {
		return nil
	}
	for _, row := range observability.ExtractMenuItems(payload, venueID, "") {
		if strings.TrimSpace(asString(row["item_id"])) != targetItemID {
			continue
		}
		out := make([]string, 0, len(asSlice(row["option_group_ids"])))
		for _, value := range asSlice(row["option_group_ids"]) {
			id := strings.TrimSpace(asString(value))
			if id == "" {
				continue
			}
			out = append(out, id)
		}
		return dedupeStrings(out)
	}
	return nil
}

func payloadContainsItem(payload map[string]any, venueID string, itemID string) bool {
	targetItemID := strings.TrimSpace(itemID)
	if targetItemID == "" || payload == nil {
		return false
	}
	if candidate := strings.TrimSpace(asString(coalesceAny(payload["item_id"], payload["id"]))); strings.EqualFold(candidate, targetItemID) && hasItemSignals(payload) {
		return true
	}
	for _, row := range observability.ExtractMenuItems(payload, venueID, "") {
		if strings.EqualFold(strings.TrimSpace(asString(row["item_id"])), targetItemID) {
			return true
		}
	}
	for _, value := range asSlice(payload["items"]) {
		item := asMap(value)
		if item == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(asString(coalesceAny(item["item_id"], item["id"]))), targetItemID) {
			return true
		}
	}
	return false
}

func hasItemSignals(item map[string]any) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(asString(coalesceAny(item["name"], item["title"]))) != "" {
		return true
	}
	if asInt(item["price"]) > 0 || asInt(asMap(item["price"])["amount"]) > 0 || asInt(item["base_price"]) > 0 {
		return true
	}
	if len(asSlice(item["options"])) > 0 || len(asSlice(item["option_groups"])) > 0 || len(asSlice(item["option_group_ids"])) > 0 {
		return true
	}
	if description := strings.TrimSpace(asString(item["description"])); description != "" {
		return true
	}
	return false
}

func buildVenueItemSearchData(
	venueID string,
	venueSlug string,
	query string,
	category string,
	payload map[string]any,
	fallbackCurrency string,
	includeOptions bool,
	limit *int,
) (map[string]any, []string) {
	warnings := []string{}
	items := observability.ExtractMenuItems(payload, venueID, venueSlug)
	categoryFilter := strings.ToLower(strings.TrimSpace(category))
	if categoryFilter != "" {
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(asString(item["category"])), categoryFilter) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	total := len(items)
	if limit != nil && *limit > 0 && len(items) > *limit {
		items = items[:*limit]
	}
	if total == 0 {
		warnings = append(warnings, "no items matched this venue search query")
	}

	rows := make([]any, 0, len(items))
	for _, item := range items {
		basePrice := normalizeVenueSearchPrice(asMap(item["base_price"]), fallbackCurrency)
		originalPrice := normalizeVenueSearchPrice(asMap(item["original_price"]), fallbackCurrency)
		row := map[string]any{
			"item_id":     item["item_id"],
			"name":        item["name"],
			"category":    item["category"],
			"base_price":  basePrice,
			"discounts":   item["discounts"],
			"is_sold_out": item["is_sold_out"],
		}
		if hasAmountValue(originalPrice) {
			row["original_price"] = originalPrice
		}
		if includeOptions {
			row["option_group_ids"] = item["option_group_ids"]
		}
		rows = append(rows, row)
	}

	return map[string]any{
		"venue_id":   venueID,
		"venue_slug": venueSlug,
		"query":      query,
		"category":   emptyToNil(strings.TrimSpace(category)),
		"total":      total,
		"items":      rows,
	}, warnings
}

func buildVenueMenuTable(data map[string]any) string {
	headers := []string{"Item ID", "Name", "Price", "Discounts", "Option groups"}
	rows := [][]string{}
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		optionGroups := "-"
		if _, ok := item["option_group_ids"]; ok {
			optionGroups = stringsJoin(asSlice(item["option_group_ids"]), ", ")
			if optionGroups == "" {
				optionGroups = "-"
			}
		}
		discounts := stringsJoin(asSlice(item["discounts"]), ", ")
		if discounts == "" {
			discounts = "-"
		}
		rows = append(rows, []string{
			asString(item["item_id"]),
			asString(item["name"]),
			formatBasePriceForTable(asMap(item["base_price"])),
			discounts,
			optionGroups,
		})
	}
	title := "Venue menu: " + asString(data["venue_id"])
	if asBool(data["wolt_plus"]) {
		title += " (Wolt+)"
	}
	return output.RenderTable(title, headers, rows)
}

func buildVenueItemSearchTable(data map[string]any) string {
	headers := []string{"Item ID", "Name", "Category", "Price", "Sold out", "Discounts", "Option groups"}
	rows := make([][]string, 0, len(asSlice(data["items"])))
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		if item == nil {
			continue
		}
		optionGroups := "-"
		if _, ok := item["option_group_ids"]; ok {
			optionGroups = stringsJoin(asSlice(item["option_group_ids"]), ", ")
			if optionGroups == "" {
				optionGroups = "-"
			}
		}
		discounts := stringsJoin(asSlice(item["discounts"]), ", ")
		if discounts == "" {
			discounts = "-"
		}
		rows = append(rows, []string{
			fallbackString(asString(item["item_id"]), "-"),
			fallbackString(asString(item["name"]), "-"),
			fallbackString(asString(item["category"]), "-"),
			formatVenueSearchPriceForTable(asMap(item["base_price"]), asMap(item["original_price"])),
			boolToYesNo(asBool(item["is_sold_out"])),
			discounts,
			optionGroups,
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-", "-", "-"})
	}
	return output.RenderTable(
		fmt.Sprintf("Venue item search: %s (%s)", asString(data["venue_slug"]), asString(data["query"])),
		headers,
		rows,
	)
}

func formatBasePriceForTable(basePrice map[string]any) string {
	if basePrice == nil {
		return "-"
	}
	if formatted := strings.TrimSpace(asString(basePrice["formatted_amount"])); formatted != "" {
		return formatted
	}
	if _, ok := basePrice["amount"]; !ok || basePrice["amount"] == nil {
		return "-"
	}
	amount := asInt(basePrice["amount"])
	currency := strings.TrimSpace(asString(basePrice["currency"]))
	if currency == "" {
		return fmt.Sprintf("%.2f", float64(amount)/100)
	}
	return fmt.Sprintf("%s %.2f", currency, float64(amount)/100)
}

func formatVenueSearchPriceForTable(basePrice map[string]any, originalPrice map[string]any) string {
	base := formatBasePriceForTable(basePrice)
	if strings.TrimSpace(base) == "" || base == "-" {
		base = "-"
	}
	if originalPrice == nil || !hasAmountValue(originalPrice) {
		return base
	}
	original := formatBasePriceForTable(originalPrice)
	if strings.TrimSpace(original) == "" || original == "-" || original == base {
		return base
	}
	baseAmount := asInt(basePrice["amount"])
	originalAmount := asInt(originalPrice["amount"])
	if originalAmount <= 0 || baseAmount < 0 || originalAmount <= baseAmount {
		return base
	}
	return fmt.Sprintf("%s (was %s)", base, original)
}

func resolveVenueSearchFallbackCurrency(staticPayload map[string]any, searchPayload map[string]any) string {
	candidates := []any{
		asMap(staticPayload["venue"])["currency"],
		asMap(asMap(staticPayload["venue"])["price"])["currency"],
		asMap(staticPayload["venue_raw"])["currency"],
		asMap(asMap(staticPayload["venue_raw"])["price"])["currency"],
		staticPayload["currency"],
		staticPayload["currency_code"],
		asMap(searchPayload["venue"])["currency"],
		asMap(asMap(searchPayload["venue"])["price"])["currency"],
		searchPayload["currency"],
		searchPayload["currency_code"],
	}
	for _, candidate := range candidates {
		currency := strings.TrimSpace(asString(candidate))
		if currency != "" {
			return currency
		}
	}
	for _, rawItem := range asSlice(searchPayload["items"]) {
		item := asMap(rawItem)
		if item == nil {
			continue
		}
		for _, candidate := range []any{
			asMap(item["price"])["currency"],
			asMap(item["base_price"])["currency"],
			asMap(item["original_price"])["currency"],
		} {
			currency := strings.TrimSpace(asString(candidate))
			if currency != "" {
				return currency
			}
		}
	}
	return ""
}

func normalizeVenueSearchPrice(price map[string]any, fallbackCurrency string) map[string]any {
	normalized := map[string]any{
		"amount":           nil,
		"currency":         nil,
		"formatted_amount": nil,
	}
	for key, value := range price {
		normalized[key] = value
	}

	currency := strings.TrimSpace(asString(normalized["currency"]))
	if currency == "" {
		currency = strings.TrimSpace(fallbackCurrency)
	}
	if currency != "" {
		normalized["currency"] = currency
	}
	if !hasAmountValue(normalized) {
		return normalized
	}
	if strings.TrimSpace(asString(normalized["formatted_amount"])) == "" {
		amount := asInt(normalized["amount"])
		if currency != "" {
			normalized["formatted_amount"] = formatMinorAmount(amount, currency)
		} else {
			normalized["formatted_amount"] = fmt.Sprintf("%.2f", float64(amount)/100)
		}
	}
	return normalized
}

func hasAmountValue(price map[string]any) bool {
	if price == nil {
		return false
	}
	value, ok := price["amount"]
	if !ok {
		return false
	}
	return value != nil
}

func buildVenueCategoriesTable(data map[string]any) string {
	headers := []string{"Slug", "Name", "Parent", "Level", "Leaf", "Item refs"}
	rows := [][]string{}
	for _, value := range asSlice(data["categories"]) {
		category := asMap(value)
		if category == nil {
			continue
		}
		rows = append(rows, []string{
			fallbackString(asString(category["slug"]), "-"),
			fallbackString(asString(category["name"]), "-"),
			fallbackString(asString(category["parent_slug"]), "-"),
			asString(category["level"]),
			boolToYesNo(asBool(category["leaf"])),
			asString(category["item_refs_count"]),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-", "-"})
	}
	title := "Venue categories: " + asString(data["venue_id"])
	if strategy := strings.TrimSpace(asString(data["loading_strategy"])); strategy != "" {
		title += " (" + strategy + ")"
	}
	return output.RenderTable(title, headers, rows)
}

func buildVenueHoursTable(data map[string]any) string {
	headers := []string{"Day", "Open", "Close"}
	rows := [][]string{}
	for _, value := range asSlice(data["opening_windows"]) {
		window := asMap(value)
		rows = append(rows, []string{asString(window["day"]), asString(window["open"]), asString(window["close"])})
	}
	return output.RenderTable("Venue hours ("+asString(data["timezone"])+")", headers, rows)
}

func buildItemDetailTable(data map[string]any) string {
	optionGroups := asSlice(data["option_groups"])
	upsellItems := asSlice(data["upsell_items"])
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Item ID", asString(data["item_id"])},
		{"Venue ID", asString(data["venue_id"])},
		{"Description", fallbackString(asString(data["description"]), "-")},
		{"Price", fallbackString(asString(asMap(data["price"])["formatted_amount"]), "-")},
		{"Option groups", fmt.Sprintf("%d", len(optionGroups))},
		{"Upsell items", fmt.Sprintf("%d", len(upsellItems))},
	}
	sections := []string{
		output.RenderTable("Item: "+asString(data["name"]), headers, rows),
		output.RenderTable("Option groups", []string{"Group ID", "Name", "Required", "Min", "Max"}, buildItemGroupRows(optionGroups)),
	}
	if len(upsellItems) > 0 {
		sections = append(sections, output.RenderTable("Upsell items", []string{"Item ID", "Name", "Price"}, buildUpsellRows(upsellItems)))
	}
	return strings.Join(sections, "\n\n")
}

func buildItemGroupRows(optionGroups []any) [][]string {
	rows := make([][]string, 0, len(optionGroups))
	for _, optionGroup := range optionGroups {
		group := asMap(optionGroup)
		if group == nil {
			continue
		}
		required := "no"
		if asBool(group["required"]) {
			required = "yes"
		}
		rows = append(rows, []string{
			fallbackString(asString(group["group_id"]), "-"),
			fallbackString(asString(group["name"]), "-"),
			required,
			asString(group["min"]),
			asString(group["max"]),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-"})
	}
	return rows
}

func buildUpsellRows(upsellItems []any) [][]string {
	rows := make([][]string, 0, len(upsellItems))
	for _, upsellItem := range upsellItems {
		item := asMap(upsellItem)
		if item == nil {
			continue
		}
		rows = append(rows, []string{
			fallbackString(asString(item["item_id"]), "-"),
			fallbackString(asString(item["name"]), "-"),
			fallbackString(asString(asMap(item["price"])["formatted_amount"]), "-"),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-"})
	}
	return rows
}

func buildItemOptionsData(venueID string, itemID string, payload map[string]any, preferredGroupIDs []string) (map[string]any, []string) {
	warnings := []string{}
	optionSpecs := extractOptionSpecs(payload)
	currency := strings.TrimSpace(asString(asMap(payload["price"])["currency"]))
	if currency == "" {
		currency = "EUR"
	}
	groupIDs := make([]string, 0, len(optionSpecs))
	for _, preferred := range dedupeStrings(preferredGroupIDs) {
		preferred = strings.TrimSpace(preferred)
		if preferred == "" {
			continue
		}
		if _, ok := optionSpecs[preferred]; ok {
			groupIDs = append(groupIDs, preferred)
		}
	}
	if len(groupIDs) == 0 {
		for groupID := range optionSpecs {
			groupIDs = append(groupIDs, groupID)
		}
	} else if len(preferredGroupIDs) > 0 && len(groupIDs) < len(dedupeStrings(preferredGroupIDs)) {
		warnings = append(warnings, "some item option groups were missing in payload; showing resolved subset")
	}
	sort.Strings(groupIDs)

	optionGroups := make([]any, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		spec := optionSpecs[groupID]
		valueIDs := make([]string, 0, len(spec.Values))
		for valueID := range spec.Values {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)

		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			value := spec.Values[valueID]
			values = append(values, map[string]any{
				"value_id": valueID,
				"name":     emptyToNil(value.Name),
				"price": map[string]any{
					"amount":   value.Price,
					"currency": currency,
				},
				"example_option": fmt.Sprintf("%s=%s", groupID, valueID),
			})
		}

		optionGroups = append(optionGroups, map[string]any{
			"group_id": groupID,
			"name":     emptyToNil(spec.Name),
			"required": spec.Required || spec.MinSelect > 0,
			"min":      spec.MinSelect,
			"max":      spec.MaxSelect,
			"values":   values,
		})
	}
	if len(optionGroups) == 0 {
		warnings = append(warnings, "no option groups were discovered in the item payload")
	}

	return map[string]any{
		"venue_id":      venueID,
		"item_id":       itemID,
		"option_groups": optionGroups,
		"group_count":   len(optionGroups),
		"currency":      currency,
	}, warnings
}

func buildItemOptionsTable(data map[string]any) string {
	summary := output.RenderTable("Item option groups", []string{"Field", "Value"}, [][]string{
		{"Venue ID", fallbackString(asString(data["venue_id"]), "-")},
		{"Item ID", fallbackString(asString(data["item_id"]), "-")},
		{"Groups", asString(data["group_count"])},
		{"Currency", fallbackString(asString(data["currency"]), "-")},
	})

	headers := []string{"Group", "Value ID", "Value name", "Price", "Example --option"}
	rows := [][]string{}
	for _, groupValue := range asSlice(data["option_groups"]) {
		group := asMap(groupValue)
		if group == nil {
			continue
		}
		groupLabel := fallbackString(asString(group["group_id"]), "-")
		if groupName := asString(group["name"]); groupName != "" {
			groupLabel = fmt.Sprintf("%s (%s)", groupLabel, groupName)
		}
		for _, valueNode := range asSlice(group["values"]) {
			valueMap := asMap(valueNode)
			if valueMap == nil {
				continue
			}
			currency := asString(asMap(valueMap["price"])["currency"])
			rows = append(rows, []string{
				groupLabel,
				fallbackString(asString(valueMap["value_id"]), "-"),
				fallbackString(asString(valueMap["name"]), "-"),
				fallbackString(formatMinorAmount(asInt(asMap(valueMap["price"])["amount"]), currency), "-"),
				"--option " + asString(valueMap["example_option"]),
			})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-"})
	}
	return summary + "\n\n" + output.RenderTable("Selectable values", headers, rows)
}

func fallbackString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func stringsJoin(values []any, separator string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, asString(value))
	}
	return join(parts, separator)
}

func join(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}
