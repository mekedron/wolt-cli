package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type mockWolt struct {
	frontPageFunc           func(context.Context, domain.Location) (map[string]any, error)
	sectionsFunc            func(context.Context, domain.Location) ([]domain.Section, error)
	itemsFunc               func(context.Context, domain.Location) ([]domain.Item, error)
	restaurantByIDFunc      func(context.Context, string) (*domain.Restaurant, error)
	searchFunc              func(context.Context, domain.Location, string) (map[string]any, error)
	venuePageStaticFunc     func(context.Context, string) (map[string]any, error)
	venuePageDynamicFunc    func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error)
	assortmentBySlugFunc    func(context.Context, string) (map[string]any, error)
	assortmentCategoryFn    func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	assortmentItemsFn       func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error)
	assortmentItemsSearchFn func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	venueContentBySlugFn    func(context.Context, string, string, woltgateway.AuthContext) (map[string]any, error)
	venueItemPageFunc       func(context.Context, string, string) (map[string]any, error)
	itemBySlugFunc          func(context.Context, domain.Location, string) (*domain.Item, error)
	userMeFunc              func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	paymentMethodsFunc      func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	paymentProfileFunc      func(context.Context, woltgateway.AuthContext, woltgateway.PaymentMethodsProfileOptions) (map[string]any, error)
	addressFieldsFunc       func(context.Context, domain.Location, string, woltgateway.AuthContext) (map[string]any, error)
	deliveryInfoListFunc    func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	deliveryInfoCreateFn    func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
	deliveryInfoDeleteFn    func(context.Context, string, woltgateway.AuthContext) (map[string]any, error)
	orderHistoryFunc        func(context.Context, woltgateway.AuthContext, woltgateway.OrderHistoryOptions) (map[string]any, error)
	orderHistoryShowFn      func(context.Context, string, woltgateway.AuthContext) (map[string]any, error)
	favoriteVenuesFunc      func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error)
	favoriteVenueAddFn      func(context.Context, string, woltgateway.AuthContext) (map[string]any, error)
	favoriteVenueRemFn      func(context.Context, string, woltgateway.AuthContext) (map[string]any, error)
	basketCountFunc         func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	basketsPageFunc         func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error)
	addToBasketFunc         func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
	deleteBasketsFunc       func(context.Context, []string, woltgateway.AuthContext) (map[string]any, error)
	checkoutPreviewFunc     func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
	refreshAccessTokenFn    func(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error)
}

func (m *mockWolt) FrontPage(ctx context.Context, location domain.Location) (map[string]any, error) {
	if m.frontPageFunc == nil {
		return nil, errors.New("front page not mocked")
	}
	return m.frontPageFunc(ctx, location)
}

func (m *mockWolt) Sections(ctx context.Context, location domain.Location) ([]domain.Section, error) {
	if m.sectionsFunc == nil {
		return nil, errors.New("sections not mocked")
	}
	return m.sectionsFunc(ctx, location)
}

func (m *mockWolt) Items(ctx context.Context, location domain.Location) ([]domain.Item, error) {
	if m.itemsFunc == nil {
		return nil, errors.New("items not mocked")
	}
	return m.itemsFunc(ctx, location)
}

func (m *mockWolt) RestaurantByID(ctx context.Context, venueID string) (*domain.Restaurant, error) {
	if m.restaurantByIDFunc == nil {
		return nil, errors.New("restaurantByID not mocked")
	}
	return m.restaurantByIDFunc(ctx, venueID)
}

func (m *mockWolt) Search(ctx context.Context, location domain.Location, query string) (map[string]any, error) {
	if m.searchFunc == nil {
		return nil, errors.New("search not mocked")
	}
	return m.searchFunc(ctx, location, query)
}

func (m *mockWolt) VenuePageStatic(ctx context.Context, slug string) (map[string]any, error) {
	if m.venuePageStaticFunc == nil {
		return nil, errors.New("venue page static not mocked")
	}
	return m.venuePageStaticFunc(ctx, slug)
}

func (m *mockWolt) VenuePageDynamic(ctx context.Context, slug string, options woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
	if m.venuePageDynamicFunc == nil {
		return nil, errors.New("venue page dynamic not mocked")
	}
	return m.venuePageDynamicFunc(ctx, slug, options)
}

