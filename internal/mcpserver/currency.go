package mcpserver

import (
	"context"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func resolveVenueCurrency(
	ctx context.Context,
	tc *ToolCtx,
	ref venueRef,
	basket map[string]any,
	payloads ...map[string]any,
) string {
	if currency := payloadutil.CurrencyFromBasket(basket); currency != "" {
		return currency
	}
	for _, payload := range payloads {
		for _, candidate := range []any{
			payload["currency"],
			asMap(payload["price"])["currency"],
			asMap(payload["base_price"])["currency"],
		} {
			if currency := payloadutil.NormalizeCurrency(asString(candidate)); currency != "" {
				return currency
			}
		}
		if currency := payloadutil.CurrencyFromVenuePayload(payload); currency != "" {
			return currency
		}
	}
	slug := strings.TrimSpace(ref.Slug)
	if slug == "" || tc == nil || tc.wolt == nil {
		return ""
	}
	if payload, err := tc.wolt.VenuePageStatic(ctx, slug); err == nil {
		if currency := payloadutil.CurrencyFromVenuePayload(payload); currency != "" {
			return currency
		}
	}
	if payload, err := tc.wolt.VenuePageDynamic(ctx, slug, woltgateway.VenuePageDynamicOptions{}); err == nil {
		return payloadutil.CurrencyFromVenuePayload(payload)
	}
	return ""
}
