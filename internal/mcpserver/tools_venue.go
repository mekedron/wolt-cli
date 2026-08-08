package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/catalogload"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

func registerVenueTools(srv *mcp.Server, tc *ToolCtx) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_detail",
		Title:       "Venue detail",
		Description: "Show details for one venue: address, currency, rating, delivery methods, opening windows, tags. Accepts a slug, raw venue id, or a wolt.com URL.",
		Annotations: readOnly,
	}, tc.handleVenueDetail)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_resolve_venue",
		Title:       "Resolve an exact venue",
		Description: "Resolve a venue by exact name, slug, raw venue id, or Wolt URL using supported Wolt search/static data plus an exact-match discovery fallback. It is not limited to the non-exhaustive discovery feed and includes closed and scheduled-order venues.",
		Annotations: readOnly,
	}, tc.handleResolveVenue)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_menu",
		Title:       "Venue menu",
		Description: "Browse a venue's menu. Returns normalized items with prices, image URLs, discounts, and current availability. Supports query/category filters and pagination.",
		Annotations: readOnly,
	}, tc.handleVenueMenu)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_hours",
		Title:       "Venue opening hours",
		Description: "Return venue-local opening windows in the venue's timezone.",
		Annotations: readOnly,
	}, tc.handleVenueHours)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_item",
		Title:       "Item detail",
		Description: "Fetch one item's current payload (name, price, image URLs, options, availability, description). Useful before adding to a cart.",
		Annotations: readOnly,
	}, tc.handleVenueItem)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_search_items",
		Title:       "Search items within a venue",
		Description: "Search a venue's menu by free-text query. Returns matching items with price + discount info.",
		Annotations: readOnly,
	}, tc.handleVenueSearchItems)
}

// ---------------- wolt_venue_detail ----------------

type VenueDetailInput struct {
	LocationInput
	Venue   string `json:"venue"             jsonschema:"venue slug, 24-char id, or wolt.com URL"`
	Include string `json:"include,omitempty" jsonschema:"comma-separated extras: hours,tags,rating,fees,promotions (default: all)"`
}

