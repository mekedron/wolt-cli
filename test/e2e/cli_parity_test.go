package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Valaraucoo/wolt-cli/internal/cli"
	"github.com/Valaraucoo/wolt-cli/internal/domain"
	woltgateway "github.com/Valaraucoo/wolt-cli/internal/gateway/wolt"
)

type mockWolt struct {
	frontPageFunc        func(context.Context, domain.Location) (map[string]any, error)
	sectionsFunc         func(context.Context, domain.Location) ([]domain.Section, error)
	itemsFunc            func(context.Context, domain.Location) ([]domain.Item, error)
	restaurantByIDFunc   func(context.Context, string) (*domain.Restaurant, error)
	searchFunc           func(context.Context, domain.Location, string) (map[string]any, error)
	venuePageStaticFunc  func(context.Context, string) (map[string]any, error)
	venuePageDynamicFunc func(context.Context, string) (map[string]any, error)
	venueItemPageFunc    func(context.Context, string, string) (map[string]any, error)
	itemBySlugFunc       func(context.Context, domain.Location, string) (*domain.Item, error)
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

func (m *mockWolt) VenuePageDynamic(ctx context.Context, slug string) (map[string]any, error) {
	if m.venuePageDynamicFunc == nil {
		return nil, errors.New("venue page dynamic not mocked")
	}
	return m.venuePageDynamicFunc(ctx, slug)
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
		Address:   "Krakow",
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Execute(context.Background(), args, deps, &stdout, &stderr)
	return exitCode, stdout.String() + stderr.String()
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
	for _, notExpected := range []string{"╭", "╰", "random"} {
		if strings.Contains(out, notExpected) {
			t.Fatalf("did not expect output to contain %q\noutput:\n%s", notExpected, out)
		}
	}
}

func TestRandomCommandIsRemoved(t *testing.T) {
	exitCode, out := runCLI(t, "random")
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "No such command 'random'") {
		t.Fatalf("expected unknown command message\noutput:\n%s", out)
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
}

func TestDiscoverFeedUsesDefaultProfileLocation(t *testing.T) {
	profile := domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}
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
		t.Fatalf("expected location %+v, got %+v", profile.Location, seen)
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
				return nil, woltgateway.ErrUpstream
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
