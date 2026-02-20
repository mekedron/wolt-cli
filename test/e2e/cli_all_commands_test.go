package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type recordingConfig struct {
	loadCfg domain.Config
	loadErr error
	saved   *domain.Config
}

func (r *recordingConfig) Path() string {
	return "/tmp/test-config.json"
}

func (r *recordingConfig) Load(context.Context) (domain.Config, error) {
	if r.loadErr != nil {
		return domain.Config{}, r.loadErr
	}
	return r.loadCfg, nil
}

func (r *recordingConfig) Save(_ context.Context, cfg domain.Config) error {
	copyCfg := cfg
	r.saved = &copyCfg
	return nil
}

type recordingLocation struct {
	seenAddress string
	location    domain.Location
	err         error
}

func (r *recordingLocation) Get(_ context.Context, address string) (domain.Location, error) {
	r.seenAddress = address
	if r.err != nil {
		return domain.Location{}, r.err
	}
	return r.location, nil
}

func TestDiscoverCategoriesJSON(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Burger One", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: &domain.Venue{ID: "venue-1", Tags: []string{"burger", "vegan"}}},
				{Title: "Burger Two", TrackID: "2", Link: domain.Link{Target: "venue-2"}, Venue: &domain.Venue{ID: "venue-2", Tags: []string{"burger"}}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "categories", "--lat", "50.0", "--lon", "19.0", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	categories := asSlicePayload(t, asMapPayload(t, payload["data"])["categories"])
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	first := asMapPayload(t, categories[0])
	second := asMapPayload(t, categories[1])
	if first["slug"] != "burger" || second["slug"] != "vegan" {
		t.Fatalf("expected category slugs burger, vegan got %v and %v", first["slug"], second["slug"])
	}
}

func TestDiscoverFeedWoltPlusFilterJSON(t *testing.T) {
	plusVenue := buildVenue("venue-1", "plus-venue", "Plus Street")
	plusVenue.ShowWoltPlus = true
	regularVenue := buildVenue("venue-2", "regular-venue", "Regular Street")
	regularVenue.ShowWoltPlus = false
	regularVenue.Promotions = nil
	regularVenue.Badges = nil
	regularVenue.Tags = []string{"burger"}

	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Plus Venue", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: plusVenue},
				{Title: "Regular Venue", TrackID: "2", Link: domain.Link{Target: "venue-2"}, Venue: regularVenue},
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return map[string]any{"city_data": map[string]any{"name": "Krakow"}}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--wolt-plus", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["wolt_plus_only"] != true {
		t.Fatalf("expected wolt_plus_only true, got %v", data["wolt_plus_only"])
	}
	sectionRows := asSlicePayload(t, data["sections"])
	if len(sectionRows) != 1 {
		t.Fatalf("expected one section, got %d", len(sectionRows))
	}
	items := asSlicePayload(t, asMapPayload(t, sectionRows[0])["items"])
	if len(items) != 1 {
		t.Fatalf("expected one filtered item, got %d", len(items))
	}
	if asMapPayload(t, items[0])["name"] != "Plus Venue" {
		t.Fatalf("expected Plus Venue, got %v", asMapPayload(t, items[0])["name"])
	}
}

func TestSearchVenuesWithoutQueryListsRestaurants(t *testing.T) {
	items := []domain.Item{
		{Title: "Burger Place", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Burger Street")},
		{Title: "Sushi Place", TrackID: "2", Link: domain.Link{Target: "venue-2"}, Venue: buildVenue("venue-2", "sushi-place", "Sushi Street")},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return items, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "search", "venues", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["total"]) != 2 {
		t.Fatalf("expected total 2, got %v", data["total"])
	}
	rows := asSlicePayload(t, data["items"])
	if len(rows) != 2 {
		t.Fatalf("expected 2 items, got %d", len(rows))
	}
	first := asMapPayload(t, rows[0])
	if first["price_range"] != float64(2) {
		t.Fatalf("expected price_range 2, got %v", first["price_range"])
	}
	if first["price_range_scale"] != "$$" {
		t.Fatalf("expected price_range_scale $$, got %v", first["price_range_scale"])
	}
	promotions := asSlicePayload(t, first["promotions"])
	if len(promotions) != 1 || promotions[0] != "Free delivery" {
		t.Fatalf("expected promotions [Free delivery], got %v", promotions)
	}
}

func TestVenueMenuJSON(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id":             "venue-1",
			"show_wolt_plus": true,
		},
	}
	assortmentPayload := map[string]any{
		"categories": []any{
			map[string]any{
				"name":     "sides",
				"item_ids": []any{"item-1"},
			},
		},
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Fries",
				"price": 599,
				"promotions": []any{
					map[string]any{"text": "2 for 1"},
				},
				"options": []any{map[string]any{"option_id": "opt-1"}},
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return assortmentPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "burger-place", "--include-options", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected 1 menu item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	if first["item_id"] != "item-1" {
		t.Fatalf("expected item_id item-1, got %v", first["item_id"])
	}
	if data["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true, got %v", data["wolt_plus"])
	}
	discounts := asSlicePayload(t, first["discounts"])
	if len(discounts) != 1 || discounts[0] != "2 for 1" {
		t.Fatalf("expected discounts [2 for 1], got %v", discounts)
	}
	if len(asSlicePayload(t, first["option_group_ids"])) != 1 {
		t.Fatalf("expected option_group_ids to be present")
	}
}