func (m *mockWolt) AssortmentByVenueSlug(ctx context.Context, slug string) (map[string]any, error) {
	if m.assortmentBySlugFunc == nil {
		return nil, errors.New("assortment by venue slug not mocked")
	}
	return m.assortmentBySlugFunc(ctx, slug)
}

func (m *mockWolt) AssortmentCategoryByVenueSlug(
	ctx context.Context,
	slug string,
	categorySlug string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.assortmentCategoryFn == nil {
		return nil, errors.New("assortment category by venue slug not mocked")
	}
	return m.assortmentCategoryFn(ctx, slug, categorySlug, language, auth)
}

func (m *mockWolt) AssortmentItemsByVenueSlug(
	ctx context.Context,
	slug string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.assortmentItemsFn == nil {
		return nil, errors.New("assortment items by venue slug not mocked")
	}
	return m.assortmentItemsFn(ctx, slug, itemIDs, auth)
}

func (m *mockWolt) AssortmentItemsSearchByVenueSlug(
	ctx context.Context,
	slug string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.assortmentItemsSearchFn == nil {
		return nil, errors.New("assortment items search by venue slug not mocked")
	}
	return m.assortmentItemsSearchFn(ctx, slug, query, language, auth)
}

func (m *mockWolt) VenueContentByVenueSlug(
	ctx context.Context,
	slug string,
	nextPageToken string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.venueContentBySlugFn == nil {
		return nil, errors.New("venue content by venue slug not mocked")
	}
	return m.venueContentBySlugFn(ctx, slug, nextPageToken, auth)
}

func (m *mockWolt) VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error) {
	if m.venueItemPageFunc == nil {
		return nil, errors.New("venue item page not mocked")
	}
	return m.venueItemPageFunc(ctx, venueID, itemID)
}

func (m *mockWolt) ItemBySlug(ctx context.Context, location domain.Location, slug string) (*domain.Item, error) {
	if m.itemBySlugFunc == nil {
		return nil, errors.New("item by slug not mocked")
	}
	return m.itemBySlugFunc(ctx, location, slug)
}

func (m *mockWolt) UserMe(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.userMeFunc == nil {
		return nil, errors.New("user me not mocked")
	}
	return m.userMeFunc(ctx, auth)
}

func (m *mockWolt) PaymentMethods(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.paymentMethodsFunc == nil {
		return nil, errors.New("payment methods not mocked")
	}
	return m.paymentMethodsFunc(ctx, auth)
}

func (m *mockWolt) PaymentMethodsProfile(
	ctx context.Context,
	auth woltgateway.AuthContext,
	options woltgateway.PaymentMethodsProfileOptions,
) (map[string]any, error) {
	if m.paymentProfileFunc == nil {
		return map[string]any{}, nil
	}
	return m.paymentProfileFunc(ctx, auth, options)
}

func (m *mockWolt) AddressFields(
	ctx context.Context,
	location domain.Location,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.addressFieldsFunc == nil {
		return nil, errors.New("address fields not mocked")
	}
	return m.addressFieldsFunc(ctx, location, language, auth)
}

func (m *mockWolt) DeliveryInfoList(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deliveryInfoListFunc == nil {
		return nil, errors.New("delivery info list not mocked")
	}
	return m.deliveryInfoListFunc(ctx, auth)
}

func (m *mockWolt) DeliveryInfoCreate(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deliveryInfoCreateFn == nil {
		return nil, errors.New("delivery info create not mocked")
	}
	return m.deliveryInfoCreateFn(ctx, payload, auth)
}

func (m *mockWolt) DeliveryInfoDelete(ctx context.Context, addressID string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deliveryInfoDeleteFn == nil {
		return nil, errors.New("delivery info delete not mocked")
	}
	return m.deliveryInfoDeleteFn(ctx, addressID, auth)
}

func (m *mockWolt) OrderHistory(
	ctx context.Context,
	auth woltgateway.AuthContext,
	options woltgateway.OrderHistoryOptions,
) (map[string]any, error) {
	if m.orderHistoryFunc == nil {
		return nil, errors.New("order history not mocked")
	}
	return m.orderHistoryFunc(ctx, auth, options)
}

func (m *mockWolt) OrderHistoryPurchase(
	ctx context.Context,
	purchaseID string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	if m.orderHistoryShowFn == nil {
		return nil, errors.New("order history purchase not mocked")
	}
	return m.orderHistoryShowFn(ctx, purchaseID, auth)
}

