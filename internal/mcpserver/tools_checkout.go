package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/checkoutpayload"
	"github.com/mekedron/wolt-cli/internal/service/deliveryselection"
)

func registerCheckoutTools(srv *mcp.Server, tc *ToolCtx) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_checkout_preview",
		Title:       "Preview checkout pricing",
		Description: "Preview the checkout totals for a venue's basket without placing an order. Reports subtotal, delivery fee, service fee, total. Read-only — never places an order.",
		Annotations: readOnly,
	}, tc.handleCheckoutPreview)
}

type CheckoutPreviewInput struct {
	LocationInput
	Venue        string `json:"venue"                    jsonschema:"venue slug, id, or url"`
	DeliveryMode string `json:"delivery_mode,omitempty"  jsonschema:"standard or priority; defaults to standard"`
	Tip          int    `json:"tip,omitempty"            jsonschema:"tip in minor units (e.g. 200 = 2.00 in the venue currency)"`
	PromoCode    string `json:"promo_code,omitempty"     jsonschema:"promo code to apply"`
}

type CheckoutUnavailableItem struct {
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type CheckoutPreviewError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	RetryAfterMS int    `json:"retry_after_ms,omitempty"`
}

type CheckoutPreviewOutput struct {
	Summary                string                    `json:"summary"`
	Status                 string                    `json:"status"`
	RequestedDeliveryMode  string                    `json:"requested_delivery_mode"`
	AppliedDeliveryMode    string                    `json:"applied_delivery_mode,omitempty"`
	AvailableDeliveryModes []string                  `json:"available_delivery_modes"`
	SelectedDeliveryConfig map[string]any            `json:"selected_delivery_config,omitempty"`
	Data                   map[string]any            `json:"data,omitempty"`
	Basket                 map[string]any            `json:"basket,omitempty"`
	UnavailableItems       []CheckoutUnavailableItem `json:"unavailable_items,omitempty"`
	Error                  *CheckoutPreviewError     `json:"error,omitempty"`
	Warnings               []string                  `json:"warnings,omitempty"`
}

