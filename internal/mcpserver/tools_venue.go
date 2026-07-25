package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
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
		Name:        "wolt_venue_menu",
		Title:       "Venue menu",
		Description: "Browse a venue's menu. Returns normalized items with prices, image URLs, discounts, and current availability. Supports query/category filters and pagination.",
		Annotations: readOnly,
	}, tc.handleVenueMenu)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_venue_hours",
		Title:       "Venue opening hours",
		Description: "Return opening windows for a venue, in the venue's timezone (overridable).",
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
	Include string `json:"include,omitempty" jsonschema:"comma-separated extras: hours,tags,rating,fees (default: all)"`
}

type VenueDetailOutput struct {
	Summary  string         `json:"summary"`
	Venue    map[string]any `json:"venue"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleVenueDetail(ctx context.Context, _ *mcp.CallToolRequest, in VenueDetailInput) (*mcp.CallToolResult, VenueDetailOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueDetailOutput{}, toolErr(err)
	}
	includes := parseIncludeSet(in.Include, []string{"hours", "tags", "rating", "fees"})

	// Need both a discovery-feed item (for tagline/badges) and the restaurant
	// document; the location feeds ItemBySlug. Falls through to profile address
	// if the caller omits lat/lon/address.
	location, _, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, VenueDetailOutput{}, toolErr(fmt.Errorf("venue_detail needs a location: %w", err))
	}
	item, err := tc.wolt.ItemBySlug(ctx, location, firstNonEmpty(ref.Slug, ref.ID))
	if err != nil || item == nil {
		return nil, VenueDetailOutput{}, toolErr(fmt.Errorf("venue not found at this location: %s", in.Venue))
	}
	restaurant, err := tc.wolt.RestaurantByID(ctx, ref.ID)
	if err != nil || restaurant == nil {
		return nil, VenueDetailOutput{}, toolErr(fmt.Errorf("venue restaurant payload unavailable: %v", err))
	}
	data, warnings, buildErr := observability.BuildVenueDetail(item, restaurant, includes)
	if buildErr != nil {
		return nil, VenueDetailOutput{}, toolErr(buildErr)
	}
	return nil, VenueDetailOutput{
		Summary:  fmt.Sprintf("venue %s (%s)", asString(data["name"]), asString(data["slug"])),
		Venue:    data,
		Warnings: warnings,
	}, nil
}

// ---------------- wolt_venue_menu ----------------

type VenueMenuInput struct {
	Venue    string `json:"venue"               jsonschema:"venue slug, id, or url"`
	Query    string `json:"query,omitempty"     jsonschema:"case-insensitive substring filter on item name"`
	Category string `json:"category,omitempty"  jsonschema:"category name filter"`
	Limit    int    `json:"limit,omitempty"     jsonschema:"max items"`
	Offset   int    `json:"offset,omitempty"    jsonschema:"skip first N"`
}

type VenueMenuOutput struct {
	Summary  string         `json:"summary"`
	Data     map[string]any `json:"data"`
	Warnings []string       `json:"warnings,omitempty"`
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
	data, warnings := observability.BuildVenueMenu(ref.ID, []map[string]any{payload}, in.Category, false, nil)

	if in.Query != "" {
		filterMenuItemsByName(data, in.Query)
	}
	rows := asSlice(data["items"])
	if in.Offset > 0 {
		if in.Offset >= len(rows) {
			rows = []any{}
		} else {
			rows = rows[in.Offset:]
		}
	}
	if in.Limit > 0 && in.Limit < len(rows) {
		rows = rows[:in.Limit]
	}
	data["items"] = rows
	return nil, VenueMenuOutput{
		Summary:  humanCount(len(rows), "item", "items"),
		Data:     data,
		Warnings: warnings,
	}, nil
}

// ---------------- wolt_venue_hours ----------------

type VenueHoursInput struct {
	Venue    string `json:"venue"             jsonschema:"venue slug, id, or url"`
	Timezone string `json:"timezone,omitempty" jsonschema:"timezone override (defaults to venue timezone)"`
}

type VenueHoursOutput struct {
	Summary string         `json:"summary"`
	Hours   map[string]any `json:"hours"`
}

func (tc *ToolCtx) handleVenueHours(ctx context.Context, _ *mcp.CallToolRequest, in VenueHoursInput) (*mcp.CallToolResult, VenueHoursOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueHoursOutput{}, toolErr(err)
	}
	restaurant, err := tc.wolt.RestaurantByID(ctx, ref.ID)
	if err != nil || restaurant == nil {
		return nil, VenueHoursOutput{}, toolErr(fmt.Errorf("venue not found: %s", in.Venue))
	}
	data := observability.BuildVenueHours(restaurant, in.Timezone)
	return nil, VenueHoursOutput{
		Summary: fmt.Sprintf("opening windows for %s", asString(data["venue_id"])),
		Hours:   data,
	}, nil
}

// ---------------- wolt_venue_item ----------------

type VenueItemInput struct {
	Venue  string `json:"venue"   jsonschema:"venue slug, id, or url"`
	ItemID string `json:"item_id" jsonschema:"24-char item id (e.g. from wolt_venue_menu)"`
}

type VenueItemOutput struct {
	Summary string         `json:"summary"`
	Item    map[string]any `json:"item"`
}

func (tc *ToolCtx) handleVenueItem(ctx context.Context, _ *mcp.CallToolRequest, in VenueItemInput) (*mcp.CallToolResult, VenueItemOutput, error) {
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, VenueItemOutput{}, toolErr(err)
	}
	if strings.TrimSpace(in.ItemID) == "" {
		return nil, VenueItemOutput{}, toolErrf("item_id is required")
	}
	payload, pageErr := tc.wolt.VenueItemPage(ctx, ref.ID, in.ItemID)
	var currentItem map[string]any
	if strings.TrimSpace(ref.Slug) != "" {
		currentPayload, currentErr := tc.wolt.AssortmentItemsByVenueSlug(
			ctx,
			ref.Slug,
			[]string{in.ItemID},
			tc.optionalAuth(ctx),
		)
		if currentErr == nil {
			currentItem = catalogitem.Find(currentPayload, in.ItemID)
		}
	}
	if currentItem != nil {
		payload = catalogitem.MergeCurrentItem(payload, currentItem)
	} else if pageErr != nil {
		return nil, VenueItemOutput{}, toolErr(pageErr)
	}
	if currency := resolveVenueCurrency(ctx, tc, ref, nil, payload); currency != "" {
		payload["currency"] = currency
	}
	data, _ := observability.BuildItemDetail(in.ItemID, ref.ID, payload, false)
	return nil, VenueItemOutput{
		Summary: fmt.Sprintf("item %s", asString(data["name"])),
		Item:    data,
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
	payload, err := tc.wolt.AssortmentItemsSearchByVenueSlug(ctx, firstNonEmpty(ref.Slug, in.Venue), in.Query, language, tc.optionalAuth(ctx))
	if err != nil {
		return nil, VenueSearchItemsOutput{}, toolErr(err)
	}
	var limitPtr *int
	if in.Limit > 0 {
		limitPtr = &in.Limit
	}
	data, warnings := observability.BuildItemSearchResult(in.Query, []map[string]any{payload}, observability.ItemSortName, "", limitPtr, in.Offset, nil)
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

func filterMenuItemsByName(data map[string]any, needle string) {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return
	}
	rows := asSlice(data["items"])
	filtered := make([]any, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		if row == nil {
			continue
		}
		if strings.Contains(strings.ToLower(asString(row["name"])), needle) {
			filtered = append(filtered, raw)
		}
	}
	data["items"] = filtered
}
