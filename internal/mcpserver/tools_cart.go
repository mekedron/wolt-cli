package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func registerCartTools(srv *mcp.Server, tc *ToolCtx) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	destructiveFalse := false
	mutateAdd := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructiveFalse, IdempotentHint: false}
	destructiveTrue := true
	mutateRemove := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructiveTrue, IdempotentHint: false}
	mutateClear := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructiveTrue, IdempotentHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_cart_show",
		Title:       "Show baskets",
		Description: "Return the user's current baskets (one per venue) with item lines, prices, and totals. Baskets are always returned; order_availability is included when venue resolution and availability lookup succeed.",
		Annotations: readOnly,
	}, tc.handleCartShow)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_cart_count",
		Title:       "Total items in baskets",
		Description: "Return the total number of items across all the user's baskets. Cheap one-call status check.",
		Annotations: readOnly,
	}, tc.handleCartCount)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_cart_add",
		Title:       "Add an item to a basket",
		Description: "Add an item after revalidating its current availability. Merges with existing basket items (does not replace them) and resolves current price and venue currency.",
		Annotations: mutateAdd,
	}, tc.handleCartAdd)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_cart_remove",
		Title:       "Remove an item from a basket",
		Description: "Remove a single item line from a venue's basket. Count defaults to removing all of that item; pass count to remove a specific quantity.",
		Annotations: mutateRemove,
	}, tc.handleCartRemove)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_cart_clear",
		Title:       "Clear all baskets",
		Description: "Delete every basket the user has. Irreversible — confirm with the user before calling.",
		Annotations: mutateClear,
	}, tc.handleCartClear)
}

// ---------------- wolt_cart_show ----------------