func (m *mockWolt) FavoriteVenues(ctx context.Context, location domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.favoriteVenuesFunc == nil {
		return nil, errors.New("favorite venues not mocked")
	}
	return m.favoriteVenuesFunc(ctx, location, auth)
}

func (m *mockWolt) FavoriteVenueAdd(ctx context.Context, venueID string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.favoriteVenueAddFn == nil {
		return nil, errors.New("favorite venue add not mocked")
	}
	return m.favoriteVenueAddFn(ctx, venueID, auth)
}

func (m *mockWolt) FavoriteVenueRemove(ctx context.Context, venueID string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.favoriteVenueRemFn == nil {
		return nil, errors.New("favorite venue remove not mocked")
	}
	return m.favoriteVenueRemFn(ctx, venueID, auth)
}

func (m *mockWolt) BasketCount(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.basketCountFunc == nil {
		return nil, errors.New("basket count not mocked")
	}
	return m.basketCountFunc(ctx, auth)
}

func (m *mockWolt) BasketsPage(ctx context.Context, location domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.basketsPageFunc == nil {
		return nil, errors.New("baskets page not mocked")
	}
	return m.basketsPageFunc(ctx, location, auth)
}

func (m *mockWolt) AddToBasket(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.addToBasketFunc == nil {
		return nil, errors.New("add to basket not mocked")
	}
	return m.addToBasketFunc(ctx, payload, auth)
}

func (m *mockWolt) DeleteBaskets(ctx context.Context, basketIDs []string, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.deleteBasketsFunc == nil {
		return nil, errors.New("delete baskets not mocked")
	}
	return m.deleteBasketsFunc(ctx, basketIDs, auth)
}

func (m *mockWolt) CheckoutPreview(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if m.checkoutPreviewFunc == nil {
		return nil, errors.New("checkout preview not mocked")
	}
	return m.checkoutPreviewFunc(ctx, payload, auth)
}

func (m *mockWolt) RefreshAccessToken(
	ctx context.Context,
	refreshToken string,
	auth woltgateway.AuthContext,
) (woltgateway.TokenRefreshResult, error) {
	if m.refreshAccessTokenFn == nil {
		return woltgateway.TokenRefreshResult{}, errors.New("refresh access token not mocked")
	}
	return m.refreshAccessTokenFn(ctx, refreshToken, auth)
}

type mockProfiles struct {
	profile domain.Profile
	err     error
}

func (m *mockProfiles) Find(_ context.Context, _ string) (domain.Profile, error) {
	if m.err != nil {
		return domain.Profile{}, m.err
	}
	return m.profile, nil
}

type mockLocation struct{}

func (m *mockLocation) Get(_ context.Context, _ string) (domain.Location, error) {
	return domain.Location{Lat: 50, Lon: 19}, nil
}

type mockConfig struct{}

func (m *mockConfig) Path() string {
	return "/tmp/config.json"
}

func (m *mockConfig) Load(context.Context) (domain.Config, error) {
	return domain.Config{}, errors.New("not found")
}

func (m *mockConfig) Save(context.Context, domain.Config) error {
	return nil
}

func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	defaultProfile := domain.Profile{
		Name:      "default",
		IsDefault: true,
		Location:  domain.Location{Lat: 0, Lon: 0},
	}

	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: defaultProfile},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
	return runCLIWithDeps(t, deps, args...)
}

