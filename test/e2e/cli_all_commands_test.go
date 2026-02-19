package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Valaraucoo/wolt-cli/internal/cli"
	"github.com/Valaraucoo/wolt-cli/internal/domain"
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
}

func TestVenueMenuJSON(t *testing.T) {
	venueItem := &domain.Item{Title: "Burger Place", TrackID: "track-1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Street")}
	staticPayload := map[string]any{
		"item_id":          "item-1",
		"name":             "Fries",
		"description":      "Crispy fries",
		"base_price":       599,
		"currency":         "PLN",
		"option_group_ids": []any{"opt-1"},
		"category":         "sides",
		"is_sold_out":      false,
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemBySlugFunc: func(context.Context, domain.Location, string) (*domain.Item, error) {
				return venueItem, nil
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			venuePageDynamicFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{}, nil
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
	if len(asSlicePayload(t, first["option_group_ids"])) != 1 {
		t.Fatalf("expected option_group_ids to be present")
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

	exitCode, out := runCLIWithDeps(t, deps, "configure", "--profile-name", "work", "--address", "Helsinki")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Config was created successfully") {
		t.Fatalf("expected success message, got:\n%s", out)
	}
	if loc.seenAddress != "Helsinki" {
		t.Fatalf("expected location lookup for Helsinki, got %q", loc.seenAddress)
	}
	if cfg.saved == nil || len(cfg.saved.Profiles) != 1 {
		t.Fatalf("expected saved config with one profile, got %+v", cfg.saved)
	}
	profile := cfg.saved.Profiles[0]
	if profile.Name != "work" || !profile.IsDefault {
		t.Fatalf("unexpected saved profile: %+v", profile)
	}
	if profile.Location.Lat != 60.1699 || profile.Location.Lon != 24.9384 {
		t.Fatalf("unexpected saved location: %+v", profile.Location)
	}
}

func TestConfigureCommandRequiresOverwriteWhenConfigExists(t *testing.T) {
	cfg := &recordingConfig{loadCfg: domain.Config{Profiles: []domain.Profile{{Name: "default", IsDefault: true}}}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &recordingLocation{location: domain.Location{Lat: 60, Lon: 24}},
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "configure", "--profile-name", "work", "--address", "Helsinki")
	if exitCode != 1 {
		t.Fatalf("expected exit 1 when config exists without --overwrite, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "config file already exists") {
		t.Fatalf("expected existing config error, got:\n%s", out)
	}
}

var _ cli.ConfigManager = (*recordingConfig)(nil)
var _ cli.LocationResolver = (*recordingLocation)(nil)
