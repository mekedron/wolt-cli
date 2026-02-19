package wolt

import (
	"context"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
)

// API describes all Wolt upstream operations used by the CLI.
type API interface {
	FrontPage(ctx context.Context, location domain.Location) (map[string]any, error)
	Sections(ctx context.Context, location domain.Location) ([]domain.Section, error)
	Items(ctx context.Context, location domain.Location) ([]domain.Item, error)
	RestaurantByID(ctx context.Context, venueID string) (*domain.Restaurant, error)
	Search(ctx context.Context, location domain.Location, query string) (map[string]any, error)
	VenuePageStatic(ctx context.Context, slug string) (map[string]any, error)
	VenuePageDynamic(ctx context.Context, slug string) (map[string]any, error)
	VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error)
	ItemBySlug(ctx context.Context, location domain.Location, slug string) (*domain.Item, error)
}