func runCLIWithDeps(t *testing.T, deps cli.Dependencies, args ...string) (int, string) {
	t.Helper()
	ensureDefaultLocationLookupAuth(args, deps.Profiles)
	if woltMock, ok := deps.Wolt.(*mockWolt); ok {
		ensureDefaultDeliveryInfoList(woltMock, deps.Profiles)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Execute(context.Background(), args, deps, &stdout, &stderr)
	return exitCode, stdout.String() + stderr.String()
}

func ensureDefaultLocationLookupAuth(args []string, profiles cli.ProfileResolver) {
	if len(args) == 0 {
		return
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "discover", "search", "venue", "item":
	default:
		return
	}
	profileMock, ok := profiles.(*mockProfiles)
	if !ok {
		return
	}
	if strings.TrimSpace(profileMock.profile.WToken) != "" || len(profileMock.profile.Cookies) > 0 {
		return
	}
	profileMock.profile.WToken = "test-token"
}

func ensureDefaultDeliveryInfoList(woltMock *mockWolt, profiles cli.ProfileResolver) {
	if woltMock == nil || woltMock.deliveryInfoListFunc != nil {
		return
	}
	profileMock, ok := profiles.(*mockProfiles)
	if !ok {
		return
	}
	location := profileMock.profile.Location
	if location.Lat == 0 && location.Lon == 0 {
		location = domain.Location{Lat: 60.1699, Lon: 24.9384}
	}
	woltMock.deliveryInfoListFunc = func(_ context.Context, _ woltgateway.AuthContext) (map[string]any, error) {
		return map[string]any{
			"results": []any{
				map[string]any{
					"id": "default-address",
					"location": map[string]any{
						"user_coordinates": map[string]any{
							"type":        "Point",
							"coordinates": []any{location.Lon, location.Lat},
						},
					},
				},
			},
		}, nil
	}
}

func intPtr(v int) *int {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func mustJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, raw)
	}
	return payload
}

func buildVenue(id, slug, address string) *domain.Venue {
	return &domain.Venue{
		ID:               id,
		Slug:             slug,
		Name:             "Venue",
		Address:          address,
		Country:          "POL",
		Currency:         "PLN",
		Delivers:         true,
		DeliveryPriceInt: intPtr(1000),
		EstimateRange:    "25-35",
		Estimate:         30,
		Online:           boolPtr(true),
		ProductLine:      "restaurant",
		ShowWoltPlus:     true,
		Tags:             []string{"burger"},
		Rating:           &domain.Rating{Rating: 3, Score: 9.1},
		PriceRange:       2,
		Promotions:       []any{map[string]any{"text": "Free delivery", "variant": "discount"}},
	}
}

func TestRootHelpIncludesCommandDescriptions(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0},
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
	exitCode, out := runCLIWithDeps(t, deps, "--help")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	for _, expected := range []string{
		"full reference:",
		"commands:",
		"Read discovery feed and browse categories.",
		"Search venues and menu items by query.",
		"Inspect venue details, menus, and opening hours.",
		"discover feed",
		"search venues",
		"item show",
		"--include-upsell",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected output to contain %q\noutput:\n%s", expected, out)
		}
	}
	for _, notExpected := range []string{"╭", "╰"} {
		if strings.Contains(out, notExpected) {
			t.Fatalf("did not expect output to contain %q\noutput:\n%s", notExpected, out)
		}
	}
	for _, token := range []string{
		"--format: Output format: table, json, or yaml.",
		"--profile: Profile name for saved local defaults.",
		"--address: Temporary address override for this command. Geocoded to coordinates. Cannot be combined with --lat/--lon.",
		"--locale: Response locale in BCP-47 format, for example en-FI.",
		"--no-color: Disable ANSI color codes in table output.",
		"--wrtoken: Wolt refresh token for automatic access token rotation (or payload with refreshToken).",
		"--verbose: Enable verbose output (prints upstream request trace and detailed error diagnostics).",
	} {
		if count := strings.Count(out, token); count != 1 {
			t.Fatalf("expected %q to appear once in root help, got %d\noutput:\n%s", token, count, out)
		}
	}
}

func TestDiscoverWithoutSubcommandShowsHelp(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0},
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Show discovery feed sections and venues.") {
		t.Fatalf("expected discover help output, got:\n%s", out)
	}
	if !strings.Contains(out, "List available discovery categories.") {
		t.Fatalf("expected discover categories help output, got:\n%s", out)
	}
}

func TestVersion(t *testing.T) {
	exitCode, out := runCLI(t, "--version")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if out != "1.1.1\n" {
		t.Fatalf("expected version output 1.1.1, got %q", out)
	}
}

