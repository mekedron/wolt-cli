package cli

import (
	"context"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type testWoltAPI struct {
	refreshAccessTokenFn func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error)
	deliveryInfoListFn   func(context.Context, woltgateway.AuthContext) (map[string]any, error)
}

func (m *testWoltAPI) FrontPage(context.Context, domain.Location) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) Sections(context.Context, domain.Location) ([]domain.Section, error) {
	return nil, nil
}

func (m *testWoltAPI) Items(context.Context, domain.Location) ([]domain.Item, error) {
	return nil, nil
}

func (m *testWoltAPI) RestaurantByID(context.Context, string) (*domain.Restaurant, error) {
	return nil, nil
}

func (m *testWoltAPI) Search(context.Context, domain.Location, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenuePageStatic(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenuePageDynamic(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentByVenueSlug(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentCategoryByVenueSlug(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentItemsByVenueSlug(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentItemsSearchByVenueSlug(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenueContentByVenueSlug(context.Context, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenueItemPage(context.Context, string, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) ItemBySlug(context.Context, domain.Location, string) (*domain.Item, error) {
	return nil, nil
}

func (m *testWoltAPI) UserMe(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) PaymentMethods(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) PaymentMethodsProfile(context.Context, woltgateway.AuthContext, woltgateway.PaymentMethodsProfileOptions) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AddressFields(context.Context, domain.Location, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) DeliveryInfoList(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deliveryInfoListFn != nil {
		return m.deliveryInfoListFn(ctx, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) DeliveryInfoCreate(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) DeliveryInfoDelete(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) OrderHistory(context.Context, woltgateway.AuthContext, woltgateway.OrderHistoryOptions) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) OrderHistoryPurchase(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) FavoriteVenues(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) FavoriteVenueAdd(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) FavoriteVenueRemove(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) BasketCount(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) BasketsPage(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) AddToBasket(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) DeleteBaskets(context.Context, []string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) CheckoutPreview(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) RefreshAccessToken(ctx context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
	if m.refreshAccessTokenFn != nil {
		return m.refreshAccessTokenFn(ctx, refreshToken, auth)
	}
	return woltgateway.TokenRefreshResult{}, nil
}

type testProfiles struct {
	profile domain.Profile
}

func (m *testProfiles) Find(context.Context, string) (domain.Profile, error) {
	return m.profile, nil
}

type testConfigManager struct {
	cfg domain.Config
}

func (m *testConfigManager) Path() string {
	return "/tmp/test-config.json"
}

func (m *testConfigManager) Load(context.Context) (domain.Config, error) {
	return m.cfg, nil
}

func (m *testConfigManager) Save(_ context.Context, cfg domain.Config) error {
	m.cfg = cfg
	return nil
}
