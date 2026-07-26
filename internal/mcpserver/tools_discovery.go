package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/service/observability"
)

func registerDiscoveryTools(srv *mcp.Server, tc *ToolCtx) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_feed",
		Title:       "Wolt discovery feed",
		Description: "Browse the Wolt discovery home page for a location, grouped by sections like 'Popular', 'Order again', 'Fastest delivery'. One upstream call. Returns sections with their featured venues.",
		Annotations: readOnly,
	}, tc.handleFeed)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_top",
		Title:       "Top venues near a location",
		Description: "Return the top N venues for a location, sorted and filtered. Useful for 'what should I order right now' style queries. Combines the discovery feed with the same sort/filter pipeline as the wolt CLI's 'top' command.",
		Annotations: readOnly,
	}, tc.handleTop)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_search_venues",
		Title:       "Search discovery venues",
		Description: "Search the non-exhaustive discovery feed by free-text query plus optional filters. Closed venues can be absent; use wolt_resolve_venue for an exact name, slug, id, or URL. Supports paging via limit/offset.",
		Annotations: readOnly,
	}, tc.handleSearchVenues)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_categories",
		Title:       "List venue categories",
		Description: "List the category slugs available at a location (e.g. 'pizza', 'sushi', 'burger'). Use the slug as the 'category' filter on other tools.",
		Annotations: readOnly,
	}, tc.handleVenueCategories)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_resolve_address",
		Title:       "Geocode an address",
		Description: "Geocode a free-form address string into lat/lon via OSM Nominatim. Useful when the user gives an address and other tools need explicit coordinates.",
		Annotations: readOnly,
	}, tc.handleResolveAddress)
}

// ---------------- wolt_feed ----------------

type FeedInput struct {
	LocationInput
	SectionLimit int    `json:"section_limit,omitempty" jsonschema:"max number of sections to return (0 = all)"`
	PerSection   int    `json:"per_section,omitempty"   jsonschema:"max venues per section (0 = all)"`
	Query        string `json:"query,omitempty"         jsonschema:"case-insensitive substring filter applied to venue name/tagline/top-offer"`
	WoltPlus     bool   `json:"wolt_plus,omitempty"     jsonschema:"only include Wolt+ venues"`
}

type FeedOutput struct {
	Summary        string         `json:"summary"`
	LocationSource string         `json:"location_source"`
	Location       LocationOut    `json:"location"`
	Data           map[string]any `json:"data"`
}

func (tc *ToolCtx) handleFeed(ctx context.Context, _ *mcp.CallToolRequest, in FeedInput) (*mcp.CallToolResult, FeedOutput, error) {
	loc, source, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, FeedOutput{}, toolErr(err)
	}
	sections, err := tc.wolt.Sections(ctx, loc)
	if err != nil {
		return nil, FeedOutput{}, toolErr(err)
	}
	var sectionLimitPtr *int
	if in.SectionLimit > 0 {
		sectionLimitPtr = &in.SectionLimit
	}
	data := observability.BuildDiscoveryFeed(sections, "", sectionLimitPtr, in.WoltPlus)
	if in.Query != "" {
		filterFeedSectionsByQuery(data, in.Query)
	}
	if in.PerSection > 0 {
		capFeedItemsPerSection(data, in.PerSection)
	}
	totalSections := len(asSlice(data["sections"]))
	return nil, FeedOutput{
		Summary:        humanCount(totalSections, "section", "sections") + " in the discovery feed",
		LocationSource: source,
		Location:       locationOut(loc),
		Data:           data,
	}, nil
}

// ---------------- wolt_top ----------------

type TopInput struct {
	LocationInput
	N        int    `json:"n,omitempty"         jsonschema:"how many venues to return (default 10)"`
	Sort     string `json:"sort,omitempty"      jsonschema:"case-insensitive sort strategy (surrounding whitespace is ignored): recommended | distance | rating | delivery_price | delivery_time | delivery-price | delivery-time | delivery | fee"`
	Query    string `json:"query,omitempty"     jsonschema:"optional substring filter on venue name/tagline/category"`
	WoltPlus bool   `json:"wolt_plus,omitempty" jsonschema:"only include Wolt+ venues"`
}

type TopOutput struct {
	Summary        string         `json:"summary"`
	LocationSource string         `json:"location_source"`
	Location       LocationOut    `json:"location"`
	Data           map[string]any `json:"data"`
	Warnings       []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleTop(ctx context.Context, _ *mcp.CallToolRequest, in TopInput) (*mcp.CallToolResult, TopOutput, error) {
	sort, err := observability.ParseVenueSort(in.Sort)
	if err != nil {
		return nil, TopOutput{}, toolErr(err)
	}
	loc, source, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, TopOutput{}, toolErr(err)
	}
	items, err := tc.wolt.Items(ctx, loc)
	if err != nil {
		return nil, TopOutput{}, toolErr(err)
	}
	n := in.N
	if n <= 0 {
		n = 10
	}
	data, warnings := observability.BuildVenueSearchResult(items, in.Query, sort, nil, "", false, in.WoltPlus, &n, 0)
	count := len(asSlice(data["items"]))
	return nil, TopOutput{
		Summary:        humanCount(count, "top venue", "top venues"),
		LocationSource: source,
		Location:       locationOut(loc),
		Data:           data,
		Warnings:       warnings,
	}, nil
}