func TestVenueMenuTableShowsRows(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id":             "venue-1",
			"show_wolt_plus": true,
		},
	}
	assortmentPayload := map[string]any{
		"categories": []any{
			map[string]any{
				"name":     "sides",
				"item_ids": []any{"item-1"},
			},
		},
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Fries",
				"price": 599,
				"promotions": []any{
					map[string]any{"text": "2 for 1"},
				},
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return assortmentPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "burger-place", "--limit", "1")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "item-1") || !strings.Contains(out, "Fries") {
		t.Fatalf("expected table output to include item row, got:\n%s", out)
	}
	if !strings.Contains(out, "2 for 1") {
		t.Fatalf("expected table output to include discounts, got:\n%s", out)
	}
	if !strings.Contains(out, "(Wolt+)") {
		t.Fatalf("expected table output to include Wolt+ marker, got:\n%s", out)
	}
}

func TestVenueHoursJSON(t *testing.T) {
	venueItem := &domain.Item{Title: "Burger Place", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Street")}
	restaurant := &domain.Restaurant{
		ID:           "venue-1",
		TimezoneName: "UTC",
		OpeningTimes: map[string][]domain.Times{
			"monday": {
				{Type: "open", Value: map[string]int64{"$date": time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC).UnixMilli()}},
				{Type: "close", Value: map[string]int64{"$date": time.Date(2026, 2, 16, 20, 45, 0, 0, time.UTC).UnixMilli()}},
			},
		},
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "hours", "burger-place", "--timezone", "Europe/Helsinki", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["timezone"] != "Europe/Helsinki" {
		t.Fatalf("expected timezone override Europe/Helsinki, got %v", data["timezone"])
	}
	windows := asSlicePayload(t, data["opening_windows"])
	if len(windows) != 7 {
		t.Fatalf("expected 7 opening windows, got %d", len(windows))
	}
	first := asMapPayload(t, windows[0])
	if first["day"] != "monday" || first["open"] != "10:00" || first["close"] != "20:45" {
		t.Fatalf("unexpected monday window: %v", first)
	}
}

func TestVenueHoursFallbackStaticWhenItemLookupFails(t *testing.T) {
	restaurant := &domain.Restaurant{
		ID:           "venue-1",
		TimezoneName: "UTC",
		OpeningTimes: map[string][]domain.Times{
			"monday": {
				{Type: "open", Value: map[string]int64{"$date": time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC).UnixMilli()}},
				{Type: "close", Value: map[string]int64{"$date": time.Date(2026, 2, 16, 20, 45, 0, 0, time.UTC).UnixMilli()}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 404}
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"venue": map[string]any{
						"id":   "venue-1",
						"slug": "burger-place",
						"name": "Burger Place",
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "hours", "burger-place", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
}

func TestVenueHoursFallbackWhenRestaurantEndpointGone(t *testing.T) {
	venueItem := &domain.Item{
		Title: "Burger Place",
		Link:  domain.Link{Target: "venue-1"},
		Venue: &domain.Venue{ID: "venue-1", Slug: "burger-place"},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return venueItem, nil
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"venue": map[string]any{
						"id":   "venue-1",
						"slug": "burger-place",
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "hours", "burger-place", "--timezone", "Europe/Helsinki", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	if data["timezone"] != "Europe/Helsinki" {
		t.Fatalf("expected timezone override Europe/Helsinki, got %v", data["timezone"])
	}
}

func TestItemOptionsJSON(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":      "item-1",
				"name":    "Combo",
				"price":   1299,
				"options": []any{map[string]any{"option_id": "group-drink"}},
			},
		},
		"options": []any{
			map[string]any{
				"id":   "group-drink",
				"name": "Drink",
				"min":  1,
				"max":  1,
				"values": []any{
					map[string]any{"id": "value-cola", "name": "Cola", "price": 100},
				},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return assortmentPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "item", "options", "burger-place", "item-1", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["item_id"] != "item-1" {
		t.Fatalf("expected item_id item-1, got %v", data["item_id"])
	}
	if asIntPayload(data["group_count"]) != 1 {
		t.Fatalf("expected group_count 1, got %v", data["group_count"])
	}
	groups := asSlicePayload(t, data["option_groups"])
	if len(groups) != 1 {
		t.Fatalf("expected one option group, got %d", len(groups))
	}
	group := asMapPayload(t, groups[0])
	if group["group_id"] != "group-drink" {
		t.Fatalf("expected group id group-drink, got %v", group["group_id"])
	}
	values := asSlicePayload(t, group["values"])
	if len(values) != 1 {
		t.Fatalf("expected one option value, got %d", len(values))
	}
	value := asMapPayload(t, values[0])
	if value["example_option"] != "group-drink=value-cola" {
		t.Fatalf("expected example option group-drink=value-cola, got %v", value["example_option"])
	}
}

func TestItemShowFailsWhenItemMissingInVenue(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-available",
				"name":  "Combo",
				"price": 1299,
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return assortmentPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "item", "show", "burger-place", "item-missing")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "was not found for venue slug") {
		t.Fatalf("expected not-found error message, got:\n%s", out)
	}
}

func TestConfigureCommandSavesProfile(t *testing.T) {
	cfg := &recordingConfig{loadErr: errors.New("config not found")}
	loc := &recordingLocation{location: domain.Location{Lat: 60.1699, Lon: 24.9384}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: loc,
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "configure", "--profile-name", "work")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Config was created successfully") {
		t.Fatalf("expected success message, got:\n%s", out)
	}
	if loc.seenAddress != "" {
		t.Fatalf("did not expect location lookup during configure, got %q", loc.seenAddress)
	}
	if cfg.saved == nil || len(cfg.saved.Profiles) != 1 {
		t.Fatalf("expected saved config with one profile, got %+v", cfg.saved)
	}
	profile := cfg.saved.Profiles[0]
	if profile.Name != "work" || !profile.IsDefault {
		t.Fatalf("unexpected saved profile: %+v", profile)
	}
	if profile.Address != "" {
		t.Fatalf("expected no saved profile address, got %q", profile.Address)
	}
}

func TestConfigureCommandSavesNormalizedWToken(t *testing.T) {
	cfg := &recordingConfig{loadErr: errors.New("config not found")}
	loc := &recordingLocation{location: domain.Location{Lat: 60.1699, Lon: 24.9384}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: loc,
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"configure",
		"--profile-name",
		"work",
		"--wtoken",
		`{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`,
		"--cookie",
		"foo=bar",
		"--cookie",
		"__wtoken={%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if cfg.saved == nil || len(cfg.saved.Profiles) != 1 {
		t.Fatalf("expected saved config with one profile, got %+v", cfg.saved)
	}
	if cfg.saved.Profiles[0].WToken != "abc.def.ghi" {
		t.Fatalf("expected normalized wtoken abc.def.ghi, got %q", cfg.saved.Profiles[0].WToken)
	}
	if len(cfg.saved.Profiles[0].Cookies) != 2 {
		t.Fatalf("expected two saved cookies, got %v", cfg.saved.Profiles[0].Cookies)
	}
}

func TestConfigureCommandRequiresAuthInputsWhenConfigExists(t *testing.T) {
	cfg := &recordingConfig{loadCfg: domain.Config{Profiles: []domain.Profile{{Name: "default", IsDefault: true}}}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &recordingLocation{location: domain.Location{Lat: 60, Lon: 24}},
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "configure", "--profile-name", "work")
	if exitCode != 1 {
		t.Fatalf("expected exit 1 when config exists without auth update flags, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "provide --wtoken, --wrtoken, or --cookie") {
		t.Fatalf("expected missing auth flag error, got:\n%s", out)
	}
}

func TestConfigureCommandUpdatesAuthWithoutAddress(t *testing.T) {
	cfg := &recordingConfig{
		loadCfg: domain.Config{
			Profiles: []domain.Profile{
				{
					Name:          "default",
					IsDefault:     true,
					Address:       "Helsinki",
					Location:      domain.Location{Lat: 60.1699, Lon: 24.9384},
					WToken:        "old-token",
					WRefreshToken: "old-refresh",
					Cookies:       []string{"foo=bar"},
				},
			},
		},
	}
	loc := &recordingLocation{location: domain.Location{Lat: 1, Lon: 2}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1699, Lon: 24.9384}}},
		Location: loc,
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"configure",
		"--profile-name",
		"default",
		"--wtoken",
		`{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`,
		"--wrtoken",
		"%22refresh-new%22",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if cfg.saved == nil || len(cfg.saved.Profiles) != 1 {
		t.Fatalf("expected saved config with one profile, got %+v", cfg.saved)
	}
	saved := cfg.saved.Profiles[0]
	if saved.Address != "Helsinki" {
		t.Fatalf("expected address to stay unchanged, got %q", saved.Address)
	}
	if saved.Location.Lat != 60.1699 || saved.Location.Lon != 24.9384 {
		t.Fatalf("expected location to stay unchanged, got %+v", saved.Location)
	}
	if saved.WToken != "abc.def.ghi" {
		t.Fatalf("expected updated wtoken abc.def.ghi, got %q", saved.WToken)
	}
	if saved.WRefreshToken != "refresh-new" {
		t.Fatalf("expected updated wrefresh_token refresh-new, got %q", saved.WRefreshToken)
	}
	if loc.seenAddress != "" {
		t.Fatalf("did not expect address geocoding for auth-only update, got %q", loc.seenAddress)
	}
}

var _ cli.ConfigManager = (*recordingConfig)(nil)
var _ cli.LocationResolver = (*recordingLocation)(nil)
