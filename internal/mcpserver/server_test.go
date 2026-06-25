package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

// expectedToolNames is the lockfile for the v1 tool surface. Adding a new tool
// is a deliberate change — update this list (and the docs) at the same time.
var expectedToolNames = []string{
	"wolt_feed",
	"wolt_top",
	"wolt_search_venues",
	"wolt_venue_categories",
	"wolt_resolve_address",
	"wolt_venue_detail",
	"wolt_venue_menu",
	"wolt_venue_hours",
	"wolt_venue_item",
	"wolt_venue_search_items",
	"wolt_account_status",
	"wolt_account_orders",
	"wolt_account_order",
	"wolt_account_addresses",
	"wolt_account_payments",
	"wolt_favorites_list",
	"wolt_favorites_add",
	"wolt_favorites_remove",
	"wolt_cart_show",
	"wolt_cart_count",
	"wolt_cart_add",
	"wolt_cart_remove",
	"wolt_cart_clear",
	"wolt_checkout_preview",
}

func TestServerRegistersAllExpectedTools(t *testing.T) {
	ctx := context.Background()
	srv := NewServer(Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{},
		Location: &stubLocation{},
		Config:   &stubConfig{},
		Version:  "v0.0.0-test",
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	seen := map[string]struct{}{}
	for tool, iterErr := range cs.Tools(ctx, nil) {
		if iterErr != nil {
			t.Fatalf("list tools: %v", iterErr)
		}
		seen[tool.Name] = struct{}{}
	}
	if got, want := len(seen), len(expectedToolNames); got != want {
		t.Errorf("registered tool count = %d, want %d", got, want)
	}
	for _, want := range expectedToolNames {
		if _, ok := seen[want]; !ok {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestFeedToolReturnsStructuredOutput(t *testing.T) {
	ctx := context.Background()
	sectionsCalled := false
	wolt := &stubWolt{
		sectionsFn: func(context.Context, domain.Location) ([]domain.Section, error) {
			sectionsCalled = true
			return []domain.Section{{Name: "popular", Title: "Popular", Items: []domain.Item{}}}, nil
		},
	}
	srv, cs := connectInMemory(t, Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})
	defer func() { _ = cs.Close() }()
	_ = srv

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "wolt_feed",
		Arguments: map[string]any{
			"lat": 60.17,
			"lon": 24.94,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error result: %v", textContent(res))
	}
	if !sectionsCalled {
		t.Errorf("expected wolt.Sections to be called")
	}
}

func TestAccountStatusRejectsUnauthenticated(t *testing.T) {
	ctx := context.Background()
	srv, cs := connectInMemory(t, Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{findErr: errors.New("no profile")},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})
	defer func() { _ = cs.Close() }()
	_ = srv

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "wolt_account_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError=true for unauthenticated call")
	}
	if msg := textContent(res); !strings.Contains(msg, "wolt login") {
		t.Errorf("expected message to instruct 'wolt login'; got %q", msg)
	}
}

func TestResolveLocationPrefersExplicitLatLon(t *testing.T) {
	geocodeCalled := false
	tc := newToolCtx(Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{},
		Location: &stubLocation{getFn: func(context.Context, string) (domain.Location, error) {
			geocodeCalled = true
			return domain.Location{Lat: 1, Lon: 1}, nil
		}},
		Config: &stubConfig{},
	})
	loc, src, err := tc.resolveLocation(context.Background(), LocationInput{Lat: 60.5, Lon: 24.5})
	if err != nil {
		t.Fatalf("resolveLocation: %v", err)
	}
	if loc.Lat != 60.5 || loc.Lon != 24.5 {
		t.Errorf("loc = %+v, want {60.5, 24.5}", loc)
	}
	if src != "explicit" {
		t.Errorf("source = %q, want %q", src, "explicit")
	}
	if geocodeCalled {
		t.Errorf("geocoder must not be called when lat/lon are given")
	}
}

func TestResolveLocationRejectsHalfPair(t *testing.T) {
	tc := newToolCtx(Deps{Wolt: &stubWolt{}, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})
	_, _, err := tc.resolveLocation(context.Background(), LocationInput{Lat: 60.5})
	if err == nil {
		t.Fatalf("expected error for half lat/lon pair")
	}
}

func TestResolveLocationRejectsAddressWithLatLon(t *testing.T) {
	tc := newToolCtx(Deps{Wolt: &stubWolt{}, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})
	_, _, err := tc.resolveLocation(context.Background(), LocationInput{Lat: 60.5, Lon: 24.5, Address: "Foo"})
	if err == nil {
		t.Fatalf("expected error for address combined with lat/lon")
	}
}

func TestNewToolCtxPropagatesLocaleFromDeps(t *testing.T) {
	cases := []struct {
		name   string
		locale string
		want   string
	}{
		{name: "BCP-47 locale propagates verbatim", locale: "fi-FI", want: "fi-FI"},
		{name: "bare language propagates", locale: "sv", want: "sv"},
		{name: "empty locale propagates as empty string", locale: "", want: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := newToolCtx(Deps{
				Wolt:     &stubWolt{},
				Profiles: &stubProfiles{},
				Location: &stubLocation{},
				Config:   &stubConfig{},
				Locale:   tc.locale,
			})
			if ctx.locale != tc.want {
				t.Errorf("ToolCtx.locale = %q, want %q", ctx.locale, tc.want)
			}
		})
	}
}