type CartShowInput struct {
	LocationInput
	Venue string `json:"venue,omitempty" jsonschema:"optional venue slug, id, or URL; omit to return every basket"`
}
type CartShowOutput struct {
	Summary  string         `json:"summary"`
	Data     map[string]any `json:"data"`
	Filter   map[string]any `json:"filter,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleCartShow(ctx context.Context, _ *mcp.CallToolRequest, in CartShowInput) (*mcp.CallToolResult, CartShowOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CartShowOutput{}, toolErr(err)
	}
	loc, _, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, CartShowOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketsPage(ctx, loc, a)
	})
	if err != nil {
		return nil, CartShowOutput{}, toolErr(err)
	}
	var filter map[string]any
	if strings.TrimSpace(in.Venue) != "" {
		ref, resolveErr := tc.resolveVenueRef(ctx, in.Venue)
		if resolveErr != nil {
			return nil, CartShowOutput{}, toolErr(resolveErr)
		}
		payload, err = filterBasketPage(payload, ref, in.Venue)
		if err != nil {
			return nil, CartShowOutput{}, toolErr(err)
		}
		filter = map[string]any{
			"input":    in.Venue,
			"venue_id": ref.ID,
			"slug":     ref.Slug,
		}
	}
	warnings := tc.enrichBasketAvailability(ctx, payload, loc)
	count := len(payloadutil.BasketRows(payload))
	return nil, CartShowOutput{
		Summary:  humanCount(count, "basket", "baskets"),
		Data:     payload,
		Filter:   filter,
		Warnings: warnings,
	}, nil
}

// ---------------- wolt_cart_count ----------------

type CartCountInput struct{}
type CartCountOutput struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

func (tc *ToolCtx) handleCartCount(ctx context.Context, _ *mcp.CallToolRequest, _ CartCountInput) (*mcp.CallToolResult, CartCountOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CartCountOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketCount(ctx, a)
	})
	if err != nil {
		return nil, CartCountOutput{}, toolErr(err)
	}
	total := asInt(coalesceAny(payload["count"], payload["total"]))
	return nil, CartCountOutput{
		Summary: humanCount(total, "item", "items") + " across all baskets",
		Data:    payload,
	}, nil
}

// ---------------- wolt_cart_add ----------------

type CartAddInput struct {
	LocationInput
	Venue              string `json:"venue"               jsonschema:"venue slug, id, or url"`
	ItemID             string `json:"item_id"             jsonschema:"24-char item id (see wolt_venue_menu)"`
	Count              int    `json:"count,omitempty"     jsonschema:"how many to add (default 1)"`
	Price              int    `json:"price,omitempty"     jsonschema:"item price in minor units (auto-detected from menu if omitted)"`
	Currency           string `json:"currency,omitempty"  jsonschema:"3-letter currency (auto-detected if omitted)"`
	Name               string `json:"name,omitempty"      jsonschema:"item display name (auto-detected if omitted)"`
	AllowSubstitutions bool   `json:"allow_substitutions,omitempty" jsonschema:"if true, the venue may substitute equivalent items"`
}
type CartAddOutput struct {
	Summary           string         `json:"summary"`
	VenueID           string         `json:"venue_id"`
	ItemID            string         `json:"item_id"`
	Added             int            `json:"added"`
	OrderAvailability map[string]any `json:"order_availability,omitempty"`
	Response          map[string]any `json:"response,omitempty"`
	Warnings          []string       `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleCartAdd(ctx context.Context, _ *mcp.CallToolRequest, in CartAddInput) (*mcp.CallToolResult, CartAddOutput, error) {
	if strings.TrimSpace(in.Venue) == "" || strings.TrimSpace(in.ItemID) == "" {
		return nil, CartAddOutput{}, toolErrf("venue and item_id are required")
	}
	if in.Count < 0 {
		return nil, CartAddOutput{}, toolErrf("count must be zero or greater")
	}
	count := in.Count
	if count <= 0 {
		count = 1
	}
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}
	loc, _, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}

	tc.cartMutationMu.Lock()
	defer tc.cartMutationMu.Unlock()

	// Fetch the current basket before validating the mutation ID. A selected
	// basket remains an authoritative source when the public venue resolver is
	// temporarily unavailable, and AddToBasket replaces its items wholesale.
	existingPage, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketsPage(ctx, loc, a)
	})
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}
	existingBasket, err := selectVerifiedBasketForVenue(existingPage, ref)
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}
	basketIdentity := basketVenueIdentity(existingBasket)
	venueMutationID := firstNonEmpty(basketIdentity.ID, ref.ID)
	if !looksLikeObjectID(venueMutationID) {
		return nil, CartAddOutput{}, toolErrf(
			"could not verify a canonical Wolt venue id for %q, so the item was NOT added; pass the 24-character venue id or a wolt.com venue URL",
			in.Venue,
		)
	}
	venueSlug := firstNonEmpty(ref.Slug, basketIdentity.Slug)
	if venueSlug == "" {
		return nil, CartAddOutput{}, toolErrf(
			"current item availability could not be verified for venue %s; pass a venue slug or Wolt venue/item URL",
			venueMutationID,
		)
	}
	ref.ID = venueMutationID
	ref.Slug = venueSlug

	currentAssortment, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return requestAssortmentItems(ctx, tc, venueSlug, []string{in.ItemID}, a)
	})
	if err != nil {
		return nil, CartAddOutput{}, toolErr(fmt.Errorf("current item availability lookup failed: %w", err))
	}
	issues := catalogitem.ValidateItemIDs(currentAssortment, []string{in.ItemID})
	if len(issues) > 0 {
		return nil, CartAddOutput{}, toolErrf(
			"item was NOT added because current availability validation failed: %s",
			catalogitem.FormatValidationIssues(issues),
		)
	}
	currentItem := catalogitem.ScopedItem(currentAssortment, in.ItemID)

	// Availability is always checked above, even when callers provide all
	// display metadata explicitly. Resolve missing values from the fresh
	// assortment item first and use the item page only as a fallback.
	price := in.Price
	currency := payloadutil.NormalizeCurrency(in.Currency)
	name := strings.TrimSpace(in.Name)
	itemPayload := currentItem
	if price <= 0 {
		price = asInt(currentItem["price"])
		if price <= 0 {
			price = asInt(asMap(currentItem["price"])["amount"])
		}
	}
	if name == "" {
		name = asString(coalesceAny(currentItem["name"], currentItem["title"]))
	}
	if price <= 0 || currency == "" || name == "" {
		if fetched, itemErr := tc.wolt.VenueItemPage(ctx, venueMutationID, in.ItemID); itemErr == nil {
			itemPayload = catalogitem.MergeCurrentItem(fetched, currentItem)
			if price <= 0 {
				price = asInt(asMap(itemPayload["price"])["amount"])
			}
			if currency == "" {
				currency = payloadutil.NormalizeCurrency(asString(asMap(itemPayload["price"])["currency"]))
			}
			if name == "" {
				name = asString(coalesceAny(itemPayload["name"], itemPayload["title"]))
			}
		}
	}
	for _, spec := range payloadutil.ExtractOptionSpecs(itemPayload) {
		minSelect := spec.MinSelect
		if spec.Required && minSelect < 1 {
			minSelect = 1
		}
		if minSelect > 0 {
			return nil, CartAddOutput{}, toolErrf(
				"item was NOT added because it requires option selections, which wolt_cart_add does not accept",
			)
		}
	}
	if price <= 0 {
		return nil, CartAddOutput{}, toolErrf("could not determine item price; pass `price` in minor units")
	}
	orderAvailability, availabilityWarnings := tc.cartVenueAvailability(ctx, ref, loc)

	newLine := map[string]any{
		"id":      in.ItemID,
		"count":   count,
		"name":    name,
		"price":   price,
		"options": []any{},
		"substitution_settings": map[string]any{
			"is_allowed": in.AllowSubstitutions,
		},
	}

	mergedItems := []any{newLine}
	if weightConfig, weighted := payloadutil.WeightConfigFromItem(itemPayload); weighted {
		mergedItems, err = payloadutil.MergeWeightedBasketItems(existingBasket, in.ItemID, count, newLine, weightConfig)
	} else if existingBasket != nil {
		mergedItems, err = payloadutil.MergeBasketItems(existingBasket, in.ItemID, count, newLine)
	}
	if err != nil {
		return nil, CartAddOutput{}, toolErrf("item was NOT added: %v", err)
	}
	if currency == "" {
		currency = resolveVenueCurrency(ctx, tc, ref, existingBasket, currentAssortment, itemPayload)
	}
	if currency == "" {
		return nil, CartAddOutput{}, toolErrf(
			"item was NOT added because the venue currency could not be verified",
		)
	}

	addPayload := map[string]any{
		"items":    mergedItems,
		"venue_id": venueMutationID,
		"currency": currency,
	}
	resp, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.AddToBasket(ctx, addPayload, a)
	})
	if err != nil {
		return nil, CartAddOutput{}, toolErr(err)
	}
	summary := fmt.Sprintf("added %d × %s to %s basket", count, firstNonEmpty(name, in.ItemID), venueMutationID)
	if asBool(orderAvailability["scheduled_only"]) {
		summary += "; basket is available for scheduled order only"
	}
	return nil, CartAddOutput{
		Summary:           summary,
		VenueID:           venueMutationID,
		ItemID:            in.ItemID,
		Added:             count,
		OrderAvailability: orderAvailability,
		Response:          resp,
		Warnings:          availabilityWarnings,
	}, nil
}

