package mcpserver

import (
	"context"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogload"
)

// requestAssortmentItems reads the public exact-item endpoint. Some Wolt
// markets reject otherwise valid saved credentials on this endpoint while
// accepting the same request anonymously, so current catalog reads retry
// without credentials only after an actual 401. Rate limits and temporary
// failures must retain their original classification. Basket and checkout
// mutations remain authenticated.
func requestAssortmentItems(
	ctx context.Context,
	tc *ToolCtx,
	venueSlug string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	return catalogload.RequestPayload(
		ctx,
		auth,
		catalogload.RetryPolicy{},
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return tc.wolt.AssortmentItemsByVenueSlug(ctx, venueSlug, itemIDs, requestAuth)
		},
	)
}

func requestAssortmentSearch(
	ctx context.Context,
	tc *ToolCtx,
	venueSlug string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	return catalogload.RequestPayload(
		ctx,
		auth,
		catalogload.RetryPolicy{},
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return tc.wolt.AssortmentItemsSearchByVenueSlug(
				ctx,
				venueSlug,
				query,
				language,
				requestAuth,
			)
		},
	)
}
