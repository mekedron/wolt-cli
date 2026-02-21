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

func TestDiscoverFeedTableIncludesSlug(t *testing.T) {
	venue := buildVenue("venue-1", "plus-venue", "Plus Street")
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Plus Venue", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue},
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

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Slug") {
		t.Fatalf("expected table to include Slug column, got:\n%s", out)
	}
	if !strings.Contains(out, "plus-venue") {
		t.Fatalf("expected table to include venue slug value, got:\n%s", out)
	}
}

func TestDiscoverFeedMergesDynamicPromotions(t *testing.T) {
	venue := buildVenue("venue-1", "plus-venue", "Plus Street")
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Plus Venue", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue},
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
			venuePageDynamicFunc: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				return map[string]any{
					"venue_raw": map[string]any{
						"discounts": []any{
							map[string]any{
								"description": map[string]any{"title": "40% off selected items"},
							},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionRows := asSlicePayload(t, data["sections"])
	if len(sectionRows) != 1 {
		t.Fatalf("expected one section, got %d", len(sectionRows))
	}
	items := asSlicePayload(t, asMapPayload(t, sectionRows[0])["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	promotions := asSlicePayload(t, asMapPayload(t, items[0])["promotions"])
	if len(promotions) != 2 {
		t.Fatalf("expected two promotion labels, got %v", promotions)
	}
	if !containsStringPayload(promotions, "Free delivery") {
		t.Fatalf("expected Free delivery in promotions, got %v", promotions)
	}
	if !containsStringPayload(promotions, "40% off selected items") {
		t.Fatalf("expected campaign promotion in promotions, got %v", promotions)
	}
}

func TestDiscoverFeedEnrichesWoltPlusFromStaticVenue(t *testing.T) {
	venue := buildVenue("venue-1", "plus-venue", "Plus Street")
	venue.ShowWoltPlus = false
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Plus Venue", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue},
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
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"venue_raw": map[string]any{
						"is_wolt_plus": true,
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionRows := asSlicePayload(t, data["sections"])
	if len(sectionRows) != 1 {
		t.Fatalf("expected one section, got %d", len(sectionRows))
	}
	items := asSlicePayload(t, asMapPayload(t, sectionRows[0])["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	if first["wolt_plus"] != true {
		t.Fatalf("expected wolt_plus true from static payload, got %v", first["wolt_plus"])
	}
}

func TestDiscoverFeedPaginationUsesGlobalLimitOffset(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Venue A", TrackID: "1", Link: domain.Link{Target: "venue-a"}, Venue: buildVenue("venue-a", "venue-a", "Street A")},
				{Title: "Venue B", TrackID: "2", Link: domain.Link{Target: "venue-b"}, Venue: buildVenue("venue-b", "venue-b", "Street B")},
			},
		},
		{
			Name:  "nearby",
			Title: "Nearby",
			Items: []domain.Item{
				{Title: "Venue C", TrackID: "3", Link: domain.Link{Target: "venue-c"}, Venue: buildVenue("venue-c", "venue-c", "Street C")},
				{Title: "Venue D", TrackID: "4", Link: domain.Link{Target: "venue-d"}, Venue: buildVenue("venue-d", "venue-d", "Street D")},
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

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--limit", "2", "--offset", "1", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["total"]) != 4 {
		t.Fatalf("expected total 4, got %v", data["total"])
	}
	if asIntPayload(data["count"]) != 2 {
		t.Fatalf("expected count 2, got %v", data["count"])
	}
	if asIntPayload(data["offset"]) != 1 {
		t.Fatalf("expected offset 1, got %v", data["offset"])
	}
	if asIntPayload(data["next_offset"]) != 3 {
		t.Fatalf("expected next_offset 3, got %v", data["next_offset"])
	}
	if asIntPayload(data["total_pages"]) != 2 {
		t.Fatalf("expected total_pages 2, got %v", data["total_pages"])
	}

	names := []string{}
	for _, sectionValue := range asSlicePayload(t, data["sections"]) {
		section := asMapPayload(t, sectionValue)
		for _, itemValue := range asSlicePayload(t, section["items"]) {
			item := asMapPayload(t, itemValue)
			names = append(names, asStringPayload(item["name"]))
		}
	}
	if len(names) != 2 || names[0] != "Venue B" || names[1] != "Venue C" {
		t.Fatalf("expected paginated names [Venue B Venue C], got %v", names)
	}
}

func TestDiscoverFeedFastSkipsVenueEnrichment(t *testing.T) {
	venue := buildVenue("venue-1", "plus-venue", "Plus Street")
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Plus Venue", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue},
			},
		},
	}
	dynamicCalls := 0
	staticCalls := 0

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return map[string]any{"city_data": map[string]any{"name": "Krakow"}}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
			venuePageDynamicFunc: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				dynamicCalls++
				return map[string]any{
					"venue_raw": map[string]any{
						"discounts": []any{
							map[string]any{
								"description": map[string]any{"title": "40% off selected items"},
							},
						},
					},
				}, nil
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				staticCalls++
				return map[string]any{
					"venue_raw": map[string]any{
						"is_wolt_plus": true,
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--fast", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if dynamicCalls != 0 {
		t.Fatalf("expected no dynamic calls in fast mode, got %d", dynamicCalls)
	}
	if staticCalls != 0 {
		t.Fatalf("expected no static calls in fast mode, got %d", staticCalls)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["enrichment_mode"] != "fast" {
		t.Fatalf("expected enrichment_mode fast, got %v", data["enrichment_mode"])
	}
	sectionRows := asSlicePayload(t, data["sections"])
	items := asSlicePayload(t, asMapPayload(t, sectionRows[0])["items"])
	promotions := asSlicePayload(t, asMapPayload(t, items[0])["promotions"])
	if containsStringPayload(promotions, "40% off selected items") {
		t.Fatalf("expected fast mode to skip campaign enrichment, got %v", promotions)
	}
}

func TestDiscoverFeedUsesFrontPayloadSectionsWithoutFallbackCall(t *testing.T) {
	sectionsCalls := 0

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			frontPageFunc: func(context.Context, domain.Location) (map[string]any, error) {
				return map[string]any{
					"city_data": map[string]any{"name": "Krakow"},
					"sections": []any{
						map[string]any{
							"name":  "popular",
							"title": "Popular",
							"items": []any{
								map[string]any{
									"title":    "Front Venue",
									"track_id": "track-1",
									"link":     map[string]any{"target": "venue-1"},
									"venue": map[string]any{
										"id":             "venue-1",
										"slug":           "front-venue",
										"estimate_range": "20-30",
										"currency":       "EUR",
									},
								},
							},
						},
					},
				}, nil
			},
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				sectionsCalls++
				return nil, errors.New("should not be called when front payload includes sections")
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "discover", "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if sectionsCalls != 0 {
		t.Fatalf("expected zero fallback sections calls, got %d", sectionsCalls)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionRows := asSlicePayload(t, data["sections"])
	items := asSlicePayload(t, asMapPayload(t, sectionRows[0])["items"])
	first := asMapPayload(t, items[0])
	if first["name"] != "Front Venue" {
		t.Fatalf("expected Front Venue row from front payload, got %v", first["name"])
	}
}

func TestDiscoverFeedSupportsQuerySortAndPage(t *testing.T) {
	venueA := buildVenue("venue-a", "venue-a", "Street A")
	venueA.Rating = &domain.Rating{Score: 8.5}
	venueB := buildVenue("venue-b", "venue-b", "Street B")
	venueB.Rating = &domain.Rating{Score: 9.1}
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{Title: "Alpha Burger", TrackID: "1", Link: domain.Link{Target: "venue-a"}, Venue: venueA},
				{Title: "Beta Burger", TrackID: "2", Link: domain.Link{Target: "venue-b"}, Venue: venueB},
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

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"discover",
		"feed",
		"--query",
		"burger",
		"--sort",
		"rating",
		"--limit",
		"1",
		"--page",
		"2",
		"--fast",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["sort"] != "rating" {
		t.Fatalf("expected sort rating, got %v", data["sort"])
	}
	if asIntPayload(data["page"]) != 2 {
		t.Fatalf("expected page 2, got %v", data["page"])
	}
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected count 1, got %v", data["count"])
	}
	sectionsRows := asSlicePayload(t, data["sections"])
	items := asSlicePayload(t, asMapPayload(t, sectionsRows[0])["items"])
	first := asMapPayload(t, items[0])
	if first["name"] != "Alpha Burger" {
		t.Fatalf("expected second page to return Alpha Burger, got %v", first["name"])
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

func TestSearchVenuesSupportsPageAndFilters(t *testing.T) {
	venueA := buildVenue("venue-a", "venue-a", "Street A")
	venueA.Rating = &domain.Rating{Score: 8.6}
	venueA.DeliveryPriceInt = intPtr(500)
	venueB := buildVenue("venue-b", "venue-b", "Street B")
	venueB.Rating = &domain.Rating{Score: 9.3}
	venueB.DeliveryPriceInt = intPtr(100)
	items := []domain.Item{
		{Title: "Alpha Burger", TrackID: "1", Link: domain.Link{Target: "venue-a"}, Venue: venueA},
		{Title: "Beta Burger", TrackID: "2", Link: domain.Link{Target: "venue-b"}, Venue: venueB},
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

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"search",
		"venues",
		"--query",
		"burger",
		"--sort",
		"rating",
		"--min-rating",
		"8.5",
		"--max-delivery-fee",
		"500",
		"--limit",
		"1",
		"--page",
		"2",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected count 1, got %v", data["count"])
	}
	rows := asSlicePayload(t, data["items"])
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if asMapPayload(t, rows[0])["name"] != "Alpha Burger" {
		t.Fatalf("expected second page venue Alpha Burger, got %v", asMapPayload(t, rows[0])["name"])
	}
}

func TestSearchVenuesMergesDynamicPromotions(t *testing.T) {
	items := []domain.Item{
		{Title: "Burger Place", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Burger Street")},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return items, nil
			},
			venuePageDynamicFunc: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				return map[string]any{
					"venue_raw": map[string]any{
						"discounts": []any{
							map[string]any{
								"description": map[string]any{"title": "40% off selected items"},
							},
						},
					},
				}, nil
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
	rows := asSlicePayload(t, data["items"])
	if len(rows) != 1 {
		t.Fatalf("expected 1 item, got %d", len(rows))
	}
	promotions := asSlicePayload(t, asMapPayload(t, rows[0])["promotions"])
	if len(promotions) != 2 {
		t.Fatalf("expected two promotion labels, got %v", promotions)
	}
	if !containsStringPayload(promotions, "Free delivery") {
		t.Fatalf("expected Free delivery in promotions, got %v", promotions)
	}
	if !containsStringPayload(promotions, "40% off selected items") {
		t.Fatalf("expected campaign promotion in promotions, got %v", promotions)
	}
}

func TestSearchVenuesTableIncludesSlug(t *testing.T) {
	items := []domain.Item{
		{Title: "Groceries One", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "groceries-one", "Grocery Street")},
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

	exitCode, out := runCLIWithDeps(t, deps, "search", "venues", "--open-now", "--query", "groceries")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Slug") {
		t.Fatalf("expected table to include Slug column, got:\n%s", out)
	}
	if !strings.Contains(out, "groceries-one") {
		t.Fatalf("expected table to include venue slug value, got:\n%s", out)
	}
}

func TestSearchItemsSupportsPageAndFilters(t *testing.T) {
	searchPayload := map[string]any{
		"venue": map[string]any{
			"currency": "EUR",
		},
		"items": []any{
			map[string]any{
				"id":          "item-a",
				"name":        "Alpha Burger",
				"price":       700,
				"is_sold_out": false,
				"discounts":   []any{"20% off"},
			},
			map[string]any{
				"id":          "item-b",
				"name":        "Beta Burger",
				"price":       900,
				"is_sold_out": false,
				"discounts":   []any{"10% off"},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return []domain.Item{}, nil
			},
			searchFunc: func(context.Context, domain.Location, string) (map[string]any, error) {
				return searchPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"search",
		"items",
		"--query",
		"burger",
		"--sort",
		"price",
		"--min-price",
		"700",
		"--max-price",
		"900",
		"--discounts-only",
		"--limit",
		"1",
		"--page",
		"2",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected count 1, got %v", data["count"])
	}
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one row, got %d", len(items))
	}
	if asMapPayload(t, items[0])["name"] != "Beta Burger" {
		t.Fatalf("expected second page to return Beta Burger, got %v", asMapPayload(t, items[0])["name"])
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

func TestVenueMenuSupportsPageSortAndFilters(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{"id": "item-a", "name": "Alpha", "price": 700, "is_sold_out": false, "promotions": []any{"20% off"}},
			map[string]any{"id": "item-b", "name": "Beta", "price": 900, "is_sold_out": false, "promotions": []any{"10% off"}},
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

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"venue",
		"menu",
		"burger-place",
		"--sort",
		"price",
		"--discounts-only",
		"--limit",
		"1",
		"--page",
		"2",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected count 1, got %v", data["count"])
	}
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one row, got %d", len(items))
	}
	if asMapPayload(t, items[0])["name"] != "Beta" {
		t.Fatalf("expected second page to return Beta, got %v", asMapPayload(t, items[0])["name"])
	}
}

func TestVenueMenuMergesDynamicCampaignDiscounts(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Steakhouse",
				"price": 1075,
			},
		},
	}
	dynamicPayload := map[string]any{
		"venue_raw": map[string]any{
			"discounts": []any{
				map[string]any{
					"effects": map[string]any{
						"item_discount": map[string]any{
							"fraction": 0.4,
							"include": map[string]any{
								"items": []any{"item-1"},
							},
						},
					},
					"effect_item_badge": map[string]any{
						"text": "40% off selected items",
					},
				},
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			venuePageDynamicFunc: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				return dynamicPayload, nil
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "burger-place", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected 1 menu item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	basePrice := asMapPayload(t, first["base_price"])
	if asIntPayload(basePrice["amount"]) != 645 {
		t.Fatalf("expected discounted base_price amount 645, got %v", basePrice["amount"])
	}
	originalPrice := asMapPayload(t, first["original_price"])
	if asIntPayload(originalPrice["amount"]) != 1075 {
		t.Fatalf("expected original_price amount 1075, got %v", originalPrice["amount"])
	}
	discounts := asSlicePayload(t, first["discounts"])
	if len(discounts) != 1 || discounts[0] != "40% off selected items" {
		t.Fatalf("expected discounts [40%% off selected items], got %v", discounts)
	}
}

func TestVenueMenuForwardsAuthToDynamicRequest(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Steakhouse",
				"price": 1075,
			},
		},
	}
	dynamicPayload := map[string]any{
		"venue_raw": map[string]any{
			"discounts": []any{
				map[string]any{
					"effects": map[string]any{
						"item_discount": map[string]any{
							"fraction": 0.4,
							"include": map[string]any{
								"items": []any{"item-1"},
							},
						},
					},
					"effect_item_badge": map[string]any{
						"text": "40% off selected items",
					},
				},
			},
		},
	}
	seenToken := ""

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			venuePageDynamicFunc: func(_ context.Context, _ string, options woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				seenToken = options.Auth.WToken
				return dynamicPayload, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return assortmentPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 60.14889, Lon: 24.6911577},
			WToken:    "profile-token",
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "burger-place", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "profile-token" {
		t.Fatalf("expected dynamic request auth token profile-token, got %q", seenToken)
	}
}

func TestVenueCategoriesJSON(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"loading_strategy": "partial",
		"categories": []any{
			map[string]any{
				"id":       "cat-main",
				"name":     "Main",
				"slug":     "main",
				"item_ids": []any{},
				"subcategories": []any{
					map[string]any{
						"id":       "cat-main-burger",
						"name":     "Burgers",
						"slug":     "burgers",
						"item_ids": []any{"item-1", "item-2"},
					},
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "categories", "wolt-market-niittari", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	if data["loading_strategy"] != "partial" {
		t.Fatalf("expected loading_strategy partial, got %v", data["loading_strategy"])
	}
	categories := asSlicePayload(t, data["categories"])
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	first := asMapPayload(t, categories[0])
	second := asMapPayload(t, categories[1])
	if first["slug"] != "main" || second["slug"] != "burgers" {
		t.Fatalf("expected category slugs [main, burgers], got [%v, %v]", first["slug"], second["slug"])
	}
}

func TestVenueMenuPartialRequiresCategoryOrSearch(t *testing.T) {
	categoryCalls := 0
	venueContentCalls := 0
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"loading_strategy": "partial",
		"categories": []any{
			map[string]any{
				"id":       "cat-main",
				"name":     "Main",
				"slug":     "main",
				"item_ids": []any{},
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
			assortmentCategoryFn: func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error) {
				categoryCalls++
				return map[string]any{}, nil
			},
			venueContentBySlugFn: func(context.Context, string, string, woltgateway.AuthContext) (map[string]any, error) {
				venueContentCalls++
				return map[string]any{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "wolt-market-niittari", "--format", "json")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	if categoryCalls != 0 {
		t.Fatalf("expected no category endpoint calls without --category, got %d", categoryCalls)
	}
	if venueContentCalls != 0 {
		t.Fatalf("expected no venue-content calls without --category, got %d", venueContentCalls)
	}
	payload := mustJSON(t, out)
	errorPayload := asMapPayload(t, payload["error"])
	if errorPayload["code"] != "WOLT_INVALID_ARGUMENT" {
		t.Fatalf("expected WOLT_INVALID_ARGUMENT, got %v", errorPayload["code"])
	}
	message, _ := errorPayload["message"].(string)
	if !strings.Contains(message, "wolt venue categories wolt-market-niittari") {
		t.Fatalf("expected category guidance in error message, got %q", message)
	}
	if !strings.Contains(message, "wolt venue search wolt-market-niittari --query <text>") {
		t.Fatalf("expected venue search guidance in error message, got %q", message)
	}
}

func TestVenueSearchScopedByVenue(t *testing.T) {
	searchCalls := 0
	searchQuery := ""
	searchLanguage := ""
	searchSlug := ""

	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	searchPayload := map[string]any{
		"categories": []any{
			map[string]any{"id": "dairy", "name": "Dairy", "item_ids": []any{"item-1"}},
			map[string]any{"id": "bakery", "name": "Bakery", "item_ids": []any{"item-2"}},
		},
		"items": []any{
			map[string]any{
				"id":               "item-1",
				"name":             "Milk 1L",
				"price":            map[string]any{"amount": 199, "currency": "EUR"},
				"category_name":    "Dairy",
				"is_sold_out":      false,
				"promotions":       []any{"10% off"},
				"option_group_ids": []any{"opt-1"},
			},
			map[string]any{
				"id":            "item-2",
				"name":          "Bread",
				"price":         map[string]any{"amount": 249, "currency": "EUR"},
				"category_name": "Bakery",
				"is_sold_out":   true,
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentItemsSearchFn: func(
				_ context.Context,
				slug string,
				query string,
				language string,
				_ woltgateway.AuthContext,
			) (map[string]any, error) {
				searchCalls++
				searchSlug = slug
				searchQuery = query
				searchLanguage = language
				return searchPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"venue",
		"search",
		"wolt-market-niittari",
		"--query",
		"milk",
		"--category",
		"dairy",
		"--include-options",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if searchCalls != 1 {
		t.Fatalf("expected one venue search call, got %d", searchCalls)
	}
	if searchSlug != "wolt-market-niittari" {
		t.Fatalf("unexpected venue slug %q", searchSlug)
	}
	if searchQuery != "milk" {
		t.Fatalf("unexpected search query %q", searchQuery)
	}
	if searchLanguage != "en" {
		t.Fatalf("expected language en, got %q", searchLanguage)
	}

	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue id venue-1, got %v", data["venue_id"])
	}
	if data["query"] != "milk" {
		t.Fatalf("expected query milk, got %v", data["query"])
	}
	if data["total"] != float64(1) {
		t.Fatalf("expected total 1, got %v", data["total"])
	}
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	if first["item_id"] != "item-1" {
		t.Fatalf("expected item-1, got %v", first["item_id"])
	}
	if first["category"] != "Dairy" {
		t.Fatalf("expected Dairy category, got %v", first["category"])
	}
	basePrice := asMapPayload(t, first["base_price"])
	if basePrice["amount"] != float64(199) {
		t.Fatalf("expected amount 199, got %v", basePrice["amount"])
	}
	if basePrice["currency"] != "EUR" {
		t.Fatalf("expected currency EUR, got %v", basePrice["currency"])
	}
	formattedAmount, _ := basePrice["formatted_amount"].(string)
	if !strings.Contains(formattedAmount, "1.99") {
		t.Fatalf("expected formatted amount containing 1.99, got %v", basePrice["formatted_amount"])
	}
	if len(asSlicePayload(t, first["discounts"])) == 0 {
		t.Fatalf("expected discounts payload for item, got %v", first["discounts"])
	}
	if len(asSlicePayload(t, first["option_group_ids"])) != 1 {
		t.Fatalf("expected option group ids in output, got %v", first["option_group_ids"])
	}
}

func TestVenueSearchFillsCurrencyAndDerivedDiscount(t *testing.T) {
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id":       "venue-1",
			"currency": "EUR",
		},
	}
	searchPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":             "3812374682d6e1eb42b3fd3e",
				"name":           "Coca-Cola Zero 6-pack",
				"price":          419,
				"original_price": 529,
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return staticPayload, nil
			},
			assortmentItemsSearchFn: func(
				context.Context,
				string,
				string,
				string,
				woltgateway.AuthContext,
			) (map[string]any, error) {
				return searchPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"venue",
		"search",
		"wolt-market-niittari",
		"--query",
		"Coca-Cola Zero 0,33 6-pack",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	basePrice := asMapPayload(t, first["base_price"])
	if basePrice["currency"] != "EUR" {
		t.Fatalf("expected fallback currency EUR, got %v", basePrice["currency"])
	}
	baseFormatted, _ := basePrice["formatted_amount"].(string)
	if !strings.Contains(baseFormatted, "4.19") {
		t.Fatalf("expected base formatted amount to contain 4.19, got %v", basePrice["formatted_amount"])
	}
	originalPrice := asMapPayload(t, first["original_price"])
	if originalPrice["currency"] != "EUR" {
		t.Fatalf("expected original price currency EUR, got %v", originalPrice["currency"])
	}
	originalFormatted, _ := originalPrice["formatted_amount"].(string)
	if !strings.Contains(originalFormatted, "5.29") {
		t.Fatalf("expected original formatted amount to contain 5.29, got %v", originalPrice["formatted_amount"])
	}
	discounts := asSlicePayload(t, first["discounts"])
	if len(discounts) == 0 || !strings.Contains(strings.ToLower(asStringPayload(discounts[0])), "off") {
		t.Fatalf("expected derived discount label, got %v", discounts)
	}
}

func TestVenueMenuCategoryLoadsSelectedCategory(t *testing.T) {
	categoryCalls := []string{}
	staticPayload := map[string]any{
		"venue": map[string]any{
			"id": "venue-1",
		},
	}
	assortmentPayload := map[string]any{
		"loading_strategy": "partial",
		"categories": []any{
			map[string]any{
				"id":       "cat-bakery",
				"name":     "Bakery",
				"slug":     "bakery",
				"item_ids": []any{},
			},
		},
	}
	categoryPayload := map[string]any{
		"category": map[string]any{
			"id":   "cat-bakery",
			"name": "Bakery",
			"slug": "bakery",
		},
		"categories": []any{
			map[string]any{
				"id":       "cat-bakery",
				"item_ids": []any{"item-1"},
			},
		},
	}
	itemsPayload := map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-1",
				"name":  "Sourdough Bread",
				"price": 399,
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
			assortmentCategoryFn: func(_ context.Context, _ string, categorySlug string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
				categoryCalls = append(categoryCalls, categorySlug)
				return categoryPayload, nil
			},
			assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
				return itemsPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "menu", "wolt-market-niittari", "--category", "bakery", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if len(categoryCalls) != 1 || categoryCalls[0] != "bakery" {
		t.Fatalf("expected one category call with bakery, got %v", categoryCalls)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one category item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	if first["item_id"] != "item-1" {
		t.Fatalf("expected item_id item-1, got %v", first["item_id"])
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

func containsStringPayload(values []any, expected string) bool {
	for _, raw := range values {
		if strings.TrimSpace(asStringPayload(raw)) == strings.TrimSpace(expected) {
			return true
		}
	}
	return false
}

var _ cli.ConfigManager = (*recordingConfig)(nil)
var _ cli.LocationResolver = (*recordingLocation)(nil)