func TestDiscoverFeedJSON(t *testing.T) {
	venue := buildVenue("venue-1", "venue-one", "Street 1")
	section := domain.Section{
		Name:  "popular",
		Title: "Popular",
		Items: []domain.Item{{Title: "Venue One", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: venue}},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return map[string]any{"city_data": map[string]any{"name": "Krakow"}}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return []domain.Section{section}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--lat", "50.0", "--lon", "19.0", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["city"] != "Krakow" {
		t.Fatalf("expected city Krakow, got %v", data["city"])
	}
	sections := asSlicePayload(t, data["sections"])
	firstSection := asMapPayload(t, sections[0])
	items := asSlicePayload(t, firstSection["items"])
	firstItem := asMapPayload(t, items[0])
	if firstItem["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", firstItem["venue_id"])
	}
	deliveryFee := asMapPayload(t, firstItem["delivery_fee"])
	if deliveryFee["formatted_amount"] != "PLN 10.00" {
		t.Fatalf("expected formatted fee PLN 10.00, got %v", deliveryFee["formatted_amount"])
	}
	if firstItem["price_range"] != float64(2) {
		t.Fatalf("expected price_range 2, got %v", firstItem["price_range"])
	}
	if firstItem["price_range_scale"] != "$$" {
		t.Fatalf("expected price_range_scale $$, got %v", firstItem["price_range_scale"])
	}
	promotions := asSlicePayload(t, firstItem["promotions"])
	if len(promotions) != 1 || promotions[0] != "Free delivery" {
		t.Fatalf("expected promotions [Free delivery], got %v", promotions)
	}
	if firstItem["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true, got %v", firstItem["wolt_plus"])
	}
}

func TestDiscoverFeedUsesAccountAddressCoordinates(t *testing.T) {
	profile := domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}, WToken: "token"}
	venue := buildVenue("venue-1", "venue-one", "Street 1")
	section := domain.Section{Name: "popular", Title: "Popular", Items: []domain.Item{{Title: "Venue One", TrackID: "x", Link: domain.Link{Target: "venue-1"}, Venue: venue}}}
	seen := domain.Location{}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(_ context.Context, location domain.Location) (map[string]any, error) {
				seen = location
				return map[string]any{"city_data": map[string]any{"name": "Krakow"}}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return []domain.Section{section}, nil
			},
		},
		Profiles: &mockProfiles{profile: profile},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seen.Lat != profile.Location.Lat || seen.Lon != profile.Location.Lon {
		t.Fatalf("expected account address location %+v, got %+v", profile.Location, seen)
	}
}

func TestDiscoverFeedRequiresLatAndLonTogether(t *testing.T) {
	exitCode, out := runCLI(t, "discover", "feed", "--lat", "50.0", "--format", "json")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	errPayload := asMapPayload(t, payload["error"])
	if errPayload["code"] != "WOLT_INVALID_ARGUMENT" {
		t.Fatalf("expected WOLT_INVALID_ARGUMENT, got %v", errPayload["code"])
	}
	if !strings.Contains(strings.ToLower(asStringPayload(errPayload["message"])), "both --lat and --lon") {
		t.Fatalf("expected lat/lon validation message, got %v", errPayload["message"])
	}
}

func TestDiscoverFeedRejectsAddressWithLatLon(t *testing.T) {
	exitCode, out := runCLI(t, "discover", "feed", "--address", "Helsinki", "--lat", "50.0", "--lon", "19.0", "--format", "json")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	errPayload := asMapPayload(t, payload["error"])
	if errPayload["code"] != "WOLT_INVALID_ARGUMENT" {
		t.Fatalf("expected WOLT_INVALID_ARGUMENT, got %v", errPayload["code"])
	}
	if !strings.Contains(strings.ToLower(asStringPayload(errPayload["message"])), "do not combine --address") {
		t.Fatalf("expected address/lat/lon conflict message, got %v", errPayload["message"])
	}
}