func TestVenueSearchItemsUsesConfiguredLocale(t *testing.T) {
	cases := []struct {
		name         string
		locale       string
		wantLanguage string
	}{
		{name: "fi-FI -> fi", locale: "fi-FI", wantLanguage: "fi"},
		{name: "sv-SE -> sv", locale: "sv-SE", wantLanguage: "sv"},
		{name: "bare en passes through", locale: "en", wantLanguage: "en"},
		{name: "empty locale defaults to en", locale: "", wantLanguage: "en"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			gotLanguage := ""
			callCount := 0
			wolt := &stubWolt{
				assortmentSearchFn: func(_ context.Context, _ string, _ string, language string, _ woltgateway.AuthContext) (map[string]any, error) {
					callCount++
					gotLanguage = language
					return map[string]any{"items": []any{}}, nil
				},
			}
			srv, cs := connectInMemory(t, Deps{
				Wolt:     wolt,
				Profiles: &stubProfiles{},
				Location: &stubLocation{},
				Config:   &stubConfig{},
				Locale:   tc.locale,
			})
			defer func() { _ = cs.Close() }()
			_ = srv

			res, err := cs.CallTool(ctx, &mcp.CallToolParams{
				Name: "wolt_venue_search_items",
				Arguments: map[string]any{
					"venue": "test-venue-slug",
					"query": "pizza",
				},
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("expected success, got error result: %v", textContent(res))
			}
			if callCount != 1 {
				t.Fatalf("AssortmentItemsSearchByVenueSlug called %d times, want 1", callCount)
			}
			if gotLanguage != tc.wantLanguage {
				t.Errorf("language passed to gateway = %q, want %q", gotLanguage, tc.wantLanguage)
			}
		})
	}
}

func TestTokenExpiredHandlesJWT(t *testing.T) {
	// JWT with exp=1700000000 (2023-11-14 — in the past). Header.payload.sig
	// where payload is {"exp":1700000000}.
	expired := "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjE3MDAwMDAwMDB9.sig"
	if !tokenExpired(expired, parseTime("2024-01-01T00:00:00Z"), 0) {
		t.Errorf("expected expired token to be reported as expired")
	}
	if tokenExpired(expired, parseTime("2023-01-01T00:00:00Z"), 0) {
		t.Errorf("expected unexpired token at earlier 'now' to not be reported as expired")
	}
	if tokenExpired("not.a.jwt", parseTime("2024-01-01T00:00:00Z"), 0) {
		t.Errorf("malformed token must return false, not panic")
	}
	if tokenExpired("", parseTime("2024-01-01T00:00:00Z"), 0) {
		t.Errorf("empty token must return false")
	}
}

func TestResolveVenueRefFallsBackToDynamic(t *testing.T) {
	staticCalls, dynamicCalls := 0, 0
	wolt := &stubWolt{
		venueStaticFn: func(_ context.Context, _ string) (map[string]any, error) {
			staticCalls++
			// Wolt now 404s the static page for most venues.
			return nil, errors.New("status 404")
		},
		venueDynamicFn: func(_ context.Context, slug string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			dynamicCalls++
			if slug != "eat-poke-iso-omena" {
				t.Fatalf("expected slug passed to dynamic page, got %q", slug)
			}
			return map[string]any{"venue": map[string]any{"id": "637e383476c00f021e6bf084"}}, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}, Location: &stubLocation{}, Config: &stubConfig{}})

	ref, err := tc.resolveVenueRef(context.Background(), "eat-poke-iso-omena")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.ID != "637e383476c00f021e6bf084" {
		t.Fatalf("expected id resolved via dynamic fallback, got %q", ref.ID)
	}
	if staticCalls != 1 || dynamicCalls != 1 {
		t.Fatalf("expected static=1 dynamic=1, got static=%d dynamic=%d", staticCalls, dynamicCalls)
	}
}

// TestHandleCartAddRejectsUnresolvedVenue locks in the issue #19 fix for the
// MCP path: when a slug cannot be resolved to a real venue id, wolt_cart_add
// must error rather than POST the slug as venue_id (which the Wolt backend
// turns into a non-persisting phantom basket while reporting success).
func TestHandleCartAddRejectsUnresolvedVenue(t *testing.T) {
	addCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("status 404")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("status 404")
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			addCalled = true
			return map[string]any{"id": "phantom", "venue_id": "unresolved-slug"}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt:     wolt,
		Profiles: &stubProfiles{profile: domain.Profile{Name: "default", WToken: "token"}},
		Location: &stubLocation{},
		Config:   &stubConfig{},
	})

	_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		Venue:  "unresolved-slug",
		ItemID: "637f5bdbe4e55632767da017",
		Price:  500,
	})
	if err == nil {
		t.Fatalf("expected an error for an unresolved venue, got nil")
	}
	if !strings.Contains(err.Error(), "venue id") {
		t.Fatalf("expected a venue-resolution error, got: %v", err)
	}
	if addCalled {
		t.Fatalf("AddToBasket must NOT be called when the venue is unresolved (would create a phantom basket)")
	}
}

