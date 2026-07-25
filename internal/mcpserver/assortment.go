package mcpserver

import (
	"context"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// requestAssortmentItems reads the public exact-item endpoint. Some Wolt
// markets reject otherwise valid saved credentials on this endpoint while
// accepting the same request anonymously, so current catalog reads retry
// without credentials. Basket and checkout mutations remain authenticated.
func requestAssortmentItems(
	ctx context.Context,
	tc *ToolCtx,
	venueSlug string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	payload, err := tc.wolt.AssortmentItemsByVenueSlug(ctx, venueSlug, itemIDs, auth)
	if err == nil || !auth.HasCredentials() {
		return payload, err
	}
	return tc.wolt.AssortmentItemsByVenueSlug(
		ctx,
		venueSlug,
		itemIDs,
		woltgateway.AuthContext{},
	)
}