// ---------------- wolt_search_venues ----------------

type SearchVenuesInput struct {
	LocationInput
	Query          string  `json:"query,omitempty"            jsonschema:"search query (matches name, address, tags)"`
	Sort           string  `json:"sort,omitempty"             jsonschema:"case-insensitive sort strategy (surrounding whitespace is ignored): recommended | distance | rating | delivery_price | delivery_time | delivery-price | delivery-time | delivery | fee"`
	Category       string  `json:"category,omitempty"         jsonschema:"category slug (see wolt_venue_categories)"`
	OpenNow        bool    `json:"open_now,omitempty"         jsonschema:"only currently open venues"`
	WoltPlus       bool    `json:"wolt_plus,omitempty"        jsonschema:"only Wolt+ venues"`
	MinRating      float64 `json:"min_rating,omitempty"       jsonschema:"minimum rating (e.g. 8.5)"`
	MaxDeliveryFee int     `json:"max_delivery_fee,omitempty" jsonschema:"max delivery fee in minor units (e.g. 500 = EUR 5.00)"`
	Limit          int     `json:"limit,omitempty"            jsonschema:"max rows (0 = all)"`
	Offset         int     `json:"offset,omitempty"           jsonschema:"skip first N rows"`
}

type SearchVenuesOutput struct {
	Summary        string         `json:"summary"`
	LocationSource string         `json:"location_source"`
	Location       LocationOut    `json:"location"`
	Data           map[string]any `json:"data"`
	Warnings       []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleSearchVenues(ctx context.Context, _ *mcp.CallToolRequest, in SearchVenuesInput) (*mcp.CallToolResult, SearchVenuesOutput, error) {
	sort, err := observability.ParseVenueSort(in.Sort)
	if err != nil {
		return nil, SearchVenuesOutput{}, toolErr(err)
	}
	loc, source, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, SearchVenuesOutput{}, toolErr(err)
	}
	items, err := tc.wolt.Items(ctx, loc)
	if err != nil {
		return nil, SearchVenuesOutput{}, toolErr(err)
	}
	var limitPtr *int
	if in.Limit > 0 {
		limitPtr = &in.Limit
	}
	data, warnings := observability.BuildVenueSearchResult(items, in.Query, sort, nil, in.Category, in.OpenNow, in.WoltPlus, limitPtr, in.Offset)
	if in.MinRating > 0 || in.MaxDeliveryFee > 0 {
		applyVenueRowFilters(data, in.MinRating, in.MaxDeliveryFee)
	}
	count := len(asSlice(data["items"]))
	return nil, SearchVenuesOutput{
		Summary:        humanCount(count, "venue", "venues") + " match",
		LocationSource: source,
		Location:       locationOut(loc),
		Data:           data,
		Warnings:       warnings,
	}, nil
}

// ---------------- wolt_venue_categories ----------------

type VenueCategoriesInput struct {
	LocationInput
}

type VenueCategoriesOutput struct {
	Summary        string         `json:"summary"`
	LocationSource string         `json:"location_source"`
	Location       LocationOut    `json:"location"`
	Data           map[string]any `json:"data"`
}

func (tc *ToolCtx) handleVenueCategories(ctx context.Context, _ *mcp.CallToolRequest, in VenueCategoriesInput) (*mcp.CallToolResult, VenueCategoriesOutput, error) {
	loc, source, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, VenueCategoriesOutput{}, toolErr(err)
	}
	sections, err := tc.wolt.Sections(ctx, loc)
	if err != nil {
		return nil, VenueCategoriesOutput{}, toolErr(err)
	}
	data := observability.BuildCategoryList(sections)
	count := len(asSlice(data["categories"]))
	return nil, VenueCategoriesOutput{
		Summary:        humanCount(count, "category", "categories"),
		LocationSource: source,
		Location:       locationOut(loc),
		Data:           data,
	}, nil
}

// ---------------- wolt_resolve_address ----------------

type ResolveAddressInput struct {
	Address string `json:"address" jsonschema:"free-form address string"`
}

type ResolveAddressOutput struct {
	Summary  string      `json:"summary"`
	Address  string      `json:"address"`
	Location LocationOut `json:"location"`
}

func (tc *ToolCtx) handleResolveAddress(ctx context.Context, _ *mcp.CallToolRequest, in ResolveAddressInput) (*mcp.CallToolResult, ResolveAddressOutput, error) {
	if tc.location == nil {
		return nil, ResolveAddressOutput{}, toolErrf("location resolver unavailable")
	}
	if in.Address == "" {
		return nil, ResolveAddressOutput{}, toolErrf("address is required")
	}
	loc, err := tc.location.Get(ctx, in.Address)
	if err != nil {
		return nil, ResolveAddressOutput{}, toolErr(err)
	}
	return nil, ResolveAddressOutput{
		Summary:  "geocoded successfully",
		Address:  in.Address,
		Location: locationOut(loc),
	}, nil
}