func TestDiscoverFeedUsesAddressOverride(t *testing.T) {
	seenLocation := domain.Location{}
	locationResolver := &recordingLocation{
		location: domain.Location{Lat: 60.1699, Lon: 24.9384},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(_ context.Context, location domain.Location) (map[string]any, error) {
				seenLocation = location
				return map[string]any{"city_data": map[string]any{"name": "Helsinki"}}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return []domain.Section{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: locationResolver,
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--address", "Kamppi, Helsinki", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if locationResolver.seenAddress != "Kamppi, Helsinki" {
		t.Fatalf("expected geocoding with requested address, got %q", locationResolver.seenAddress)
	}
	if seenLocation.Lat != 60.1699 || seenLocation.Lon != 24.9384 {
		t.Fatalf("expected geocoded location to be used, got %+v", seenLocation)
	}
}

func TestSearchVenuesJSON(t *testing.T) {
	matching := domain.Item{Title: "Burger Place", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Burger Street")}
	nonMatching := domain.Item{Title: "Sushi Place", TrackID: "track-2", Link: domain.Link{Target: "venue-2"}, Venue: buildVenue("venue-2", "sushi-place", "Sushi Street")}
	nonMatching.Venue.ShowWoltPlus = false
	nonMatching.Venue.Tags = []string{"sushi"}
	nonMatching.Venue.Rating = &domain.Rating{Rating: 3, Score: 8.4}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return []domain.Item{matching, nonMatching}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "search", "venues", "--query", "burger", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["query"] != "burger" {
		t.Fatalf("expected query burger, got %v", data["query"])
	}
	if asIntPayload(data["total"]) != 1 {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	items := asSlicePayload(t, data["items"])
	if asMapPayload(t, items[0])["slug"] != "burger-place" {
		t.Fatalf("expected slug burger-place, got %v", asMapPayload(t, items[0])["slug"])
	}
}

func TestSearchItemsFallbackJSON(t *testing.T) {
	fallbackItem := domain.Item{Title: "Whopper Meal", TrackID: "item-track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Street")}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return []domain.Item{fallbackItem}, nil
			},
			searchFunc: func(context.Context, domain.Location, string) (map[string]any, error) {
				return nil, woltgateway.ErrUpstream
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "search", "items", "--query", "whopper", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["total"]) != 1 {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	items := asSlicePayload(t, data["items"])
	if asMapPayload(t, items[0])["item_id"] != "item-track-1" {
		t.Fatalf("expected fallback item id item-track-1, got %v", asMapPayload(t, items[0])["item_id"])
	}
	warnings := asSlicePayload(t, payload["warnings"])
	combined := strings.ToLower(strings.Join(sliceToStrings(warnings), " "))
	if !strings.Contains(combined, "fallback") {
		t.Fatalf("expected fallback warning, got %v", warnings)
	}
}

func TestVenueShowJSON(t *testing.T) {
	venueItem := &domain.Item{Title: "Burger Place", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Burger Street")}
	restaurant := &domain.Restaurant{
		ID:                    "venue-1",
		Slug:                  "burger-place",
		Name:                  []domain.Translation{{Lang: "en", Value: "Burger Place"}},
		Address:               "Street 1",
		City:                  "Krakow",
		Country:               "POL",
		Currency:              "PLN",
		FoodTags:              []string{"burger"},
		PriceRange:            2,
		PublicURL:             "https://wolt.com/test",
		AllowedPaymentMethods: []string{"card"},
		Description:           []domain.Translation{{Lang: "en", Value: "Description"}},
		DeliveryMethods:       []string{"homedelivery"},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return venueItem, nil
			},
			restaurantByIDFunc: func(context.Context, string) (*domain.Restaurant, error) {
				return restaurant, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "show", "burger-place", "--include", "tags", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	if data["slug"] != "burger-place" {
		t.Fatalf("expected slug burger-place, got %v", data["slug"])
	}
	tags := asSlicePayload(t, data["tags"])
	if len(tags) != 1 || tags[0] != "burger" {
		t.Fatalf("expected tags [burger], got %v", tags)
	}
}

func TestVenueShowFallbackStaticWhenItemLookupFails(t *testing.T) {
	restaurant := &domain.Restaurant{
		ID:              "venue-1",
		Slug:            "burger-place",
		Address:         "Street 1",
		Currency:        "PLN",
		DeliveryMethods: []string{"homedelivery"},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 404}
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"venue": map[string]any{
						"id":             "venue-1",
						"slug":           "burger-place",
						"name":           "Burger Place",
						"address":        "Street 1",
						"currency":       "PLN",
						"delivery_price": 500,
					},
				}, nil
			},
			restaurantByIDFunc: func(context.Context, string) (*domain.Restaurant, error) {
				return restaurant, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "show", "burger-place", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	if data["slug"] != "burger-place" {
		t.Fatalf("expected slug burger-place, got %v", data["slug"])
	}
}

func TestVenueShowFallbackWhenRestaurantEndpointGone(t *testing.T) {
	venueItem := &domain.Item{
		Title: "Burger Place",
		Link:  domain.Link{Target: "venue-1"},
		Venue: buildVenue("venue-1", "burger-place", "Street 1"),
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return venueItem, nil
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"venue": map[string]any{
						"id":       "venue-1",
						"slug":     "burger-place",
						"name":     "Burger Place",
						"address":  "Street 1",
						"currency": "PLN",
					},
				}, nil
			},
			restaurantByIDFunc: func(context.Context, string) (*domain.Restaurant, error) {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 410}
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "show", "burger-place", "--include", "fees", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	if asMapPayload(t, data["delivery_fee"])["amount"] == nil {
		t.Fatalf("expected delivery fee in fallback output, got %v", data["delivery_fee"])
	}
}

func TestItemShowJSON(t *testing.T) {
	venueItem := &domain.Item{Title: "Burger Place", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Street")}
	itemPayload := map[string]any{
		"item_id":       "item-1",
		"name":          "Whopper Meal",
		"description":   "Burger with fries",
		"price":         map[string]any{"amount": 1595, "currency": "PLN"},
		"option_groups": []any{map[string]any{"id": "group-1", "name": "Choose drink", "required": true, "min": 1, "max": 1}},
		"upsell_items":  []any{map[string]any{"item_id": "item-2", "name": "Nuggets", "price": map[string]any{"amount": 745, "currency": "PLN"}}},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return venueItem, nil
			},
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return itemPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "item", "show", "burger-place", "item-1", "--include-upsell", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["item_id"] != "item-1" {
		t.Fatalf("expected item_id item-1, got %v", data["item_id"])
	}
	if data["name"] != "Whopper Meal" {
		t.Fatalf("expected name Whopper Meal, got %v", data["name"])
	}
	upsell := asSlicePayload(t, data["upsell_items"])
	if len(upsell) != 1 {
		t.Fatalf("expected 1 upsell item, got %d", len(upsell))
	}
}

func TestDiscoverFeedJSONReturnsErrorEnvelope(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     "GET",
					URL:        "https://consumer-api.wolt.com/v1/pages/front?lat=50&lon=19",
					StatusCode: 401,
					Body:       `{"error":"Unauthorized"}`,
				}
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--lat", "50.0", "--lon", "19.0", "--format", "json")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	if payload["data"] != nil {
		t.Fatalf("expected data nil, got %v", payload["data"])
	}
	errPayload := asMapPayload(t, payload["error"])
	if errPayload["code"] != "WOLT_UPSTREAM_ERROR" {
		t.Fatalf("expected WOLT_UPSTREAM_ERROR, got %v", errPayload["code"])
	}
	if !strings.Contains(strings.ToLower(asStringPayload(errPayload["message"])), "wolt") {
		t.Fatalf("expected upstream message, got %v", errPayload["message"])
	}
	if strings.Contains(asStringPayload(errPayload["message"]), "consumer-api.wolt.com") {
		t.Fatalf("expected non-verbose error to hide request URL, got %v", errPayload["message"])
	}
}

func TestDiscoverFeedVerboseJSONIncludesUpstreamDiagnostics(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     "GET",
					URL:        "https://consumer-api.wolt.com/v1/pages/front?lat=50&lon=19",
					StatusCode: 401,
					Body:       `{"error":"Unauthorized"}`,
				}
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--lat", "50.0", "--lon", "19.0", "--format", "json", "--verbose")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	errPayload := asMapPayload(t, payload["error"])
	message := asStringPayload(errPayload["message"])
	for _, expected := range []string{"status=401", "consumer-api.wolt.com", "Unauthorized"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected verbose error to contain %q, got %v", expected, message)
		}
	}
}

func asMapPayload(t *testing.T, value any) map[string]any {
	t.Helper()
	payload, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", value)
	}
	return payload
}

func asSlicePayload(t *testing.T, value any) []any {
	t.Helper()
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		t.Fatalf("expected slice payload, got %T", value)
		return nil
	}
}

func asStringPayload(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func asIntPayload(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func sliceToStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, asStringPayload(value))
	}
	return result
}

var _ cli.LocationResolver = (*mockLocation)(nil)
var _ cli.ProfileResolver = (*mockProfiles)(nil)
var _ cli.ConfigManager = (*mockConfig)(nil)
var _ woltgateway.API = (*mockWolt)(nil)
