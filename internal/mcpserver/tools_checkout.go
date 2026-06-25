package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/checkoutpayload"
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
	DeliveryMode string `json:"delivery_mode,omitempty"  jsonschema:"standard | priority | schedule"`
	Tip          int    `json:"tip,omitempty"            jsonschema:"tip in minor units (e.g. 200 = EUR 2.00)"`
	PromoCode    string `json:"promo_code,omitempty"     jsonschema:"promo code to apply"`
}

type CheckoutPreviewOutput struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

func (tc *ToolCtx) handleCheckoutPreview(ctx context.Context, _ *mcp.CallToolRequest, in CheckoutPreviewInput) (*mcp.CallToolResult, CheckoutPreviewOutput, error) {
	if strings.TrimSpace(in.Venue) == "" {
		return nil, CheckoutPreviewOutput{}, toolErrf("venue is required")
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
	basket := selectBasketForVenue(basketsPage, ref.ID)
	if basket == nil {
		return nil, CheckoutPreviewOutput{}, toolErrf("no basket found for venue %s; add items first via wolt_cart_add", ref.ID)
	}

	deliveryMode := strings.TrimSpace(in.DeliveryMode)
	if deliveryMode == "" {
		deliveryMode = "standard"
	}

	payload, _, err := checkoutpayload.Build(ctx, tc.wolt, tc.wolt.VenuePageStatic, basket, loc, deliveryMode, in.Tip, in.PromoCode)
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}

	preview, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.CheckoutPreview(ctx, payload, a)
	})
	if err != nil {
		return nil, CheckoutPreviewOutput{}, toolErr(err)
	}
	return nil, CheckoutPreviewOutput{
		Summary: "checkout preview for venue " + ref.ID,
		Data:    preview,
	}, nil
}