type VenueDetailOutput struct {
	Summary        string         `json:"summary"`
	Venue          map[string]any `json:"venue"`
	Location       *LocationOut   `json:"location,omitempty"`
	LocationSource string         `json:"location_source,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleVenueDetail(ctx context.Context, _ *mcp.CallToolRequest, in VenueDetailInput) (*mcp.CallToolResult, VenueDetailOutput, error) {
	warnings := []string{}
	normalized := normalizeVenueInput(in.Venue)
	if normalized == "" {
		return nil, VenueDetailOutput{}, toolErr(fmt.Errorf("venue identifier is required (slug, id, or wolt.com URL)"))
	}
	directReference := isDirectVenueReference(in.Venue, normalized)
	explicitLocation := in.Lat != 0 || in.Lon != 0 || strings.TrimSpace(in.Address) != ""

	location, source, locationErr := tc.resolveLocation(ctx, in.LocationInput)
	var locationPtr *domain.Location
	switch {
	case locationErr == nil:
		locationPtr = &location
	case explicitLocation || !directReference:
		return nil, VenueDetailOutput{}, toolErr(fmt.Errorf("venue_detail needs a location: %w", locationErr))
	default:
		warnings = append(warnings, "delivery-area availability is unknown because no location was available")
	}

	var (
		ref       venueRef
		candidate map[string]any
		err       error
	)
	if locationPtr != nil {
		ref, candidate, err = tc.resolveVenueRefWithSearch(ctx, in.Venue, location)
	} else {
		ref, err = tc.resolveVenueRef(ctx, in.Venue)
		if err == nil && (!looksLikeObjectID(ref.ID) || strings.TrimSpace(ref.Slug) == "") {
			err = fmt.Errorf(
				"venue_detail could not resolve %q directly; pass a canonical slug, id, or URL, or provide a location for exact-name search",
				in.Venue,
			)
		}
	}
	if err != nil {
		return nil, VenueDetailOutput{}, toolErr(err)
	}

	staticPayload := ref.StaticPayload
	if staticPayload == nil {
		var staticErr error
		staticPayload, staticErr = tc.wolt.VenuePageStatic(ctx, firstNonEmpty(ref.Slug, ref.ID))
		if staticErr != nil {
			warnings = append(warnings, "venue static details could not be loaded")
			staticPayload = nil
		} else {
			applyVenueIdentity(&ref, staticPayload)
		}
	}

	var dynamicPayload map[string]any
	if strings.TrimSpace(ref.Slug) != "" {
		dynamicPayload, err = tc.requestVenuePageDynamic(ctx, ref.Slug, woltgateway.VenuePageDynamicOptions{
			Location:               locationPtr,
			SelectedDeliveryMethod: "homedelivery",
			Auth:                   tc.optionalAuth(ctx),
		})
		if err != nil {
			warnings = append(warnings, "location-aware venue availability could not be loaded")
			dynamicPayload = nil
		}
	}
	includes := parseIncludeSet(in.Include, []string{"hours", "tags", "rating", "fees", "promotions"})
	data, buildWarnings, buildErr := observability.BuildVenueDetailFromPayload(
		venueIdentityFromRef(ref, in.Venue),
		staticPayload,
		dynamicPayload,
		candidate,
		locationPtr,
		includes,
	)
	if buildErr != nil {
		return nil, VenueDetailOutput{}, toolErr(buildErr)
	}
	warnings = append(warnings, buildWarnings...)
	out := VenueDetailOutput{
		Summary:  fmt.Sprintf("venue %s (%s)", asString(data["name"]), asString(data["slug"])),
		Venue:    data,
		Warnings: warnings,
	}
	if locationPtr != nil {
		locationValue := locationOut(location)
		out.Location = &locationValue
		out.LocationSource = source
	}
	return nil, out, nil
}

// ---------------- wolt_resolve_venue ----------------

type ResolveVenueInput struct {
	LocationInput
	Venue string `json:"venue" jsonschema:"exact venue name, slug, 24-char id, or wolt.com URL"`
}

type ResolveVenueOutput struct {
	Summary        string         `json:"summary"`
	Venue          map[string]any `json:"venue"`
	Location       LocationOut    `json:"location"`
	LocationSource string         `json:"location_source"`
	Warnings       []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleResolveVenue(ctx context.Context, _ *mcp.CallToolRequest, in ResolveVenueInput) (*mcp.CallToolResult, ResolveVenueOutput, error) {
	location, source, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, ResolveVenueOutput{}, toolErr(fmt.Errorf("venue resolver needs a location: %w", err))
	}
	ref, candidate, err := tc.resolveVenueRefWithSearch(ctx, in.Venue, location)
	if err != nil {
		return nil, ResolveVenueOutput{}, toolErr(err)
	}
	warnings := []string{}
	staticPayload := ref.StaticPayload
	if staticPayload == nil {
		var staticErr error
		staticPayload, staticErr = tc.wolt.VenuePageStatic(ctx, firstNonEmpty(ref.Slug, ref.ID))
		if staticErr != nil {
			warnings = append(warnings, "venue static details could not be loaded")
			staticPayload = nil
		} else {
			applyVenueIdentity(&ref, staticPayload)
		}
	}
	var dynamicPayload map[string]any
	var dynamicErr error
	if strings.TrimSpace(ref.Slug) != "" {
		dynamicPayload, dynamicErr = tc.requestVenuePageDynamic(ctx, ref.Slug, woltgateway.VenuePageDynamicOptions{
			Location:               &location,
			SelectedDeliveryMethod: "homedelivery",
			Auth:                   tc.optionalAuth(ctx),
		})
	}
	data, buildWarnings, buildErr := observability.BuildVenueDetailFromPayload(
		venueIdentityFromRef(ref, in.Venue),
		staticPayload,
		dynamicPayload,
		candidate,
		&location,
		parseIncludeSet("", []string{"hours", "tags", "rating", "fees", "promotions"}),
	)
	if buildErr != nil {
		return nil, ResolveVenueOutput{}, toolErr(buildErr)
	}
	warnings = append(warnings, buildWarnings...)
	if dynamicErr != nil {
		warnings = append(warnings, "location-aware venue availability could not be loaded")
	}
	return nil, ResolveVenueOutput{
		Summary:        fmt.Sprintf("resolved venue %s (%s)", asString(data["name"]), asString(data["slug"])),
		Venue:          data,
		Location:       locationOut(location),
		LocationSource: source,
		Warnings:       warnings,
	}, nil
}

// ---------------- wolt_venue_menu ----------------

type VenueMenuInput struct {
	Venue    string `json:"venue"               jsonschema:"venue slug, id, or url"`
	Query    string `json:"query,omitempty"     jsonschema:"case-insensitive substring filter on item name"`
	Category string `json:"category,omitempty"  jsonschema:"exact leaf category slug from data.catalog.available_categories"`
	Limit    int    `json:"limit,omitempty"     jsonschema:"max items"`
	Offset   int    `json:"offset,omitempty"    jsonschema:"skip first N"`
}

type VenueMenuOutput struct {
	Summary  string         `json:"summary"`
	Data     map[string]any `json:"data"`
	Warnings []string       `json:"warnings,omitempty"`
}

const noMenuItemsWarning = "no menu items were discovered in upstream venue payloads"

type venueMenuSelection struct {
	payloads                  []map[string]any
	metadataPayloads          []map[string]any
	warnings                  []string
	availableCategories       []catalogload.Category
	loadedCategorySlugs       []string
	categoryFilter            string
	name                      string
	rootMaterializedItemCount int
	partial                   bool
	complete                  bool
	hasQuery                  bool
	hasCategory               bool
}

func (tc *ToolCtx) handleVenueMenu(ctx context.Context, _ *mcp.CallToolRequest, in VenueMenuInput) (*mcp.CallToolResult, VenueMenuOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueMenuOutput{}, toolErr(err)
	}
	payload, err := tc.wolt.AssortmentByVenueSlug(ctx, firstNonEmpty(ref.Slug, in.Venue))
	if err != nil {
		return nil, VenueMenuOutput{}, toolErr(err)
	}
	venueSlug := firstNonEmpty(ref.Slug, in.Venue)
	selection, err := tc.prepareVenueMenuSelection(ctx, venueSlug, payload, in)
	if err != nil {
		return nil, VenueMenuOutput{}, toolErr(err)
	}

	identity := observability.ExtractVenueIdentity(
		venueIdentityFromRef(ref, in.Venue),
		ref.StaticPayload,
		payload,
	)
	data, menuWarnings := observability.BuildVenueMenu(
		ref.ID,
		selection.payloads,
		selection.categoryFilter,
		false,
		nil,
		observability.ItemVenueContext{
			VenueID:          identity.ID,
			VenueSlug:        identity.Slug,
			CanonicalURL:     identity.CanonicalURL,
			Currency:         resolveVenueCurrency(ctx, tc, ref, nil, ref.StaticPayload, payload),
			MetadataPayloads: selection.metadataPayloads,
		},
	)
	summary, warnings := finalizeVenueMenuData(
		data,
		payload,
		venueSlug,
		in,
		selection,
		append(selection.warnings, menuWarnings...),
	)
	return nil, VenueMenuOutput{
		Summary:  summary,
		Data:     data,
		Warnings: warnings,
	}, nil
}

func (tc *ToolCtx) prepareVenueMenuSelection(
	ctx context.Context,
	venueSlug string,
	rootPayload map[string]any,
	in VenueMenuInput,
) (venueMenuSelection, error) {
	materializedItemIDs := materializedMenuItemIDs(rootPayload)
	availableCategories := catalogload.Categories(rootPayload)
	partial := catalogload.RootIsPartial(rootPayload, materializedItemIDs)
	selection := venueMenuSelection{
		payloads:                  []map[string]any{rootPayload},
		metadataPayloads:          []map[string]any{},
		warnings:                  []string{},
		availableCategories:       availableCategories,
		loadedCategorySlugs:       []string{},
		categoryFilter:            in.Category,
		name:                      "full",
		rootMaterializedItemCount: len(materializedItemIDs),
		partial:                   partial,
		complete:                  !partial,
		hasQuery:                  strings.TrimSpace(in.Query) != "",
		hasCategory:               strings.TrimSpace(in.Category) != "",
	}

	language := domain.ResolveAssortmentLanguage(tc.locale)
	switch {
	case selection.hasCategory:
		categorySlug := strings.TrimSpace(in.Category)
		categoryResult, err := catalogload.LoadCategory(
			ctx,
			tc.wolt,
			venueSlug,
			categorySlug,
			language,
			tc.optionalAuth(ctx),
		)
		if err != nil {
			return venueMenuSelection{}, fmt.Errorf("load category %q: %w", categorySlug, err)
		}
		// The category endpoint is authoritative for this selection. Root
		// assortment items can belong to other categories or be stale copies.
		selection.payloads = []map[string]any{categoryResult.Payload}
		selection.metadataPayloads = []map[string]any{rootPayload}
		selection.warnings = append(selection.warnings, categoryResult.Warnings...)
		selection.loadedCategorySlugs = append(selection.loadedCategorySlugs, categorySlug)
		selection.categoryFilter = ""
		selection.name = "category"
		selection.complete = categoryResult.Complete
	case selection.hasQuery:
		searchPayload, err := requestAssortmentSearch(
			ctx,
			tc,
			venueSlug,
			in.Query,
			language,
			tc.optionalAuth(ctx),
		)
		if err != nil {
			return venueMenuSelection{}, err
		}
		// The search endpoint is authoritative for which items matched. Keep
		// root assortment items out of this selection; venue identity and
		// currency are supplied separately through the resolved context.
		selection.payloads = []map[string]any{searchPayload}
		selection.metadataPayloads = []map[string]any{rootPayload}
		selection.name = "search"
		selection.complete = true
	case selection.partial && selection.rootMaterializedItemCount == 0:
		selection.name = "metadata_only"
		selection.complete = false
	}
	return selection, nil
}

func finalizeVenueMenuData(
	data map[string]any,
	rootPayload map[string]any,
	venueSlug string,
	in VenueMenuInput,
	selection venueMenuSelection,
	warnings []string,
) (string, []string) {
	if selection.hasQuery || selection.hasCategory || selection.partial {
		warnings = removeWarning(warnings, noMenuItemsWarning)
	}
	if selection.partial && selection.name == "metadata_only" {
		warnings = append(
			warnings,
			"partial catalog: root assortment exposes category metadata but not the full item catalog; pass category=<leaf slug> from data.catalog.available_categories or use wolt_venue_search_items",
		)
	}
	if selection.hasQuery && selection.hasCategory {
		filterMenuItemsByQuery(data, in.Query)
	}

	rows := asSlice(data["items"])
	selectionItemCount := len(rows)
	rows = paginateVenueMenuRows(rows, in.Offset, in.Limit)
	data["items"] = rows
	if strings.TrimSpace(asString(data["venue_slug"])) == "" {
		data["venue_slug"] = venueSlug
	}
	if selection.hasCategory && selectionItemCount == 0 {
		warnings = append(warnings, emptyVenueMenuSelectionWarning(in))
	}

	warnings = addVenueMenuCatalogMetadata(
		data,
		rootPayload,
		in,
		selection,
		len(rows),
		warnings,
	)
	if selection.partial && selection.name == "metadata_only" {
		return fmt.Sprintf(
			"partial catalog: %d categories available; no category loaded",
			len(selection.availableCategories),
		), warnings
	}
	return humanCount(len(rows), "item", "items"), warnings
}

func addVenueMenuCatalogMetadata(
	data map[string]any,
	rootPayload map[string]any,
	in VenueMenuInput,
	selection venueMenuSelection,
	itemsReturned int,
	warnings []string,
) []string {
	status := "complete"
	catalogComplete := true
	switch {
	case selection.partial:
		status = "partial"
		catalogComplete = false
	case selection.rootMaterializedItemCount == 0 && len(selection.availableCategories) == 0:
		status = "unavailable"
		catalogComplete = false
		if selection.name == "full" {
			selection.complete = false
		}
		warnings = removeWarning(warnings, noMenuItemsWarning)
		warnings = append(
			warnings,
			"catalog unavailable: the root assortment returned neither materialized items nor category metadata",
		)
	}
	data["catalog"] = map[string]any{
		"status":                status,
		"complete":              catalogComplete,
		"loading_strategy":      strings.TrimSpace(asString(rootPayload["loading_strategy"])),
		"selection":             selection.name,
		"selection_complete":    selection.complete,
		"requested_category":    emptyStringToNil(strings.TrimSpace(in.Category)),
		"available_categories":  selection.availableCategories,
		"loaded_category_slugs": selection.loadedCategorySlugs,
		"items_returned":        itemsReturned,
	}
	if selection.partial && selection.name == "metadata_only" && len(asSlice(data["categories"])) == 0 {
		categoryNames := make([]string, 0, len(selection.availableCategories))
		for _, category := range selection.availableCategories {
			categoryNames = append(categoryNames, category.Name)
		}
		data["categories"] = categoryNames
	}
	return warnings
}

func paginateVenueMenuRows(rows []any, offset int, limit int) []any {
	if offset > 0 {
		if offset >= len(rows) {
			return []any{}
		}
		rows = rows[offset:]
	}
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	return rows
}

func emptyVenueMenuSelectionWarning(in VenueMenuInput) string {
	if strings.TrimSpace(in.Query) != "" {
		return fmt.Sprintf(
			"no menu items matched query %q in category %q",
			strings.TrimSpace(in.Query),
			strings.TrimSpace(in.Category),
		)
	}
	return fmt.Sprintf(
		"category %q returned no menu items",
		strings.TrimSpace(in.Category),
	)
}

func materializedMenuItemIDs(payload map[string]any) []string {
	itemIDs := []string{}
	for _, item := range observability.ExtractMenuItems(payload, "", "") {
		itemID := strings.TrimSpace(asString(item["item_id"]))
		if itemID != "" {
			itemIDs = append(itemIDs, itemID)
		}
	}
	return itemIDs
}

// ---------------- wolt_venue_hours ----------------

type VenueHoursInput struct {
	Venue    string `json:"venue"             jsonschema:"venue slug, id, or url"`
	Timezone string `json:"timezone,omitempty" jsonschema:"expected venue timezone; a differing value is not applied to undated venue-local weekly hours"`
}

type VenueHoursOutput struct {
	Summary  string         `json:"summary"`
	Hours    map[string]any `json:"hours"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleVenueHours(ctx context.Context, _ *mcp.CallToolRequest, in VenueHoursInput) (*mcp.CallToolResult, VenueHoursOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueHoursOutput{}, toolErr(err)
	}
	staticPayload := ref.StaticPayload
	if staticPayload == nil {
		staticPayload, err = tc.wolt.VenuePageStatic(ctx, firstNonEmpty(ref.Slug, ref.ID))
		if err != nil {
			return nil, VenueHoursOutput{}, toolErr(fmt.Errorf("venue hours unavailable for %q: %w", in.Venue, err))
		}
		applyVenueIdentity(&ref, staticPayload)
	}
	data, warnings, buildErr := observability.BuildVenueHoursFromPayload(
		venueIdentityFromRef(ref, in.Venue),
		staticPayload,
		in.Timezone,
	)
	if buildErr != nil {
		return nil, VenueHoursOutput{}, toolErr(buildErr)
	}
	return nil, VenueHoursOutput{
		Summary:  fmt.Sprintf("opening windows for %s", asString(data["venue_id"])),
		Hours:    data,
		Warnings: warnings,
	}, nil
}