// ---------------- wolt_cart_remove ----------------

type CartRemoveInput struct {
	LocationInput
	Venue  string `json:"venue"   jsonschema:"venue slug, id, or url"`
	ItemID string `json:"item_id" jsonschema:"item id to remove"`
	Count  int    `json:"count,omitempty" jsonschema:"how many to remove (default: all of that item)"`
}
type CartRemoveOutput struct {
	Summary  string         `json:"summary"`
	Removed  int            `json:"removed"`
	Response map[string]any `json:"response,omitempty"`
}

func (tc *ToolCtx) handleCartRemove(ctx context.Context, _ *mcp.CallToolRequest, in CartRemoveInput) (*mcp.CallToolResult, CartRemoveOutput, error) {
	if strings.TrimSpace(in.Venue) == "" || strings.TrimSpace(in.ItemID) == "" {
		return nil, CartRemoveOutput{}, toolErrf("venue and item_id are required")
	}
	if in.Count < 0 {
		return nil, CartRemoveOutput{}, toolErrf("count must be zero or greater")
	}
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}
	loc, _, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}

	tc.cartMutationMu.Lock()
	defer tc.cartMutationMu.Unlock()

	existingPage, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketsPage(ctx, loc, a)
	})
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}
	basket, err := selectVerifiedBasketForVenue(existingPage, ref)
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}
	if basket == nil {
		return nil, CartRemoveOutput{}, toolErrf("no basket found for venue %s", ref.ID)
	}
	remainingItems, removed, err := payloadutil.RemoveBasketItems(basket, in.ItemID, in.Count)
	if err != nil {
		return nil, CartRemoveOutput{}, toolErrf("item quantity was not changed: %v", err)
	}
	if removed == 0 {
		return nil, CartRemoveOutput{}, toolErrf("item %s not in basket for venue %s", in.ItemID, ref.ID)
	}

	if len(remainingItems) == 0 {
		// Empty cart → delete the basket entirely.
		basketID := payloadutil.BasketID(basket)
		if basketID == "" {
			return nil, CartRemoveOutput{}, toolErrf("internal: could not locate basket id")
		}
		resp, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
			return tc.wolt.DeleteBaskets(ctx, []string{basketID}, a)
		})
		if err != nil {
			return nil, CartRemoveOutput{}, toolErr(err)
		}
		return nil, CartRemoveOutput{
			Summary:  fmt.Sprintf("removed %d × %s (basket now empty, deleted)", removed, in.ItemID),
			Removed:  removed,
			Response: resp,
		}, nil
	}

	venueID := firstNonEmpty(basketVenueIdentity(basket).ID, ref.ID)
	if !looksLikeObjectID(venueID) {
		return nil, CartRemoveOutput{}, toolErrf(
			"item quantity was not changed because the basket venue id could not be verified",
		)
	}
	currency := resolveVenueCurrency(ctx, tc, ref, basket)
	if currency == "" {
		return nil, CartRemoveOutput{}, toolErrf(
			"item quantity was not changed because the venue currency could not be verified",
		)
	}
	payload := map[string]any{
		"items":    remainingItems,
		"venue_id": venueID,
		"currency": currency,
	}
	resp, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.AddToBasket(ctx, payload, a)
	})
	if err != nil {
		return nil, CartRemoveOutput{}, toolErr(err)
	}
	return nil, CartRemoveOutput{
		Summary:  fmt.Sprintf("removed %d × %s from basket", removed, in.ItemID),
		Removed:  removed,
		Response: resp,
	}, nil
}