func (tc *ToolCtx) handleCheckoutPreview(ctx context.Context, _ *mcp.CallToolRequest, in CheckoutPreviewInput) (*mcp.CallToolResult, CheckoutPreviewOutput, error) {
	if strings.TrimSpace(in.Venue) == "" {
		return nil, CheckoutPreviewOutput{}, toolErrf("venue is required")
	}
	if in.Tip < 0 {
		return nil, CheckoutPreviewOutput{}, toolErrf("tip must be zero or greater")
	}
	deliveryMode, err := normalizeCheckoutDeliveryMode(in.DeliveryMode)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	ref, err := tc.resolveVenueRef(ctx, in.Venue)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	loc, _, err := tc.resolveLocation(ctx, in.LocationInput)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}

	// Re-derive items + currency from the user's existing basket — Wolt's
	// checkout-preview endpoint requires a snapshot of what to price out.
	basketsPage, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.BasketsPage(ctx, loc, a)
	})
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	basket, err := selectVerifiedBasketForVenue(basketsPage, ref)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	if basket == nil {
		return nil, CheckoutPreviewOutput{}, toolErrf("no basket found for venue %s; add items first via wolt_cart_add", ref.ID)
	}
	basketIdentity := basketVenueIdentity(basket)
	basketVenue := asMap(basket["venue"])
	if basketVenue == nil {
		basketVenue = map[string]any{}
		basket["venue"] = basketVenue
	}
	venueID := firstNonEmpty(basketIdentity.ID, ref.ID)
	venueSlug := firstNonEmpty(basketIdentity.Slug, ref.Slug)
	if !looksLikeObjectID(venueID) || venueSlug == "" {
		return nil, CheckoutPreviewOutput{}, toolErrf(
			"checkout preview blocked because the canonical venue id and slug could not both be resolved",
		)
	}
	basketVenue["id"] = venueID
	basketVenue["slug"] = venueSlug
	basketItemIDs := catalogitem.BasketItemIDs(basket)
	currentItems, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return requestAssortmentItems(ctx, tc, venueSlug, basketItemIDs, a)
	})
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(fmt.Errorf("current basket availability lookup failed: %w", err))
	}
	issues := catalogitem.ValidateItemIDs(currentItems, basketItemIDs)
	if len(issues) > 0 {
		unavailable := checkoutUnavailableItems(issues, basket)
		summary := "checkout preview blocked: " + catalogitem.FormatValidationIssues(issues)
		return checkoutErrorResult(summary), CheckoutPreviewOutput{
			Summary:                summary,
			Status:                 "blocked",
			RequestedDeliveryMode:  deliveryMode,
			AvailableDeliveryModes: []string{"standard"},
			Basket:                 basket,
			UnavailableItems:       unavailable,
			Error: &CheckoutPreviewError{
				Code:      "UNAVAILABLE_ITEMS",
				Message:   "Remove or replace the unavailable items before retrying checkout preview.",
				Retryable: false,
			},
		}, nil
	}

	payload, buildWarnings, err := checkoutpayload.Build(ctx, tc.wolt, tc.wolt.VenuePageStatic, basket, loc, deliveryMode, in.Tip, in.PromoCode)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}

	preview, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.CheckoutPreview(ctx, payload, a)
	})
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	deliveryState := deliveryselection.Parse(preview)
	modeUnconfirmed := deliveryState.SelectionAmbiguous ||
		deliveryState.SelectedMode != deliveryMode ||
		deliveryState.SelectedConfig == nil
	if modeUnconfirmed {
		summary := fmt.Sprintf(
			"%s delivery was requested but Wolt did not confirm that it was applied",
			deliveryMode,
		)
		return checkoutErrorResult(summary), CheckoutPreviewOutput{
			Summary:                summary,
			Status:                 "delivery_mode_unavailable",
			RequestedDeliveryMode:  deliveryMode,
			AvailableDeliveryModes: deliveryState.AvailableModes,
			SelectedDeliveryConfig: deliveryState.SelectedConfig,
			Data:                   preview,
			Error: &CheckoutPreviewError{
				Code:      "DELIVERY_MODE_UNAVAILABLE",
				Message:   fmt.Sprintf("The checkout response did not select %s delivery.", deliveryMode),
				Retryable: false,
			},
			Warnings: buildWarnings,
		}, nil
	}
	appliedMode := deliveryMode
	if deliveryState.SelectedMode != "" {
		appliedMode = deliveryState.SelectedMode
	}
	return nil, CheckoutPreviewOutput{
		Summary:                "checkout preview for venue " + venueID,
		Status:                 "ready",
		RequestedDeliveryMode:  deliveryMode,
		AppliedDeliveryMode:    appliedMode,
		AvailableDeliveryModes: deliveryState.AvailableModes,
		SelectedDeliveryConfig: deliveryState.SelectedConfig,
		Data:                   preview,
		Warnings:               buildWarnings,
	}, nil
}

func normalizeCheckoutDeliveryMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "standard", nil
	}
	if mode != "standard" && mode != "priority" {
		return "", fmt.Errorf("unsupported delivery_mode %q; allowed values: standard, priority", raw)
	}
	return mode, nil
}

func checkoutErrorResult(summary string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: summary}},
		IsError: true,
	}
}

func checkoutUnavailableItems(issues []catalogitem.ValidationIssue, basket map[string]any) []CheckoutUnavailableItem {
	basketNames := map[string]string{}
	for _, raw := range asSlice(basket["items"]) {
		line := asMap(raw)
		itemID := strings.TrimSpace(asString(line["id"]))
		name := strings.TrimSpace(asString(coalesceAny(line["name"], line["title"])))
		if itemID != "" && name != "" {
			basketNames[itemID] = name
		}
	}
	out := make([]CheckoutUnavailableItem, 0, len(issues))
	for _, issue := range issues {
		name := strings.TrimSpace(issue.Name)
		if fallback := basketNames[issue.ItemID]; fallback != "" && (name == "" || name == issue.ItemID) {
			name = fallback
		}
		if name == "" {
			name = issue.ItemID
		}
		out = append(out, CheckoutUnavailableItem{
			ItemID: issue.ItemID,
			Name:   name,
			Reason: issue.Reason,
		})
	}
	return out
}