// ---------------- wolt_venue_item ----------------

type VenueItemInput struct {
	Venue  string `json:"venue"   jsonschema:"venue slug, id, or url"`
	ItemID string `json:"item_id" jsonschema:"24-char item id (e.g. from wolt_venue_menu)"`
}

type VenueItemOutput struct {
	Summary  string         `json:"summary"`
	Item     map[string]any `json:"item"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleVenueItem(ctx context.Context, _ *mcp.CallToolRequest, in VenueItemInput) (*mcp.CallToolResult, VenueItemOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueItemOutput{}, toolErr(err)
	}
	if strings.TrimSpace(in.ItemID) == "" {
		return nil, VenueItemOutput{}, toolErrf("item_id is required")
	}
	var payload map[string]any
	var pageErr error
	if domain.IsObjectID(ref.ID) {
		payload, pageErr = tc.wolt.VenueItemPage(ctx, ref.ID, in.ItemID)
	}
	var currentItem map[string]any
	currentLookupSucceeded := false
	warnings := []string{}
	if strings.TrimSpace(ref.Slug) != "" {
		currentPayload, currentErr := requestAssortmentItems(
			ctx,
			tc,
			ref.Slug,
			[]string{in.ItemID},
			tc.optionalAuth(ctx),
		)
		if currentErr == nil {
			currentLookupSucceeded = true
			currentItem = catalogitem.ScopedItem(currentPayload, in.ItemID)
		} else {
			warnings = append(warnings, "current item availability could not be verified")
		}
	}
	if scoped := catalogitem.ScopedItem(payload, in.ItemID); scoped != nil {
		payload = scoped
	}
	if currentItem != nil {
		payload = catalogitem.MergeCurrentItem(payload, currentItem)
	} else if pageErr != nil && !currentLookupSucceeded {
		return nil, VenueItemOutput{}, toolErr(pageErr)
	}
	if currentLookupSucceeded && currentItem == nil {
		payload = catalogitem.MarkMissingFromCurrentAssortment(payload, in.ItemID)
		warnings = append(warnings, "item is missing from the current assortment")
	}
	if payload == nil && !currentLookupSucceeded {
		return nil, VenueItemOutput{}, toolErrf("item detail is unavailable")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if currency := resolveVenueCurrency(ctx, tc, ref, nil, payload); currency != "" {
		payload["currency"] = currency
	}
	identity := observability.ExtractVenueIdentity(
		venueIdentityFromRef(ref, in.Venue),
		ref.StaticPayload,
		payload,
	)
	itemContext := observability.ItemVenueContext{
		VenueID:              identity.ID,
		VenueSlug:            identity.Slug,
		CanonicalURL:         identity.CanonicalURL,
		Currency:             resolveVenueCurrency(ctx, tc, ref, nil, ref.StaticPayload, payload),
		AvailabilityVerified: &currentLookupSucceeded,
	}
	data, detailWarnings := observability.BuildItemDetail(in.ItemID, ref.ID, payload, false, itemContext)
	warnings = append(warnings, detailWarnings...)
	return nil, VenueItemOutput{
		Summary:  fmt.Sprintf("item %s", asString(data["name"])),
		Item:     data,
		Warnings: warnings,
	}, nil
}

// ---------------- wolt_venue_search_items ----------------

type VenueSearchItemsInput struct {
	Venue  string `json:"venue"        jsonschema:"venue slug, id, or url"`
	Query  string `json:"query"        jsonschema:"item name query"`
	Limit  int    `json:"limit,omitempty"  jsonschema:"max items"`
	Offset int    `json:"offset,omitempty" jsonschema:"skip first N"`
}

type VenueSearchItemsOutput struct {
	Summary  string         `json:"summary"`
	Data     map[string]any `json:"data"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleVenueSearchItems(ctx context.Context, _ *mcp.CallToolRequest, in VenueSearchItemsInput) (*mcp.CallToolResult, VenueSearchItemsOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueSearchItemsOutput{}, toolErr(err)
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, VenueSearchItemsOutput{}, toolErrf("query is required")
	}
	language := domain.ResolveAssortmentLanguage(tc.locale)
	payload, err := requestAssortmentSearch(
		ctx,
		tc,
		firstNonEmpty(ref.Slug, in.Venue),
		in.Query,
		language,
		tc.optionalAuth(ctx),
	)
	if err != nil {
		return nil, VenueSearchItemsOutput{}, toolErr(err)
	}
	var limitPtr *int
	if in.Limit > 0 {
		limitPtr = &in.Limit
	}
	identity := observability.ExtractVenueIdentity(
		venueIdentityFromRef(ref, in.Venue),
		ref.StaticPayload,
		payload,
	)
	itemContext := observability.ItemVenueContext{
		VenueID:      identity.ID,
		VenueSlug:    identity.Slug,
		CanonicalURL: identity.CanonicalURL,
		Currency:     resolveVenueCurrency(ctx, tc, ref, nil, ref.StaticPayload, payload),
	}
	data, warnings := observability.BuildItemSearchResult(
		in.Query,
		[]map[string]any{payload},
		"",
		limitPtr,
		in.Offset,
		nil,
		itemContext,
	)
	return nil, VenueSearchItemsOutput{
		Summary:  humanCount(len(asSlice(data["items"])), "match", "matches"),
		Data:     data,
		Warnings: warnings,
	}, nil
}

