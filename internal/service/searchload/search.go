// Package searchload coordinates global item-search requests shared by the
// CLI and MCP surfaces.
package searchload

import (
	"context"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogload"
)

// API is the gateway surface required by global item search.
type API interface {
	SearchItems(
		ctx context.Context,
		location domain.Location,
		query string,
		limit int,
		auth woltgateway.AuthContext,
	) (map[string]any, error)
}

// RequestItems performs a global item search. Saved credentials are forwarded
// when available so Wolt can apply account context; an unauthorized optional
// session falls back to the public endpoint.
func RequestItems(
	ctx context.Context,
	api API,
	location domain.Location,
	query string,
	limit int,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	return catalogload.RequestPayload(
		ctx,
		auth,
		catalogload.RetryPolicy{},
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return api.SearchItems(ctx, location, query, limit, requestAuth)
		},
	)
}
