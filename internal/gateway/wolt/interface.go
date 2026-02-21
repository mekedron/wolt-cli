package wolt

import (
	"context"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// AuthContext stores optional auth credentials for upstream calls.
type AuthContext struct {
	WToken       string
	RefreshToken string
	Cookies      []string
}

// HasCredentials reports whether at least one auth method is provided.
func (a AuthContext) HasCredentials() bool {
	if strings.TrimSpace(a.WToken) != "" {
		return true
	}
	return len(a.Cookies) > 0
}

// API describes all Wolt upstream operations used by the CLI.
type API interface {
	FrontPage(ctx context.Context, location domain.Location) (map[string]any, error)
	Sections(ctx context.Context, location domain.Location) ([]domain.Section, error)
	Items(ctx context.Context, location domain.Location) ([]domain.Item, error)
	RestaurantByID(ctx context.Context, venueID string) (*domain.Restaurant, error)
	Search(ctx context.Context, location domain.Location, query string) (map[string]any, error)
	VenuePageStatic(ctx context.Context, slug string) (map[string]any, error)
	VenuePageDynamic(ctx context.Context, slug string, options VenuePageDynamicOptions) (map[string]any, error)
	AssortmentByVenueSlug(ctx context.Context, slug string) (map[string]any, error)
	AssortmentCategoryByVenueSlug(ctx context.Context, slug string, categorySlug string, language string, auth AuthContext) (map[string]any, error)
	AssortmentItemsByVenueSlug(ctx context.Context, slug string, itemIDs []string, auth AuthContext) (map[string]any, error)
	AssortmentItemsSearchByVenueSlug(ctx context.Context, slug string, query string, language string, auth AuthContext) (map[string]any, error)
	VenueContentByVenueSlug(ctx context.Context, slug string, nextPageToken string, auth AuthContext) (map[string]any, error)
	VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error)
	ItemBySlug(ctx context.Context, location domain.Location, slug string) (*domain.Item, error)
	UserMe(ctx context.Context, auth AuthContext) (map[string]any, error)
	PaymentMethods(ctx context.Context, auth AuthContext) (map[string]any, error)
	PaymentMethodsProfile(ctx context.Context, auth AuthContext, options PaymentMethodsProfileOptions) (map[string]any, error)
	AddressFields(ctx context.Context, location domain.Location, language string, auth AuthContext) (map[string]any, error)
	DeliveryInfoList(ctx context.Context, auth AuthContext) (map[string]any, error)
	DeliveryInfoCreate(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error)
	DeliveryInfoDelete(ctx context.Context, addressID string, auth AuthContext) (map[string]any, error)
	OrderHistory(ctx context.Context, auth AuthContext, options OrderHistoryOptions) (map[string]any, error)
	OrderHistoryPurchase(ctx context.Context, purchaseID string, auth AuthContext) (map[string]any, error)
	FavoriteVenues(ctx context.Context, location domain.Location, auth AuthContext) (map[string]any, error)
	FavoriteVenueAdd(ctx context.Context, venueID string, auth AuthContext) (map[string]any, error)
	FavoriteVenueRemove(ctx context.Context, venueID string, auth AuthContext) (map[string]any, error)
	BasketCount(ctx context.Context, auth AuthContext) (map[string]any, error)
	BasketsPage(ctx context.Context, location domain.Location, auth AuthContext) (map[string]any, error)
	AddToBasket(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error)
	DeleteBaskets(ctx context.Context, basketIDs []string, auth AuthContext) (map[string]any, error)
	CheckoutPreview(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error)
	RefreshAccessToken(ctx context.Context, refreshToken string, auth AuthContext) (TokenRefreshResult, error)
}

// VenuePageDynamicOptions controls optional request context for dynamic venue page calls.
type VenuePageDynamicOptions struct {
	Location               *domain.Location
	SelectedDeliveryMethod string
	Auth                   AuthContext
}

// PaymentMethodsProfileOptions controls payment profile endpoint query params.
type PaymentMethodsProfileOptions struct {
	Country          string
	AvailableMethods []string
	IsFTU            bool
}

// TokenRefreshResult stores rotated access/refresh credentials.
type TokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// OrderHistoryOptions controls order-history endpoint query params.
type OrderHistoryOptions struct {
	Limit     int
	PageToken string
}