// ---------------- shared helpers used by venue tools ----------------

func parseIncludeSet(raw string, defaults []string) map[string]struct{} {
	out := map[string]struct{}{}
	values := strings.TrimSpace(raw)
	if values == "" {
		for _, v := range defaults {
			out[v] = struct{}{}
		}
		return out
	}
	for _, part := range strings.Split(values, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token != "" {
			out[token] = struct{}{}
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func venueIdentityFromRef(ref venueRef, raw string) observability.VenueIdentity {
	return observability.VenueIdentity{
		ID:           strings.TrimSpace(ref.ID),
		Slug:         strings.TrimSpace(ref.Slug),
		CanonicalURL: domain.CanonicalVenueURL(raw, ""),
	}
}

func filterMenuItemsByQuery(data map[string]any, query string) {
	if strings.TrimSpace(query) == "" {
		return
	}
	rows := asSlice(data["items"])
	filtered := make([]any, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		if row == nil {
			continue
		}
		if observability.ItemMatchesQuery(row, query) {
			filtered = append(filtered, raw)
		}
	}
	data["items"] = filtered
}

func removeWarning(warnings []string, exact string) []string {
	out := warnings[:0]
	for _, warning := range warnings {
		if warning != exact {
			out = append(out, warning)
		}
	}
	return out
}

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
