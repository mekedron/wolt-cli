package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
	"github.com/spf13/cobra"
)

func newCartCommand(deps Dependencies) *cobra.Command {
	cart := newCartShowCommand(deps)
	cart.Use = "cart"
	cart.Short = "Show and update cart contents."
	cart.AddCommand(newCartAddCommand(deps))
	cart.AddCommand(newCartRemoveCommand(deps))
	cart.AddCommand(newCartClearCommand(deps))
	cart.AddCommand(newCartCountCommand(deps))
	return cart
}

func newCartShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var venueID string
	var details bool
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show basket items and totals.",
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
			data, warnings := buildCartState(page, venueID)
			warnings = append(warnings, authWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildCartTable(data, details), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&venueID, "venue-id", "", "Restrict output to one venue basket.")
	cmd.Flags().BoolVar(&details, "details", false, "Include selected option/value details for each cart line in table output.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for cart endpoints. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for cart endpoints. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func newCartAddCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var count int
	var optionFlags []string
	var allowSubstitutions bool
	var nameOverride string
	var priceOverride int
	var currencyOverride string
	var venueSlug string
	var nameQuery string
	var cheapest bool
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "add <venue> [<item-id>]",
		Short: "Add an item to basket.",
		Long: "Add an item to basket.\n\n" +
			"<item-id> accepts a 24-char Mongo ObjectID or a Wolt item URL\n" +
			"(.../venue/<slug>/itemid-<id>). Omit <item-id> and pass\n" +
			"--query \"<name>\" to look the item up by name in the venue menu\n" +
			"(must match exactly one item). Add --cheapest to take the cheapest\n" +
			"in-stock match instead of erroring on ambiguity, or use --cheapest\n" +
			"alone to add the venue's cheapest in-stock item. When the only\n" +
			"positional arg is a Wolt item URL, the venue slug is read from the\n" +
			"URL — no separate <venue> argument needed.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			if count <= 0 {
				return fmt.Errorf("%s", requiredArg("--count must be greater than 0"))
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}

			venueArg := args[0]
			rawItem := ""
			if len(args) >= 2 {
				rawItem = args[1]
			}
			// One-arg URL form: when the only positional value is a Wolt item
			// URL that yields both an item id and a venue slug hint, treat it
			// as `cart add <slug-from-url> <item-url>`.
			if rawItem == "" && looksURLShaped(strings.TrimSpace(venueArg)) {
				if probe := resolveItemReference(venueArg); probe.ItemID != "" && probe.VenueSlugHint != "" {
					rawItem = venueArg
					venueArg = probe.VenueSlugHint
				}
			}

			venueID := strings.TrimSpace(venueArg)
			if resolved, resolveErr := resolveVenueReference(cmd.Context(), deps, venueID); resolveErr == nil {
				venueID = resolved.VenueID
				if strings.TrimSpace(venueSlug) == "" {
					venueSlug = resolved.VenueSlug
				}
			}

			itemRef := resolveItemReference(rawItem)
			itemID := itemRef.ItemID
			trimmedItem := strings.TrimSpace(rawItem)
			if itemID == "" && trimmedItem != "" {
				if looksURLShaped(trimmedItem) {
					return fmt.Errorf("could not extract an item id from %q; expected a Wolt item URL like .../venue/<slug>/itemid-<id>", trimmedItem)
				}
				// Bare non-ObjectID strings are forwarded to upstream unchanged so we
				// stay compatible with internal identifiers and let Wolt reject if
				// they are genuinely invalid.
				itemID = trimmedItem
			}
			if strings.TrimSpace(venueSlug) == "" && itemRef.VenueSlugHint != "" {
				venueSlug = itemRef.VenueSlugHint
			}
			if itemID == "" {
				slugForLookup := strings.TrimSpace(venueSlug)
				if slugForLookup == "" {
					slugForLookup = strings.TrimSpace(venueArg)
				}
				switch {
				case cheapest:
					if slugForLookup == "" {
						return fmt.Errorf("--cheapest requires a venue slug to search; pass the venue slug (or URL) as the first argument")
					}
					match, lookupErr := resolveCheapestItem(cmd.Context(), deps, slugForLookup, venueID, nameQuery)
					if lookupErr != nil {
						return lookupErr
					}
					itemID = match.ID
					if strings.TrimSpace(nameOverride) == "" {
						nameOverride = match.Name
					}
					// Carry the resolved price through so the add succeeds even if
					// the per-item page lookup below comes back without one.
					if priceOverride <= 0 {
						priceOverride = match.Price
					}
				case strings.TrimSpace(nameQuery) != "":
					if slugForLookup == "" {
						return fmt.Errorf("--query requires a venue slug to search; pass the venue slug (or URL) as the first argument")
					}
					match, lookupErr := resolveItemByName(
						cmd.Context(),
						deps,
						slugForLookup,
						venueID,
						nameQuery,
						domain.ResolveAssortmentLanguage(flags.Locale),
						auth,
					)
					if lookupErr != nil {
						return lookupErr
					}
					itemID = match.ID
					if strings.TrimSpace(nameOverride) == "" {
						nameOverride = match.Name
					}
				default:
					return fmt.Errorf("either <item-id> or --query is required")
				}
			}
			if venueID == "" || itemID == "" {
				return fmt.Errorf("venue and item-id are required")
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

			warnings := []string{}
			slugCandidates := []string{}
			if overrideSlug := strings.TrimSpace(venueSlug); overrideSlug != "" {
				slugCandidates = append(slugCandidates, overrideSlug)
			}
			if ref := strings.TrimSpace(venueArg); ref != "" && !looksLikeObjectID(ref) {
				slugCandidates = append(slugCandidates, normalizeVenueInput(ref))
			}
			slugCandidates = dedupeStrings(slugCandidates)

			// The id→slug lookup is an extra upstream round-trip on the path of
			// every add, so it is consulted lazily: only when no slug was given
			// explicitly, or when the explicit ones did not resolve the item.
			venuePageSlugTried := false
			appendVenuePageSlug := func() bool {
				if venuePageSlugTried {
					return false
				}
				venuePageSlugTried = true
				pageSlug := resolveVenueSlugFromID(cmd.Context(), deps, venueID)
				if pageSlug == "" {
					return false
				}
				extended := dedupeStrings(append(slugCandidates, pageSlug))
				if len(extended) == len(slugCandidates) {
					return false
				}
				slugCandidates = extended
				return true
			}
			if len(slugCandidates) == 0 {
				appendVenuePageSlug()
			}
			if len(slugCandidates) == 0 {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_ITEM_AVAILABILITY_UNKNOWN",
					"Unable to resolve the venue slug, so current item availability could not be verified. Pass the venue slug or a Wolt venue/item URL.",
				)
			}

			currentAssortmentPayload := map[string]any{}
			availabilityLookupSucceeded := false
			for index := 0; index < len(slugCandidates); index++ {
				candidateSlug := slugCandidates[index]
				payload, availabilityWarnings, availabilityErr := invokeWithAuthAutoRefresh(
					cmd.Context(),
					deps,
					flags,
					&auth,
					func(authCtx woltgateway.AuthContext) (map[string]any, error) {
						return requestAssortmentItemsPayload(
							cmd.Context(),
							deps,
							candidateSlug,
							[]string{itemID},
							authCtx,
						)
					},
				)
				warnings = append(warnings, availabilityWarnings...)
				if availabilityErr != nil {
					warnings = append(warnings, fmt.Sprintf("availability lookup failed for venue slug %s", candidateSlug))
				} else {
					availabilityLookupSucceeded = true
					if catalogitem.Find(payload, itemID) != nil {
						currentAssortmentPayload = payload
						venueSlug = candidateSlug
						break
					}
				}
				// Explicit candidates are exhausted without a hit; fall back to
				// the slug Wolt reports for this venue id before giving up.
				if index == len(slugCandidates)-1 {
					appendVenuePageSlug()
				}
			}
			if !availabilityLookupSucceeded {
				return emitErrorWithWarnings(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_ITEM_AVAILABILITY_UNKNOWN",
					"Item was not added because current availability could not be verified.",
					warnings,
				)
			}
			availabilityIssues := catalogitem.ValidateItemIDs(currentAssortmentPayload, []string{itemID})
			if len(availabilityIssues) > 0 {
				return emitErrorWithWarnings(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_ITEM_UNAVAILABLE",
					"Item was not added because current availability validation failed: "+
						catalogitem.FormatValidationIssues(availabilityIssues),
					warnings,
				)
			}

			itemPayload := map[string]any{}
			if payload, itemErr := deps.Wolt.VenueItemPage(cmd.Context(), venueID, itemID); itemErr == nil {
				itemPayload = payload
			} else {
				warnings = append(warnings, "item endpoint unavailable")
			}
			if fallback := buildItemPayloadFromAssortment(currentAssortmentPayload, itemID); fallback != nil {
				itemPayload = mergeItemPayloadFallback(itemPayload, fallback)
			}
			if needsAssortmentFallback(itemPayload) {
				for _, candidateSlug := range dedupeStrings(slugCandidates) {
					assortmentPayload := map[string]any{}
					if payload, err := deps.Wolt.AssortmentByVenueSlug(cmd.Context(), candidateSlug); err == nil {
						assortmentPayload = payload
					}
					if fallback := buildItemPayloadFromAssortment(assortmentPayload, itemID); fallback != nil {
						itemPayload = mergeItemPayloadFallback(itemPayload, fallback)
						break
					}
					if !needsVenueContentFallback(assortmentPayload, venueID) {
						continue
					}
					venueContentPayloads, fallbackWarnings := loadVenueContentPayloads(cmd.Context(), deps, candidateSlug, auth, 2)
					warnings = append(warnings, fallbackWarnings...)
					if fallback := buildItemPayloadFromMenuPayloads(venueContentPayloads, venueID, itemID); fallback != nil {
						itemPayload = mergeItemPayloadFallback(itemPayload, fallback)
						warnings = append(warnings, "used venue content fallback metadata for cart item")
						break
					}
				}
			}

			name := strings.TrimSpace(nameOverride)
			if name == "" {
				name = strings.TrimSpace(asString(itemPayload["name"]))
			}
			if name == "" {
				name = itemID
			}

			price := priceOverride
			if price <= 0 {
				price = asInt(asMap(itemPayload["price"])["amount"])
			}
			if price <= 0 {
				price = asInt(itemPayload["price"])
			}
			if price <= 0 {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_INVALID_ARGUMENT",
					"Unable to infer item price. Provide --price in minor units.",
				)
			}

			currency := payloadutil.NormalizeCurrency(currencyOverride)
			if currency == "" {
				currency = currencyFromItemPayload(itemPayload)
			}
			if currency == "" {
				currency = payloadutil.CurrencyFromVenuePayload(currentAssortmentPayload)
			}
			if currency == "" {
				currency = currencyFromVenue(cmd.Context(), deps, venueSlug)
			}

			selectedOptions, err := parseOptionSelections(optionFlags)
			if err != nil {
				return err
			}
			if len(selectedOptions) > 0 && len(extractOptionSpecs(itemPayload)) == 0 {
				warnings = append(warnings, "option metadata unavailable; provide option IDs or use --venue-slug to resolve option names")
			}
			options := buildBasketOptions(itemPayload, selectedOptions)
			newLineItem := map[string]any{
				"id":      itemID,
				"count":   count,
				"name":    name,
				"price":   price,
				"options": options,
				"substitution_settings": map[string]any{
					"is_allowed": allowSubstitutions,
				},
			}

			mergedItems := []any{newLineItem}
			venueMutationID := venueID
			existingPage, preAddAuthWarnings, preAddErr := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketsPage(cmd.Context(), location, authCtx)
				},
			)
			warnings = append(warnings, preAddAuthWarnings...)
			if preAddErr == nil {
				selectedBasket, _, _ := selectBasketWithMeta(existingPage, venueID)
				if selectedBasket != nil {
					if currency == "" {
						currency = payloadutil.CurrencyFromBasket(selectedBasket)
					}
					resolvedVenue := asMap(selectedBasket["venue"])
					if resolvedVenueID := strings.TrimSpace(asString(resolvedVenue["id"])); resolvedVenueID != "" {
						venueMutationID = resolvedVenueID
					}
					existingItems := asSlice(selectedBasket["items"])
					if len(existingItems) > 0 {
						mergedItems = make([]any, 0, len(existingItems)+1)
						mergedCurrentLine := false
						for _, rawValue := range existingItems {
							line := asMap(rawValue)
							if line == nil {
								continue
							}
							lineID := strings.TrimSpace(asString(line["id"]))
							lineCount := asInt(line["count"])
							if lineCount <= 0 {
								lineCount = 1
							}
							if lineID != "" && strings.EqualFold(lineID, itemID) {
								mergedItems = append(mergedItems, buildBasketUpsertItem(line, lineCount+count))
								mergedCurrentLine = true
								continue
							}
							mergedItems = append(mergedItems, buildBasketUpsertItem(line, lineCount))
						}
						if !mergedCurrentLine {
							mergedItems = append(mergedItems, newLineItem)
						}
					}
				}
			} else {
				warnings = append(warnings, "unable to load existing basket snapshot before add; upstream may replace existing lines")
			}
			if currency == "" {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_CURRENCY_UNKNOWN",
					"Item was not added because the venue currency could not be verified.",
				)
			}

			// Refuse to POST when the venue still hasn't resolved to a real
			// Wolt venue id (a 24-char ObjectID). Posting a slug as venue_id
			// makes the Wolt backend return a success-shaped response and bump
			// the basket count, but the basket never persists to the listable
			// cart — a silent "phantom basket" (issue #19). Failing here is far
			// better than reporting a fake success.
			if !looksLikeObjectID(venueMutationID) {
				warnings = dedupeStrings(warnings)
				return emitErrorWithWarnings(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_VENUE_UNRESOLVED",
					fmt.Sprintf(
						"Could not resolve venue %q to a Wolt venue id, so the item was NOT added (the basket would not persist). Pass the 24-character venue id or a full wolt.com venue URL.",
						strings.TrimSpace(venueArg),
					),
					warnings,
				)
			}

			addPayload := map[string]any{
				"items":    mergedItems,
				"venue_id": venueMutationID,
				"currency": currency,
			}
			resultPayload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.AddToBasket(cmd.Context(), addPayload, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, append(append([]string{}, warnings...), authWarnings...)...)
			}

			total := map[string]any{
				"amount":           count * price,
				"formatted_amount": formatMinorAmount(count*price, currency),
			}
			totalItems := count
			if countPayload, _, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketCount(cmd.Context(), authCtx)
				},
			); err == nil {
				if resolvedCount := asInt(countPayload["count"]); resolvedCount > 0 {
					totalItems = resolvedCount
				}
			}
			if page, _, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketsPage(cmd.Context(), location, authCtx)
				},
			); err == nil {
				state, _ := buildCartState(page, venueID)
				if resolvedTotal := asMap(state["total"]); resolvedTotal != nil {
					total = resolvedTotal
				}
			}

			data := map[string]any{
				"basket_id":     asString(resultPayload["id"]),
				"venue_id":      asString(coalesceAny(resultPayload["venue_id"], venueID)),
				"mutation":      "add",
				"line_id":       itemID,
				"total_items":   totalItems,
				"total":         total,
				"item_name":     name,
				"item_price":    price,
				"item_currency": currency,
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildCartMutationTable(data), flags.Output)
			}
			warnings = append(warnings, authWarnings...)
			warnings = dedupeStrings(warnings)
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().IntVar(&count, "count", 1, "Quantity to add.")
	cmd.Flags().StringArrayVar(&optionFlags, "option", nil, "Option selection in group-id=value-id or group-id=value-id:count form (IDs or names; repeatable).")
	cmd.Flags().BoolVar(&allowSubstitutions, "allow-substitutions", false, "Allow substitutions for unavailable items.")
	cmd.Flags().StringVar(&nameOverride, "name", "", "Override item display name.")
	cmd.Flags().IntVar(&priceOverride, "price", 0, "Override item price in minor units.")
	cmd.Flags().StringVar(&currencyOverride, "currency", "", "Override basket currency, for example EUR.")
	cmd.Flags().StringVar(&venueSlug, "venue-slug", "", "Venue slug used to enrich item metadata/options when needed.")
	cmd.Flags().StringVar(&nameQuery, "query", "", "Resolve <item-id> by searching the venue menu for an exact-name match. Errors when the query is ambiguous (use --cheapest to take the cheapest match instead).")
	cmd.Flags().BoolVar(&cheapest, "cheapest", false, "Resolve <item-id> to the cheapest in-stock menu item, optionally narrowed by --query. Skips sold-out and unpriced items; picks deterministically rather than erroring on ambiguity.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for cart totals refresh. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for cart totals refresh. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func newCartCountCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "count",
		Short: "Show basket item count.",
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

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketCount(cmd.Context(), authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
			}
			data := map[string]any{"count": asInt(payload["count"])}
			if format == output.FormatTable {
				return writeTable(cmd, output.RenderTable("Cart count", []string{"Count"}, [][]string{{asString(data["count"])}}), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	addGlobalFlags(cmd, &flags)
	return cmd
}

func newCartRemoveCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var venueID string
	var count int
	var all bool
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "remove <item-id>",
		Short: "Remove item quantity from basket.",
		Long:  "Remove item quantity from basket.\n\n<item-id> accepts a 24-char Mongo ObjectID or a Wolt item URL (.../venue/<slug>/itemid-<id>).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			if count <= 0 {
				return fmt.Errorf("%s", requiredArg("--count must be greater than 0"))
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
			selected, _, selectionWarnings := selectBasketWithMeta(page, venueID)
			if selected == nil {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_EMPTY_CART",
					"No basket found for selected venue.",
				)
			}

			itemRef := resolveItemReference(args[0])
			itemID := itemRef.ItemID
			if itemID == "" {
				trimmed := strings.TrimSpace(args[0])
				if looksURLShaped(trimmed) {
					return fmt.Errorf("could not extract an item id from %q; expected a Wolt item URL like .../venue/<slug>/itemid-<id>", trimmed)
				}
				itemID = trimmed
			}
			line, currentCount := findBasketLineByID(selected, itemID)
			if line == nil {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_ITEM_NOT_FOUND",
					fmt.Sprintf("Item %q not found in selected basket.", itemID),
				)
			}
			removeCount := count
			if all || removeCount > currentCount {
				removeCount = currentCount
			}
			nextCount := currentCount - removeCount
			basketID := asString(selected["id"])
			venue := asMap(selected["venue"])
			venueResolvedID := asString(venue["id"])
			currency := payloadutil.CurrencyFromBasket(selected)
			if currency == "" {
				currency = currencyFromVenue(cmd.Context(), deps, asString(coalesceAny(
					venue["slug"],
					venue["venue_slug"],
					venue["public_slug"],
					venue["url_slug"],
				)))
			}
			if currency == "" && nextCount > 0 {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_CURRENCY_UNKNOWN",
					"Item quantity was not changed because the venue currency could not be verified.",
				)
			}

			mutation := "remove"
			if nextCount <= 0 {
				if len(asSlice(selected["items"])) > 1 {
					return emitError(
						cmd,
						format,
						profile,
						flags.Locale,
						flags.Output,
						"WOLT_REMOVE_UNSUPPORTED",
						"Removing a full line from multi-item baskets is not supported by this endpoint yet. Use `cart clear` or remove fewer items.",
					)
				}
				mutation = "clear"
				if _, _, err := invokeWithAuthAutoRefresh(
					cmd.Context(),
					deps,
					flags,
					&auth,
					func(authCtx woltgateway.AuthContext) (map[string]any, error) {
						return deps.Wolt.DeleteBaskets(cmd.Context(), []string{basketID}, authCtx)
					},
				); err != nil {
					return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
				}
			} else {
				removePayload := map[string]any{
					"items": []any{
						buildBasketMutationItem(line, nextCount),
					},
					"venue_id": venueResolvedID,
					"currency": currency,
				}
				if _, _, err := invokeWithAuthAutoRefresh(
					cmd.Context(),
					deps,
					flags,
					&auth,
					func(authCtx woltgateway.AuthContext) (map[string]any, error) {
						return deps.Wolt.AddToBasket(cmd.Context(), removePayload, authCtx)
					},
				); err != nil {
					return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
				}
			}

			total := map[string]any{
				"amount":           0,
				"formatted_amount": formatMinorAmount(0, currency),
			}
			totalItems := maxInt(0, asInt(selected["total_items"])-removeCount)
			if countPayload, _, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketCount(cmd.Context(), authCtx)
				},
			); err == nil {
				totalItems = asInt(countPayload["count"])
			}
			if refreshedPage, _, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.BasketsPage(cmd.Context(), location, authCtx)
				},
			); err == nil {
				state, _ := buildCartState(refreshedPage, venueID)
				if resolvedTotal := asMap(state["total"]); resolvedTotal != nil {
					total = resolvedTotal
				}
			}

			data := map[string]any{
				"basket_id":     basketID,
				"venue_id":      venueResolvedID,
				"mutation":      mutation,
				"line_id":       itemID,
				"removed_count": removeCount,
				"total_items":   totalItems,
				"total":         total,
			}
			selectionWarnings = append(selectionWarnings, authWarnings...)
			if format == output.FormatTable {
				return writeTable(cmd, buildCartMutationTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, selectionWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&venueID, "venue-id", "", "Restrict mutation to one venue basket.")
	cmd.Flags().IntVar(&count, "count", 1, "Quantity to remove.")
	cmd.Flags().BoolVar(&all, "all", false, "Remove all quantity for this item.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for cart endpoints. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for cart endpoints. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func newCartClearCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var venueID string
	var all bool
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear selected basket or all baskets.",
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

			basketIDs := []string{}
			warnings := []string{}
			if all {
				for _, value := range asSlice(page["baskets"]) {
					basket := asMap(value)
					if basket == nil {
						continue
					}
					basketIDs = append(basketIDs, asString(basket["id"]))
				}
			} else {
				selected, _, selectionWarnings := selectBasketWithMeta(page, venueID)
				warnings = append(warnings, selectionWarnings...)
				if selected != nil {
					basketIDs = append(basketIDs, asString(selected["id"]))
				}
			}

			if len(basketIDs) == 0 {
				return emitError(
					cmd,
					format,
					profile,
					flags.Locale,
					flags.Output,
					"WOLT_EMPTY_CART",
					"No basket found to clear.",
				)
			}
			if _, _, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeleteBaskets(cmd.Context(), basketIDs, authCtx)
				},
			); err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
			}

			clearedIDs := make([]any, 0, len(basketIDs))
			for _, id := range basketIDs {
				clearedIDs = append(clearedIDs, id)
			}
			data := map[string]any{
				"mutation":        "clear",
				"basket_ids":      clearedIDs,
				"cleared_baskets": len(basketIDs),
				"total_items":     0,
				"total": map[string]any{
					"amount":           0,
					"formatted_amount": nil,
				},
			}
			warnings = append(warnings, authWarnings...)
			if format == output.FormatTable {
				return writeTable(cmd, buildCartMutationTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&venueID, "venue-id", "", "Restrict clear to one venue basket.")
	cmd.Flags().BoolVar(&all, "all", false, "Clear all baskets for the authenticated user.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for cart endpoints. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for cart endpoints. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func selectBasketWithMeta(page map[string]any, venueID string) (map[string]any, map[string]any, []string) {
	baskets := asSlice(page["baskets"])
	warnings := []string{}
	requestedVenueID := strings.TrimSpace(venueID)
	meta := map[string]any{
		"basket_count":       len(baskets),
		"requested_venue_id": emptyToNil(requestedVenueID),
		"selection_mode":     "none",
		"selected":           map[string]any{},
	}
	if len(baskets) == 0 {
		return nil, meta, warnings
	}

	if requestedVenueID == "" {
		selected := asMap(baskets[0])
		if selected == nil {
			return nil, meta, warnings
		}
		meta["selection_mode"] = "first-available"
		if len(baskets) > 1 {
			warnings = append(warnings, "multiple baskets found; using first basket (pass --venue-id to choose a specific cart)")
		}
		meta["selected"] = buildBasketSelectionDetails(selected)
		return selected, meta, warnings
	}

	for _, value := range baskets {
		basket := asMap(value)
		if basket == nil {
			continue
		}
		venue := asMap(basket["venue"])
		if strings.TrimSpace(asString(venue["id"])) == requestedVenueID {
			meta["selection_mode"] = "requested-venue-id"
			meta["selected"] = buildBasketSelectionDetails(basket)
			return basket, meta, warnings
		}
		venueSlug := strings.TrimSpace(asString(coalesceAny(venue["slug"], venue["venue_slug"], venue["public_slug"], venue["url_slug"])))
		if venueSlug != "" && strings.EqualFold(venueSlug, requestedVenueID) {
			meta["selection_mode"] = "requested-venue-slug"
			meta["selected"] = buildBasketSelectionDetails(basket)
			return basket, meta, warnings
		}
	}
	meta["selection_mode"] = "not-found"
	return nil, meta, warnings
}

func buildBasketSelectionDetails(basket map[string]any) map[string]any {
	venue := asMap(basket["venue"])
	return map[string]any{
		"basket_id":  asString(basket["id"]),
		"venue_id":   asString(venue["id"]),
		"venue_name": asString(venue["name"]),
		"venue_slug": asString(coalesceAny(venue["slug"], venue["venue_slug"], venue["public_slug"], venue["url_slug"])),
	}
}

func buildCartState(page map[string]any, venueID string) (map[string]any, []string) {
	warnings := []string{}
	selected, selection, selectionWarnings := selectBasketWithMeta(page, venueID)
	warnings = append(warnings, selectionWarnings...)
	if selected == nil {
		warnings = append(warnings, "no basket found for selected venue")
		return map[string]any{
			"basket_id": "",
			"venue_id":  strings.TrimSpace(venueID),
			"selection": selection,
			"currency":  "",
			"lines":     []any{},
			"subtotal":  map[string]any{"amount": 0, "formatted_amount": nil},
			"fees":      []any{},
			"total":     map[string]any{"amount": 0, "formatted_amount": nil},
		}, warnings
	}

	venue := asMap(selected["venue"])
	totalFormatted := asString(selected["total"])
	currency := payloadutil.CurrencyFromBasket(selected)
	items := asSlice(selected["items"])
	lines := make([]any, 0, len(items))
	subtotalAmount := 0
	totalItems := 0
	for _, value := range items {
		item := asMap(value)
		if item == nil {
			continue
		}
		count := asInt(item["count"])
		price := asInt(item["price"])
		lineAmount := price * count
		subtotalAmount += lineAmount
		totalItems += count
		lines = append(lines, map[string]any{
			"line_id": asString(item["id"]),
			"item_id": asString(item["id"]),
			"name":    asString(item["name"]),
			"count":   count,
			"options": coalesceAny(item["options"], []any{}),
			"price": map[string]any{
				"amount":           price,
				"formatted_amount": formatMinorAmount(price, currency),
			},
			"line_total": map[string]any{
				"amount":           lineAmount,
				"formatted_amount": formatMinorAmount(lineAmount, currency),
			},
		})
	}

	totalAmount := asInt(asMap(selected["telemetry"])["basket_total"])
	if totalAmount <= 0 {
		totalAmount = subtotalAmount
	}
	totalDisplay := totalFormatted
	if strings.TrimSpace(totalDisplay) == "" {
		totalDisplay = formatMinorAmount(totalAmount, currency)
	}

	return map[string]any{
		"basket_id":   asString(selected["id"]),
		"venue_id":    asString(venue["id"]),
		"venue_name":  asString(venue["name"]),
		"venue_slug":  asString(coalesceAny(venue["slug"], venue["venue_slug"], venue["public_slug"], venue["url_slug"])),
		"selection":   selection,
		"currency":    currency,
		"total_items": totalItems,
		"lines":       lines,
		"subtotal": map[string]any{
			"amount":           subtotalAmount,
			"formatted_amount": formatMinorAmount(subtotalAmount, currency),
		},
		"fees": []any{},
		"total": map[string]any{
			"amount":           totalAmount,
			"formatted_amount": emptyToNil(totalDisplay),
		},
	}, warnings
}

func buildCartTable(data map[string]any, includeDetails bool) string {
	summaryRows := [][]string{
		{"Basket ID", fallbackString(asString(data["basket_id"]), "-")},
		{"Venue ID", fallbackString(asString(data["venue_id"]), "-")},
		{"Venue name", fallbackString(asString(data["venue_name"]), "-")},
		{"Venue slug", fallbackString(asString(data["venue_slug"]), "-")},
		{"Items", asString(data["total_items"])},
		{"Total", fallbackString(asString(asMap(data["total"])["formatted_amount"]), "-")},
	}
	if selection := asMap(data["selection"]); selection != nil {
		summaryRows = append(summaryRows, []string{
			"Selection mode",
			fallbackString(asString(selection["selection_mode"]), "-"),
		})
		summaryRows = append(summaryRows, []string{
			"Baskets available",
			asString(selection["basket_count"]),
		})
		if selected := asMap(selection["selected"]); selected != nil {
			summaryRows = append(summaryRows, []string{
				"Selected basket",
				fallbackString(asString(selected["basket_id"]), "-"),
			})
		}
	}
	summary := output.RenderTable("Cart summary", []string{"Field", "Value"}, summaryRows)

	headers := []string{"Item", "Item ID", "Count", "Price", "Line total", "Options"}
	rows := [][]string{}
	for _, value := range asSlice(data["lines"]) {
		line := asMap(value)
		price := asString(asMap(line["price"])["formatted_amount"])
		if price == "" {
			price = "-"
		}
		lineTotal := asString(asMap(line["line_total"])["formatted_amount"])
		if lineTotal == "" {
			lineTotal = "-"
		}
		optionCount := len(asSlice(line["options"]))
		rows = append(rows, []string{
			asString(line["name"]),
			fallbackString(asString(line["item_id"]), "-"),
			asString(line["count"]),
			price,
			lineTotal,
			asString(optionCount),
		})
		if includeDetails {
			for _, detail := range cartLineDetails(line, asString(data["currency"])) {
				rows = append(rows, []string{
					"  " + detail,
					"",
					"",
					"",
					"",
					"",
				})
			}
		}
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "0", "-", "-", "0"})
	}
	itemsTable := output.RenderTable("Cart items", headers, rows)
	return summary + "\n\n" + itemsTable
}

