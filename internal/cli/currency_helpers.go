package cli

import (
	"context"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func currencyFromItemPayload(payload map[string]any) string {
	for _, candidate := range []any{
		payload["currency"],
		asMap(payload["price"])["currency"],
		asMap(payload["base_price"])["currency"],
	} {
		if currency := payloadutil.NormalizeCurrency(asString(candidate)); currency != "" {
			return currency
		}
	}
	return ""
}

func currencyFromVenue(ctx context.Context, deps Dependencies, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" || deps.Wolt == nil {
		return ""
	}
	if payload, err := cachedVenuePageStatic(ctx, deps, slug); err == nil {
		if currency := payloadutil.CurrencyFromVenuePayload(payload); currency != "" {
			return currency
		}
	}
	if payload, err := deps.Wolt.VenuePageDynamic(ctx, slug, woltgateway.VenuePageDynamicOptions{}); err == nil {
		return payloadutil.CurrencyFromVenuePayload(payload)
	}
	return ""
}
