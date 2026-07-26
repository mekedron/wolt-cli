package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/catalogload"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newVenueShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var include string

	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Show venue details by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := normalizeVenueInput(args[0])
			if reference == "" {
				return fmt.Errorf("venue identifier is required")
			}
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
			fallback := venueIdentityFromInput(args[0])
			staticPayload, staticErr := cachedVenuePageStatic(cmd.Context(), deps, reference)
			identity := observability.ExtractVenueIdentity(fallback, staticPayload)
			dynamicReference := fallbackString(identity.Slug, reference)
			dynamicPayload, dynamicErr := loadVenuePageDynamic(
				cmd.Context(),
				deps,
				dynamicReference,
				woltgateway.VenuePageDynamicOptions{
					Location:               &location,
					SelectedDeliveryMethod: "homedelivery",
					Auth:                   locationAuth,
				},
			)
			if staticPayload == nil && dynamicPayload == nil {
				sourceErr := dynamicErr
				if sourceErr == nil {
					sourceErr = staticErr
				}
				if sourceErr == nil {
					sourceErr = fmt.Errorf("venue payload is unavailable")
				}
				return emitUpstreamError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					flags.Verbose,
					sourceErr,
				)
			}
			data, warnings, buildErr := observability.BuildVenueDetailFromPayload(
				fallback,
				staticPayload,
				dynamicPayload,
				nil,
				&location,
				splitCSV(include),
			)
			if buildErr != nil {
				return buildErr
			}
			if staticErr != nil {
				warnings = append(warnings, "venue static page endpoint unavailable")
			}
			if dynamicErr != nil {
				warnings = append(warnings, "location-aware venue availability could not be loaded")
			}
			warnings = dedupeStrings(warnings)

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
	var limit int
	var limitSet bool
	var offset int
	var offsetSet bool
	var page int
	var pageSet bool

	cmd := &cobra.Command{
		Use:   "categories <slug>",
		Short: "List available venue menu categories by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueRef, err := resolveVenueReference(cmd.Context(), deps, args[0])
			if err != nil {
				return err
			}
			slug := fallbackString(venueRef.VenueSlug, normalizeVenueInput(args[0]))
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}

			venueID := strings.TrimSpace(venueRef.VenueID)
			staticWarnings := []string{}
			if payload, err := cachedVenuePageStatic(cmd.Context(), deps, slug); err == nil {
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

			identity := observability.ExtractVenueIdentity(
				observability.VenueIdentity{ID: venueID, Slug: slug},
				assortmentPayload,
			)
			data := buildVenueCategoriesData(identity.ID, assortmentPayload)
			resolvedOffset, err := resolvePageOffset(limit, limitSet, offset, offsetSet, page, pageSet)
			if err != nil {
				return err
			}
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			paginateFlatRows(data, "categories", limitPtr, resolvedOffset)
			if pageSet {
				data["page"] = page
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueCategoriesTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, staticWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned categories")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset returned categories")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number (requires --limit; cannot be combined with --offset)")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		pageSet = cmd.Flags().Changed("page")
	}
	return cmd
}

func buildVenueCategoriesData(venueID string, assortmentPayload map[string]any) map[string]any {
	return map[string]any{
		"venue_id":         venueID,
		"loading_strategy": strings.TrimSpace(asString(assortmentPayload["loading_strategy"])),
		"categories":       venueCategoryRows(catalogload.Categories(assortmentPayload)),
	}
}

func venueCategoryRows(categories []catalogload.Category) []map[string]any {
	rows := make([]map[string]any, 0, len(categories))
	for _, category := range categories {
		rows = append(rows, map[string]any{
			"id":              category.ID,
			"slug":            category.Slug,
			"name":            category.Name,
			"parent_slug":     emptyToNil(category.ParentSlug),
			"level":           category.Level,
			"leaf":            category.Leaf,
			"item_refs_count": category.ItemRefsCount,
		})
	}
	return rows
}

func newVenueMenuCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var category string
	var query string
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
			"For large marketplace assortments, narrow the menu with `--query <text>` " +
			"or use category-first mode (`--category <slug>`).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueRef, err := resolveVenueReference(cmd.Context(), deps, args[0])
			if err != nil {
				return err
			}
			slug := fallbackString(venueRef.VenueSlug, normalizeVenueInput(args[0]))
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
			rowOptions := venueMenuRowOptions{
				includeOptions: includeOptions,
				filters: itemRowFilters{
					MinPriceSet:   minPriceSet,
					MinPrice:      minPrice,
					MaxPriceSet:   maxPriceSet,
					MaxPrice:      maxPrice,
					HideSoldOut:   hideSoldOut,
					DiscountsOnly: discountsOnly,
				},
				sort:    sortMode,
				limit:   limitPtr,
				offset:  resolvedOffset,
				page:    page,
				pageSet: pageSet,
			}
			venueID := strings.TrimSpace(venueRef.VenueID)
			metadataPayloads := []map[string]any{}
			itemPayloads := []map[string]any{}
			warnings := []string{}
			assortmentPayload := map[string]any{}
			if payload, err := cachedVenuePageStatic(cmd.Context(), deps, slug); err == nil {
				metadataPayloads = append(metadataPayloads, payload)
				if resolvedID := venueIDFromPayload(payload); strings.TrimSpace(resolvedID) != "" {
					venueID = strings.TrimSpace(resolvedID)
				}
			} else {
				warnings = append(warnings, "venue static page endpoint unavailable")
			}
			if strings.TrimSpace(query) != "" {
				if strings.TrimSpace(category) != "" {
					if payload, err := deps.Wolt.AssortmentByVenueSlug(cmd.Context(), slug); err == nil {
						metadataPayloads = append(metadataPayloads, payload)
					} else {
						warnings = append(
							warnings,
							"venue assortment metadata is unavailable; category slug matching may be incomplete",
						)
					}
				}
				searchPayload, err := requestAssortmentItemsSearchPayload(
					cmd.Context(),
					deps,
					slug,
					strings.TrimSpace(query),
					domain.ResolveAssortmentLanguage(flags.Locale),
					auth,
				)
				if err != nil {
					return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
				}
				identityPayloads := append(append([]map[string]any{}, metadataPayloads...), searchPayload)
				identity := observability.ExtractVenueIdentity(
					observability.VenueIdentity{
						ID:           venueID,
						Slug:         slug,
						CanonicalURL: domain.CanonicalVenueURL(args[0], slug),
					},
					identityPayloads...,
				)
				data, searchWarnings := observability.BuildItemSearchResult(
					query,
					[]map[string]any{searchPayload},
					observability.ItemSortRelevance,
					category,
					nil,
					0,
					nil,
					observability.ItemVenueContext{
						VenueID:               identity.ID,
						VenueSlug:             identity.Slug,
						CanonicalURL:          identity.CanonicalURL,
						IncludeOptionGroupIDs: includeOptions,
						MetadataPayloads:      metadataPayloads,
					},
				)
				applyVenueMenuRowOptions(data, rowOptions)
				warnings = append(warnings, searchWarnings...)
				if format == output.FormatTable {
					return writeTable(cmd, buildVenueItemSearchTable(data), flags.Output)
				}
				env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
				return writeMachinePayload(cmd, env, format, flags.Output)
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
			if payload, err := loadVenuePageDynamic(cmd.Context(), deps, slug, dynamicOptions); err == nil {
				metadataPayloads = append(metadataPayloads, payload)
			} else {
				warnings = append(warnings, "venue dynamic page endpoint unavailable")
			}
			if payload, err := deps.Wolt.AssortmentByVenueSlug(cmd.Context(), slug); err == nil {
				assortmentPayload = payload
				itemPayloads = append(itemPayloads, payload)
				metadataPayloads = append(metadataPayloads, payload)
			} else {
				warnings = append(warnings, "venue assortment endpoint unavailable")
			}
			categorySlug := strings.TrimSpace(category)
			categoryFilter := categorySlug
			assortmentPartial, _ := venueAssortmentState(assortmentPayload, venueID)
			switch {
			case categorySlug != "":
				categoryLoad, err := loadAssortmentCategory(
					cmd.Context(),
					deps,
					slug,
					categorySlug,
					domain.ResolveAssortmentLanguage(flags.Locale),
					auth,
				)
				if err != nil {
					return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
				}
				itemPayloads = []map[string]any{categoryLoad.Payload}
				metadataPayloads = append(metadataPayloads, categoryLoad.Payload)
				warnings = append(warnings, categoryLoad.Warnings...)
				categoryFilter = ""
			case assortmentPartial && !fullCatalog:
				return emitError(
					cmd,
					format,
					profile.Name,
					flags.Locale,
					flags.Output,
					"WOLT_INVALID_ARGUMENT",
					fmt.Sprintf(
						"venue assortment is partial for %q; pass --category <slug> (list with \"wolt venue categories %s\"), or use \"wolt venue menu %s --query <text>\"",
						slug,
						slug,
						slug,
					),
				)
			case needsVenueContentFallback(assortmentPayload, venueID):
				contentFallbackNeeded := true
				if assortmentPartial && fullCatalog {
					warnings = append(warnings, "full catalog mode enabled for partial assortment; loading all categories (this may be slow)")
					categoryPayloads, categoryWarnings, categoriesComplete, categoryErr := loadAssortmentCategoryPayloads(
						cmd.Context(),
						deps,
						slug,
						domain.ResolveAssortmentLanguage(flags.Locale),
						auth,
						assortmentPayload,
					)
					if categoryErr != nil {
						return emitUpstreamError(
							cmd,
							format,
							profile.Name,
							flags.Locale,
							flags.Output,
							flags.Verbose,
							categoryErr,
						)
					}
					if len(categoryPayloads) > 0 {
						// Hydrated category responses are authoritative for a
						// partial catalog. Root rows are often stale previews.
						itemPayloads = append([]map[string]any(nil), categoryPayloads...)
						contentFallbackNeeded = !categoriesComplete
					}
					metadataPayloads = append(metadataPayloads, categoryPayloads...)
					warnings = append(warnings, categoryWarnings...)
				}
				if contentFallbackNeeded {
					venueContentPayloads, fallbackWarnings := loadVenueContentPayloads(cmd.Context(), deps, slug, auth, 2)
					itemPayloads = append(itemPayloads, venueContentPayloads...)
					metadataPayloads = append(metadataPayloads, venueContentPayloads...)
					warnings = append(warnings, fallbackWarnings...)
				}
			}

			identity := observability.ExtractVenueIdentity(
				observability.VenueIdentity{
					ID:           venueID,
					Slug:         slug,
					CanonicalURL: domain.CanonicalVenueURL(args[0], slug),
				},
				metadataPayloads...,
			)
			data, menuWarnings := observability.BuildVenueMenu(
				identity.ID,
				itemPayloads,
				categoryFilter,
				includeOptions,
				nil,
				observability.ItemVenueContext{
					VenueID:          identity.ID,
					VenueSlug:        identity.Slug,
					CanonicalURL:     identity.CanonicalURL,
					MetadataPayloads: metadataPayloads,
				},
			)
			if strings.TrimSpace(asString(data["venue_slug"])) == "" {
				data["venue_slug"] = slug
			}
			applyVenueMenuRowOptions(data, rowOptions)
			warnings = append(warnings, menuWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueMenuTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search menu items by name")
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

type venueMenuRowOptions struct {
	includeOptions bool
	filters        itemRowFilters
	sort           itemRowSort
	limit          *int
	offset         int
	page           int
	pageSet        bool
}

func applyVenueMenuRowOptions(data map[string]any, options venueMenuRowOptions) {
	if !options.includeOptions {
		for _, rawItem := range asSlice(data["items"]) {
			delete(asMap(rawItem), "option_group_ids")
		}
	}
	data["items"] = applyItemRowFilters(asSlice(data["items"]), options.filters)
	sortItemRows(asSlice(data["items"]), options.sort)
	data["sort"] = string(options.sort)
	paginateFlatRows(data, "items", options.limit, options.offset)
	if options.pageSet {
		data["page"] = options.page
	}
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

func newVenueHoursCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var timezone string

	cmd := &cobra.Command{
		Use:   "hours <venue>",
		Short: "Show venue opening hours by slug, ID, or Wolt URL.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := normalizeVenueInput(args[0])
			if reference == "" {
				return fmt.Errorf("venue identifier is required")
			}
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			staticPayload, err := cachedVenuePageStatic(cmd.Context(), deps, reference)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if staticPayload == nil {
				return emitUpstreamError(
					cmd,
					format,
					profile.Name,
					flags.Locale,
					flags.Output,
					flags.Verbose,
					fmt.Errorf("venue hours payload is unavailable"),
				)
			}
			data, warnings, buildErr := observability.BuildVenueHoursFromPayload(
				venueIdentityFromInput(args[0]),
				staticPayload,
				timezone,
			)
			if buildErr != nil {
				return buildErr
			}
			if format == output.FormatTable {
				return writeTable(cmd, buildVenueHoursTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&timezone, "timezone", "", "Expected venue timezone or fallback label; does not convert weekly hours.")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newItemShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var includeUpsell bool

	cmd := &cobra.Command{
		Use:   "show <venue-slug> <item-id>",
		Short: "Show item details by venue slug and item ID.",
		Long: "Show item details by venue slug and item ID.\n\n" +
			"<item-id> accepts a 24-char Mongo ObjectID or a Wolt item URL\n" +
			"(.../venue/<slug>/itemid-<id>). When the single positional arg is\n" +
			"a Wolt item URL, the venue slug is read from the URL.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueArg := args[0]
			itemArg := ""
			if len(args) >= 2 {
				itemArg = args[1]
			}
			// One-arg URL form: only positional value is a Wolt item URL with
			// both the slug and the item id baked in.
			if itemArg == "" {
				if probe := resolveItemReference(venueArg); probe.ItemID != "" && probe.VenueSlugHint != "" {
					itemArg = venueArg
					venueArg = probe.VenueSlugHint
				} else {
					return fmt.Errorf("item id is required; pass it as the second argument or use a Wolt item URL that contains both the venue slug and the item id")
				}
			}

			venueRef, err := resolveVenueReference(cmd.Context(), deps, venueArg)
			if err != nil {
				return err
			}
			venueSlug := fallbackString(venueRef.VenueSlug, normalizeVenueInput(venueArg))
			itemRef := resolveItemReference(itemArg)
			itemID := itemRef.ItemID
			if itemID == "" {
				trimmed := strings.TrimSpace(itemArg)
				if looksURLShaped(trimmed) {
					return fmt.Errorf("could not extract an item id from %q; expected a Wolt item URL like .../venue/<slug>/itemid-<id>", trimmed)
				}
				itemID = trimmed
			}
			if strings.TrimSpace(venueSlug) == "" && itemRef.VenueSlugHint != "" {
				venueSlug = itemRef.VenueSlugHint
			}
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)

			venueID, payload, availabilityVerified, warnings := resolveVenueItemPayloadBySlug(
				cmd.Context(),
				deps,
				venueRef.VenueID,
				venueSlug,
				itemID,
				auth,
			)
			if !payloadContainsItem(payload, venueID, itemID) {
				return fmt.Errorf(
					"item %q was not found for venue slug %q; run \"wolt venue menu %s --include-options\" to list valid item IDs",
					itemID,
					venueSlug,
					venueSlug,
				)
			}
			identity := observability.ExtractVenueIdentity(
				observability.VenueIdentity{
					ID:   venueID,
					Slug: venueSlug,
					CanonicalURL: fallbackString(
						domain.CanonicalVenueURL(venueArg, venueSlug),
						domain.CanonicalVenueURL(itemArg, venueSlug),
					),
				},
				payload,
			)
			data, itemWarnings := observability.BuildItemDetail(
				itemID,
				venueID,
				payload,
				includeUpsell,
				observability.ItemVenueContext{
					VenueID:              identity.ID,
					VenueSlug:            identity.Slug,
					CanonicalURL:         identity.CanonicalURL,
					AvailabilityVerified: &availabilityVerified,
				},
			)
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
	verifiedVenueID string,
	venueSlug string,
	itemID string,
	auth woltgateway.AuthContext,
) (string, map[string]any, bool, []string) {
	venueID := strings.TrimSpace(verifiedVenueID)
	if !domain.IsObjectID(venueID) {
		venueID = ""
	}
	warnings := []string{}
	assortmentPayload := map[string]any{}
	venueContentPayloads := []map[string]any{}
	venueContentLoaded := false
	currentItem := map[string]any{}
	currentLookupSucceeded := false
	loadVenueContent := func() {
		if venueContentLoaded {
			return
		}
		venueContentLoaded = true
		payloads, fallbackWarnings := loadVenueContentPayloads(ctx, deps, venueSlug, auth, 2)
		venueContentPayloads = payloads
		warnings = append(warnings, fallbackWarnings...)
	}

	if payload, err := cachedVenuePageStatic(ctx, deps, venueSlug); err == nil {
		if resolvedID := venueIDFromPayload(payload); domain.IsObjectID(resolvedID) {
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
	if payload, err := requestAssortmentItemsPayload(ctx, deps, venueSlug, []string{itemID}, auth); err == nil {
		currentLookupSucceeded = true
		currentItem = catalogitem.ScopedItem(payload, itemID)
	} else {
		warnings = append(warnings, "current item endpoint unavailable")
	}
	if needsVenueContentFallback(assortmentPayload, venueID) {
		loadVenueContent()
	}

	payload := map[string]any{}
	if domain.IsObjectID(venueID) {
		if itemPayload, err := deps.Wolt.VenueItemPage(ctx, venueID, itemID); err == nil {
			payload = itemPayload
			if len(currentItem) > 0 {
				payload = catalogitem.MergeCurrentItem(payload, currentItem)
			}
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
			if len(currentItem) > 0 {
				payload = catalogitem.MergeCurrentItem(payload, currentItem)
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
	if scoped := catalogitem.ScopedItem(payload, itemID); scoped != nil {
		payload = scoped
	}
	if len(currentItem) > 0 {
		payload = catalogitem.MergeCurrentItem(payload, currentItem)
	}
	if currentLookupSucceeded && len(currentItem) == 0 {
		payload = catalogitem.MarkMissingFromCurrentAssortment(payload, itemID)
		warnings = append(warnings, "item is missing from the current assortment")
	}
	if len(payload) > 0 {
		if currency := currencyFromVenue(ctx, deps, venueSlug); currency != "" {
			payload["currency"] = currency
		}
	}
	if len(payload) == 0 {
		warnings = append(warnings, "item payload fallback unavailable")
	}
	return venueID, payload, currentLookupSucceeded, dedupeStrings(warnings)
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

func buildVenueMenuTable(data map[string]any) string {
	headers := []string{"Item ID", "Name", "Price", "Discounts"}
	rows := [][]string{}
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		discounts := stringsJoin(asSlice(item["discounts"]), ", ")
		if discounts == "" {
			discounts = "-"
		}
		rows = append(rows, []string{
			asString(item["item_id"]),
			asString(item["name"]),
			formatBasePriceForTable(asMap(item["base_price"])),
			discounts,
		})
	}
	title := "Venue menu: " + fallbackString(asString(data["venue_slug"]), asString(data["venue_id"]))
	if query := strings.TrimSpace(asString(data["query"])); query != "" {
		title += " (" + query + ")"
	}
	if asBool(data["wolt_plus"]) {
		title += " (Wolt+)"
	}
	return output.RenderTable(title, headers, rows)
}

func buildVenueItemSearchTable(data map[string]any) string {
	headers := []string{"Item ID", "Name", "Category", "Price", "Sold out", "Discounts"}
	rows := make([][]string, 0, len(asSlice(data["items"])))
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		if item == nil {
			continue
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
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-", "-"})
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
	timezone := fallbackString(asString(data["timezone"]), "unknown timezone")
	return output.RenderTable("Venue hours ("+timezone+")", headers, rows)
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
		output.RenderTable("Option groups", []string{"Group ID", "Name", "Required", "Min", "Max", "Values"}, buildItemGroupRows(optionGroups)),
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
		values := []string{}
		for _, rawValue := range asSlice(group["values"]) {
			value := asMap(rawValue)
			if value == nil {
				continue
			}
			valueID := strings.TrimSpace(asString(coalesceAny(
				value["value_id"],
				value["id"],
			)))
			name := strings.TrimSpace(asString(value["name"]))
			if name != "" && name != valueID {
				valueID += " (" + name + ")"
			}
			if valueID != "" {
				values = append(values, valueID)
			}
		}
		rows = append(rows, []string{
			fallbackString(asString(coalesceAny(group["group_id"], group["id"])), "-"),
			fallbackString(asString(group["name"]), "-"),
			required,
			asString(group["min"]),
			asString(group["max"]),
			fallbackString(strings.Join(values, ", "), "-"),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-", "-"})
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