// ---------------- helpers ----------------

func connectInMemory(t *testing.T, deps Deps) (*mcp.Server, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	srv := NewServer(deps)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return srv, cs
}

func textContent(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// ---------------- stubs ----------------

type stubWolt struct {
	sectionsFn         func(context.Context, domain.Location) ([]domain.Section, error)
	itemsFn            func(context.Context, domain.Location) ([]domain.Item, error)
	userMeFn           func(context.Context, woltgateway.AuthContext) (map[string]any, error)
	restaurantFn       func(context.Context, string) (*domain.Restaurant, error)
	assortmentSearchFn func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	venueStaticFn      func(context.Context, string) (map[string]any, error)
	venueDynamicFn     func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error)
	addToBasketFn      func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
	basketsPageFn      func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error)
	checkoutPreviewFn  func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error)
}

func (s *stubWolt) FrontPage(context.Context, domain.Location) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) Sections(ctx context.Context, loc domain.Location) ([]domain.Section, error) {
	if s.sectionsFn != nil {
		return s.sectionsFn(ctx, loc)
	}
	return nil, nil
}
func (s *stubWolt) Items(ctx context.Context, loc domain.Location) ([]domain.Item, error) {
	if s.itemsFn != nil {
		return s.itemsFn(ctx, loc)
	}
	return nil, nil
}
func (s *stubWolt) RestaurantByID(ctx context.Context, id string) (*domain.Restaurant, error) {
	if s.restaurantFn != nil {
		return s.restaurantFn(ctx, id)
	}
	return nil, nil
}
func (s *stubWolt) Search(context.Context, domain.Location, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) VenuePageStatic(ctx context.Context, slug string) (map[string]any, error) {
	if s.venueStaticFn != nil {
		return s.venueStaticFn(ctx, slug)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) VenuePageDynamic(ctx context.Context, slug string, opts woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
	if s.venueDynamicFn != nil {
		return s.venueDynamicFn(ctx, slug, opts)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) AssortmentByVenueSlug(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) AssortmentCategoryByVenueSlug(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) AssortmentItemsByVenueSlug(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) AssortmentItemsSearchByVenueSlug(ctx context.Context, slug string, query string, language string, auth woltgateway.AuthContext) (map[string]any, error) {
	if s.assortmentSearchFn != nil {
		return s.assortmentSearchFn(ctx, slug, query, language, auth)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) VenueContentByVenueSlug(context.Context, string, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) VenueItemPage(context.Context, string, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) ItemBySlug(context.Context, domain.Location, string) (*domain.Item, error) {
	return nil, nil
}
func (s *stubWolt) UserMe(ctx context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
	if s.userMeFn != nil {
		return s.userMeFn(ctx, auth)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) Subscriptions(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) PaymentMethods(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) PaymentMethodsProfile(context.Context, woltgateway.AuthContext, woltgateway.PaymentMethodsProfileOptions) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) AddressFields(context.Context, domain.Location, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) DeliveryInfoList(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) DeliveryInfoCreate(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) DeliveryInfoDelete(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) OrderHistory(context.Context, woltgateway.AuthContext, woltgateway.OrderHistoryOptions) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) OrderHistoryPurchase(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) FavoriteVenues(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) FavoriteVenueAdd(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) FavoriteVenueRemove(context.Context, string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) BasketCount(context.Context, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) BasketsPage(ctx context.Context, loc domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
	if s.basketsPageFn != nil {
		return s.basketsPageFn(ctx, loc, auth)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) AddToBasket(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if s.addToBasketFn != nil {
		return s.addToBasketFn(ctx, payload, auth)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) DeleteBaskets(context.Context, []string, woltgateway.AuthContext) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *stubWolt) CheckoutPreview(ctx context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
	if s.checkoutPreviewFn != nil {
		return s.checkoutPreviewFn(ctx, payload, auth)
	}
	return map[string]any{}, nil
}
func (s *stubWolt) RefreshAccessToken(context.Context, string, woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
	return woltgateway.TokenRefreshResult{}, nil
}

type stubProfiles struct {
	profile domain.Profile
	findErr error
}

func (s *stubProfiles) Find(context.Context, string) (domain.Profile, error) {
	if s.findErr != nil {
		return domain.Profile{}, s.findErr
	}
	return s.profile, nil
}

type stubLocation struct {
	getFn func(context.Context, string) (domain.Location, error)
}

func (s *stubLocation) Get(ctx context.Context, address string) (domain.Location, error) {
	if s.getFn != nil {
		return s.getFn(ctx, address)
	}
	return domain.Location{}, nil
}

type stubConfig struct {
	cfg domain.Config
}

func (s *stubConfig) Path() string { return "/tmp/test-config.json" }
func (s *stubConfig) Load(context.Context) (domain.Config, error) {
	return s.cfg, nil
}
func (s *stubConfig) Save(_ context.Context, cfg domain.Config) error {
	s.cfg = cfg
	return nil
}