// ---------------- wolt_cart_clear ----------------

type CartClearInput struct{}
type CartClearOutput struct {
	Summary string `json:"summary"`
	Deleted int    `json:"deleted"`
}

func (tc *ToolCtx) handleCartClear(ctx context.Context, _ *mcp.CallToolRequest, _ CartClearInput) (*mcp.CallToolResult, CartClearOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CartClearOutput{}, toolErr(err)
	}
	// Use zero-location BasketsPage just to enumerate baskets; the lat/lon is
	// only used by the upstream for delivery-fee calc.
	loc := lookupProfileLocation(ctx, tc)
	tc.cartMutationMu.Lock()
	defer tc.cartMutationMu.Unlock()

	page, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketsPage(ctx, loc, a)
	})
	if err != nil {
		return nil, CartClearOutput{}, toolErr(err)
	}
	if !payloadutil.BasketIDsComplete(page) {
		return nil, CartClearOutput{}, toolErrf(
			"not all basket ids could be resolved; no baskets were cleared",
		)
	}
	baskets := payloadutil.BasketRows(page)
	if len(baskets) == 0 {
		return nil, CartClearOutput{Summary: "no baskets to clear", Deleted: 0}, nil
	}
	ids := payloadutil.BasketIDs(page)
	if len(ids) == 0 {
		return nil, CartClearOutput{}, toolErrf(
			"basket ids are unavailable; no baskets were cleared",
		)
	}
	if _, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.DeleteBaskets(ctx, ids, a)
	}); err != nil {
		return nil, CartClearOutput{}, toolErr(err)
	}
	return nil, CartClearOutput{
		Summary: fmt.Sprintf("deleted %d basket(s)", len(ids)),
		Deleted: len(ids),
	}, nil
}

// ---------------- cart helpers ----------------

func selectBasketForVenue(baskets []map[string]any, ref venueRef) map[string]any {
	for _, basket := range baskets {
		if basketSharesVenueReference(basket, ref, ref.Input) {
			return basket
		}
	}
	return nil
}

func selectVerifiedBasketForVenue(page map[string]any, ref venueRef) (map[string]any, error) {
	baskets, err := payloadutil.BasketRowsForMutation(page)
	if err != nil {
		return nil, fmt.Errorf("basket page cannot be used safely: %w", err)
	}
	for index, basket := range baskets {
		identity := basketVenueIdentity(basket)
		if identity.ID == "" && identity.Slug == "" {
			return nil, fmt.Errorf(
				"basket page cannot be used safely: basket at index %d has no venue identity",
				index,
			)
		}
	}
	basket := selectBasketForVenue(baskets, ref)
	if err := verifyBasketVenueIdentity(ref, basketVenueIdentity(basket)); err != nil {
		return nil, err
	}
	return basket, nil
}

