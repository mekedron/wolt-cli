package cli

import (
	"context"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type testWoltAPI struct {
	refreshAccessTokenFn func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error)
	deliveryInfoListFn   func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	venuePageStaticFn    func(context.Context, string) (map[string]any, error)
	venuePageDynamicFn   func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error)
	assortmentBySlugFn   func(context.Context, string) (map[string]any, error)
	assortmentCategoryFn func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	assortmentItemsFn    func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error)
	assortmentSearchFn   func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	venueItemPageFn      func(context.Context, string, string) (map[string]any, error)
	basketsPageFn        func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error)
	addToBasketFn        func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
	deleteBasketsFn      func(context.Context, []string, woltgateway.AuthContext) (map[string]any, error)
	checkoutPreviewFn    func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
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

func (m *testWoltAPI) Search(context.Context, domain.Location, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenuePageStatic(ctx context.Context, slug string) (map[string]any, error) {
	if m.venuePageStaticFn != nil {
		return m.venuePageStaticFn(ctx, slug)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenuePageDynamic(ctx context.Context, slug string, opts woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
	if m.venuePageDynamicFn != nil {
		return m.venuePageDynamicFn(ctx, slug, opts)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentByVenueSlug(ctx context.Context, slug string) (map[string]any, error) {
	if m.assortmentBySlugFn != nil {
		return m.assortmentBySlugFn(ctx, slug)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentCategoryByVenueSlug(
	ctx context.Context,
	slug string,
	category string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.assortmentCategoryFn != nil {
		return m.assortmentCategoryFn(ctx, slug, category, language, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) AssortmentItemsByVenueSlug(ctx context.Context, slug string, itemIDs []string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.assortmentItemsFn != nil {
		return m.assortmentItemsFn(ctx, slug, itemIDs, auth)
	}
	return availableTestItems(itemIDs), nil
}

func (m *testWoltAPI) AssortmentItemsSearchByVenueSlug(
	ctx context.Context,
	slug string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.assortmentSearchFn != nil {
		return m.assortmentSearchFn(ctx, slug, query, language, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenueContentByVenueSlug(context.Context, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) VenueItemPage(ctx context.Context, venueID string, itemID string) (map[string]any, error) {
	if m.venueItemPageFn != nil {
		return m.venueItemPageFn(ctx, venueID, itemID)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) ItemBySlug(context.Context, domain.Location, string) (*domain.Item, error) {
	return nil, nil
}

func (m *testWoltAPI) UserMe(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *testWoltAPI) Subscriptions(context.Context, woltgateway.AuthContext) (map[string]any, error) {
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

func (m *testWoltAPI) BasketsPage(ctx context.Context, location domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.basketsPageFn != nil {
		return m.basketsPageFn(ctx, location, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) AddToBasket(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.addToBasketFn != nil {
		return m.addToBasketFn(ctx, payload, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) DeleteBaskets(ctx context.Context, basketIDs []string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deleteBasketsFn != nil {
		return m.deleteBasketsFn(ctx, basketIDs, auth)
	}
	return map[string]any{}, nil
}

func (m *testWoltAPI) CheckoutPreview(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.checkoutPreviewFn != nil {
		return m.checkoutPreviewFn(ctx, payload, auth)
	}
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

func availableTestItems(itemIDs []string) map[string]any {
	items := make([]any, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		items = append(items, map[string]any{
			"id":                  itemID,
			"name":                itemID,
			"purchasable_balance": 10,
		})
	}
	return map[string]any{"items": items}
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