func cartLineDetails(line map[string]any, currency string) []string {
	options := asSlice(line["options"])
	if len(options) == 0 {
		return nil
	}

	details := make([]string, 0, len(options))
	for _, optionValue := range options {
		option := asMap(optionValue)
		if option == nil {
			continue
		}
		groupLabel := strings.TrimSpace(asString(coalesceAny(option["name"], option["title"], option["id"])))
		if groupLabel == "" {
			groupLabel = "option"
		}
		values := asSlice(option["values"])
		parts := make([]string, 0, len(values))
		for _, selectedValue := range values {
			value := asMap(selectedValue)
			if value == nil {
				continue
			}
			label := strings.TrimSpace(asString(coalesceAny(value["name"], value["title"], value["id"])))
			if label == "" {
				continue
			}
			count := asInt(value["count"])
			if count <= 0 {
				count = 1
			}
			part := label
			if count > 1 {
				part = fmt.Sprintf("%s x%d", label, count)
			}
			if extra := asInt(value["price"]); extra > 0 {
				if formatted := formatMinorAmount(extra, currency); formatted != "" {
					part = fmt.Sprintf("%s (+%s)", part, formatted)
				}
			}
			parts = append(parts, part)
		}

		if len(parts) > 0 {
			details = append(details, fmt.Sprintf("%s: %s", groupLabel, strings.Join(parts, ", ")))
			continue
		}

		encoded, err := json.Marshal(option["values"])
		if err != nil || len(encoded) == 0 {
			details = append(details, fmt.Sprintf("%s: []", groupLabel))
			continue
		}
		details = append(details, fmt.Sprintf("%s: %s", groupLabel, string(encoded)))
	}
	return details
}

func buildCartMutationTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Mutation", asString(data["mutation"])},
		{"Basket ID", fallbackString(asString(data["basket_id"]), strings.Join(toStringSlice(asSlice(data["basket_ids"])), ", "))},
		{"Venue ID", asString(data["venue_id"])},
		{"Line ID", asString(data["line_id"])},
		{"Removed count", asString(data["removed_count"])},
		{"Total items", asString(data["total_items"])},
		{"Total", fallbackString(asString(asMap(data["total"])["formatted_amount"]), "-")},
	}
	return output.RenderTable("Cart mutation", headers, rows)
}

func findBasketLineByID(basket map[string]any, itemID string) (map[string]any, int) {
	target := strings.TrimSpace(itemID)
	if target == "" {
		return nil, 0
	}
	for _, value := range asSlice(basket["items"]) {
		line := asMap(value)
		if line == nil {
			continue
		}
		if strings.TrimSpace(asString(line["id"])) == target {
			return line, asInt(line["count"])
		}
	}
	return nil, 0
}

func buildBasketMutationItem(line map[string]any, count int) map[string]any {
	item := buildBasketUpsertItem(line, count)
	item["price"] = asInt(item["price"]) * count
	return item
}

func buildBasketUpsertItem(line map[string]any, count int) map[string]any {
	if count <= 0 {
		count = 1
	}
	price := asInt(line["price"])
	lineOptions := make([]any, 0, len(asSlice(line["options"])))
	for _, optionValue := range asSlice(line["options"]) {
		option := asMap(optionValue)
		if option == nil {
			continue
		}
		values := make([]any, 0, len(asSlice(option["values"])))
		for _, value := range asSlice(option["values"]) {
			valueMap := asMap(value)
			if valueMap == nil {
				continue
			}
			valueCount := asInt(valueMap["count"])
			if valueCount <= 0 {
				valueCount = 1
			}
			values = append(values, map[string]any{
				"id":    asString(valueMap["id"]),
				"count": valueCount,
				"price": asInt(valueMap["price"]),
			})
		}
		lineOptions = append(lineOptions, map[string]any{
			"id":     asString(option["id"]),
			"values": values,
		})
	}
	return map[string]any{
		"id":      asString(line["id"]),
		"count":   count,
		"name":    asString(line["name"]),
		"price":   price,
		"options": lineOptions,
		"substitution_settings": map[string]any{
			"is_allowed": asBool(asMap(line["substitution_settings"])["is_allowed"]),
		},
	}
}

func needsAssortmentFallback(itemPayload map[string]any) bool {
	if len(itemPayload) == 0 {
		return true
	}
	if price := asInt(asMap(itemPayload["price"])["amount"]); price > 0 {
		return len(extractOptionSpecs(itemPayload)) == 0
	}
	if price := asInt(itemPayload["price"]); price > 0 {
		return len(extractOptionSpecs(itemPayload)) == 0
	}
	return true
}

func mergeItemPayloadFallback(base map[string]any, fallback map[string]any) map[string]any {
	if len(base) == 0 {
		return fallback
	}
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	if strings.TrimSpace(asString(merged["name"])) == "" {
		merged["name"] = fallback["name"]
	}
	if asInt(asMap(merged["price"])["amount"]) <= 0 && asInt(merged["price"]) <= 0 {
		merged["price"] = fallback["price"]
		merged["base_price"] = fallback["base_price"]
	}
	if len(extractOptionSpecs(merged)) == 0 {
		merged["option_groups"] = fallback["option_groups"]
		merged["options"] = fallback["options"]
		merged["items"] = fallback["items"]
	}
	return merged
}

func toStringSlice(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		str := strings.TrimSpace(asString(value))
		if str == "" {
			continue
		}
		out = append(out, str)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