func filterBasketPage(page map[string]any, ref venueRef, rawInput string) (map[string]any, error) {
	if page == nil {
		return map[string]any{"baskets": []any{}}, nil
	}
	out := make(map[string]any, len(page))
	for key, value := range page {
		out[key] = value
	}
	foundContainer := false
	for _, sourceKey := range []string{"baskets", "results"} {
		if _, present := page[sourceKey]; !present {
			continue
		}
		foundContainer = true
		filtered := make([]any, 0, 1)
		for _, rawBasket := range asSlice(page[sourceKey]) {
			basket := asMap(rawBasket)
			if !basketSharesVenueReference(basket, ref, rawInput) {
				continue
			}
			if err := verifyBasketVenueIdentity(ref, basketVenueIdentity(basket)); err != nil {
				return nil, err
			}
			filtered = append(filtered, basket)
		}
		out[sourceKey] = filtered
	}
	if !foundContainer {
		out["baskets"] = []any{}
	}
	return out, nil
}

func basketSharesVenueReference(basket map[string]any, ref venueRef, rawInput string) bool {
	if basket == nil {
		return false
	}
	identity := basketVenueIdentity(basket)
	input := normalizeVenueInput(rawInput)
	for _, expected := range []string{ref.ID, ref.Slug, input} {
		expected = strings.TrimSpace(expected)
		if expected == "" {
			continue
		}
		for _, actual := range []string{identity.ID, identity.Slug} {
			if strings.EqualFold(expected, strings.TrimSpace(actual)) {
				return true
			}
		}
	}
	return false
}

func verifyBasketVenueIdentity(ref venueRef, identity venueRef) error {
	if identity.Conflict {
		return fmt.Errorf(
			"basket contains conflicting canonical venue ids; no basket data was changed",
		)
	}
	if looksLikeObjectID(ref.ID) &&
		looksLikeObjectID(identity.ID) &&
		!strings.EqualFold(strings.TrimSpace(ref.ID), strings.TrimSpace(identity.ID)) {
		return fmt.Errorf(
			"basket venue identity conflicts with the resolved venue id; no basket data was changed",
		)
	}
	return nil
}

func basketVenueIdentity(basket map[string]any) venueRef {
	if basket == nil {
		return venueRef{}
	}
	identity := payloadutil.ExtractBasketVenueIdentity(basket)
	venue := asMap(basket["venue"])
	return venueRef{
		ID:       identity.ID,
		Conflict: identity.Conflict,
		Slug: strings.TrimSpace(firstNonEmpty(
			identity.Slug,
			normalizeVenueInput(asString(coalesceAny(venue["public_url"], venue["url"]))),
		)),
	}
}

func (tc *ToolCtx) enrichBasketAvailability(
	ctx context.Context,
	page map[string]any,
	location domain.Location,
) []string {
	warnings := []string{}
	for _, basket := range payloadutil.BasketRows(page) {
		ref := basketVenueIdentity(basket)
		if ref.Slug == "" && ref.ID != "" {
			if resolved, err := tc.resolveVenueRef(ctx, ref.ID); err == nil {
				ref = resolved
			}
		}
		availability, availabilityWarnings := tc.cartVenueAvailability(ctx, ref, location)
		if availability != nil {
			basket["order_availability"] = availability
		}
		for _, warning := range availabilityWarnings {
			warnings = append(warnings, fmt.Sprintf(
				"basket %s: %s",
				firstNonEmpty(payloadutil.BasketID(basket), ref.ID, ref.Slug, "unknown"),
				warning,
			))
		}
	}
	return warnings
}

func (tc *ToolCtx) cartVenueAvailability(
	ctx context.Context,
	ref venueRef,
	location domain.Location,
) (map[string]any, []string) {
	if tc.wolt == nil || strings.TrimSpace(ref.Slug) == "" {
		return nil, []string{"venue order availability could not be resolved"}
	}
	dynamicPayload, err := tc.requestVenuePageDynamic(ctx, ref.Slug, woltgateway.VenuePageDynamicOptions{
		Location:               &location,
		SelectedDeliveryMethod: "homedelivery",
		Auth:                   tc.optionalAuth(ctx),
	})
	if err != nil {
		return nil, []string{"venue order availability could not be loaded"}
	}
	return observability.BuildVenueAvailability(ref.StaticPayload, dynamicPayload, nil, &location), nil
}

// lookupProfileLocation returns the saved profile location if any, else zero.
// Good enough for BasketsPage, which only uses lat/lon for delivery-fee calc.
func lookupProfileLocation(ctx context.Context, tc *ToolCtx) domain.Location {
	profile, err := tc.loadProfile(ctx)
	if err != nil {
		return domain.Location{}
	}
	return profile.Location
}
