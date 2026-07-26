package e2e_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/cli"
	configstore "github.com/mekedron/wolt-cli/internal/config"
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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "categories", "--lat", "50.0", "--lon", "19.0", "--format", "json")
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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "--format", "json")
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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "--query", "burger", "--enrich", "--format", "json")
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

func TestFeedRendersSectionedVenuesWithTaglineAndTopOffer(t *testing.T) {
	woltPlusTrue := true
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular near you",
			Items: []domain.Item{
				{
					Title: "Bastard Burgers",
					Link:  domain.Link{Target: "venue-1"},
					Venue: &domain.Venue{
						ID:               "venue-1",
						Slug:             "bastard-burgers-mikonkatu",
						Name:             "Bastard Burgers Mikonkatu",
						ShortDescription: "Like a Bastard™",
						Tags:             []string{"burger"},
						Promotions: []any{
							map[string]any{"text": "14 days of €0 delivery fees", "variant": "primary"},
							map[string]any{"text": "20% off selected items", "variant": "discount"},
						},
						Rating:           &domain.Rating{Score: 8.6},
						DeliveryPriceInt: intPtr(0),
						Currency:         "EUR",
						EstimateRange:    "15-25",
						ShowWoltPlus:     true,
					},
				},
			},
		},
		{
			Name:  "fastest-delivery",
			Title: "Fastest delivery",
			Items: []domain.Item{
				{
					Title: "Kotipizza Kamppi",
					Link:  domain.Link{Target: "venue-2"},
					Venue: &domain.Venue{
						ID:                 "venue-2",
						Slug:               "kotipizza-kamppi",
						Name:               "Kotipizza Kamppi",
						ShortDescriptionV2: &domain.Translation{Lang: "fi", Value: "Kuuma, kuumempi, Kotipizza"},
						Tags:               []string{"pizza"},
						Promotions:         []any{map[string]any{"text": "Buy 2 meals pay 1", "variant": "discount"}},
						Rating:             &domain.Rating{Score: 8.4},
						DeliveryPriceInt:   intPtr(0),
						Currency:           "EUR",
						EstimateRange:      "10-20",
						Online:             &woltPlusTrue,
					},
				},
			},
		},
	}

	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionsOut := asSlicePayload(t, data["sections"])
	if len(sectionsOut) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sectionsOut))
	}
	first := asMapPayload(t, sectionsOut[0])
	if first["title"] != "Popular near you" {
		t.Fatalf("expected first section title 'Popular near you', got %v", first["title"])
	}
	firstItems := asSlicePayload(t, first["items"])
	firstRow := asMapPayload(t, firstItems[0])
	if firstRow["tagline"] != "Like a Bastard™" {
		t.Fatalf("expected tagline to be surfaced, got %v", firstRow["tagline"])
	}
	if firstRow["top_offer"] != "20% off selected items" {
		t.Fatalf("expected discount-variant promo to win as top_offer, got %v", firstRow["top_offer"])
	}

	second := asMapPayload(t, sectionsOut[1])
	secondItems := asSlicePayload(t, second["items"])
	secondRow := asMapPayload(t, secondItems[0])
	if secondRow["tagline"] != "Kuuma, kuumempi, Kotipizza" {
		t.Fatalf("expected localized tagline from short_description_v2, got %v", secondRow["tagline"])
	}
}

func TestSearchVenuesSkipsEnrichmentByDefault(t *testing.T) {
	enrichmentCalled := false
	items := []domain.Item{
		{Title: "Burger Place", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: buildVenue("venue-1", "burger-place", "Burger Street")},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			itemsFunc: func(context.Context, domain.Location) ([]domain.Item, error) {
				return items, nil
			},
			venuePageDynamicFunc: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
				enrichmentCalled = true
				return map[string]any{}, nil
			},
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				enrichmentCalled = true
				return map[string]any{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, _ := runCLIWithDeps(t, deps, "venues", "--query", "burger", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if enrichmentCalled {
		t.Fatal("expected default venues call to skip per-venue enrichment; got upstream call")
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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "--open-now", "--query", "groceries")
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

func TestSearchVenuesHighlightsOptInColumn(t *testing.T) {
	venue := buildVenue("venue-1", "burger-place", "Burger Street")
	venue.BadgesV2 = []domain.Badge{{Icon: "wolt-plus", Variant: "primary", Text: "Wolt+"}}
	venue.PreviewItems = []any{map[string]any{"name": "Cheeseburger pizza", "formatted_price": "19.90 €"}}
	items := []domain.Item{{Title: "Burger Place", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue}}

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

	// Default (no --show-highlights flag) — auto mode shows the column
	// because this venue has menu_highlights data.
	exitCodeDefault, outDefault := runCLIWithDeps(t, deps, "venues", "--query", "burger")
	if exitCodeDefault != 0 {
		t.Fatalf("expected exit 0 (default), got %d\noutput:\n%s", exitCodeDefault, outDefault)
	}
	if !strings.Contains(outDefault, "Highlights") {
		t.Fatalf("expected Highlights column auto-shown when row has data, got:\n%s", outDefault)
	}
	if !strings.Contains(outDefault, "Cheeseburger pizza 19.90 €") {
		t.Fatalf("expected highlight value in cell, got:\n%s", outDefault)
	}
	if !strings.Contains(outDefault, "+ Burger Place") {
		t.Fatalf("expected Wolt+ glyph prefix on venue cell, got:\n%s", outDefault)
	}

	// --show-highlights=false force-hides even when data is present.
	exitCodeOff, outOff := runCLIWithDeps(t, deps, "venues", "--query", "burger", "--show-highlights=false")
	if exitCodeOff != 0 {
		t.Fatalf("expected exit 0 (--show-highlights=false), got %d\noutput:\n%s", exitCodeOff, outOff)
	}
	if strings.Contains(outOff, "Highlights") {
		t.Fatalf("expected Highlights column hidden when force-off, got:\n%s", outOff)
	}
}

func TestSearchVenuesHighlightsHiddenWhenDataAbsent(t *testing.T) {
	plain := buildVenue("venue-2", "plain-place", "Plain Street")
	items := []domain.Item{{Title: "Plain Place", TrackID: "1", Link: domain.Link{Target: "venue-2"}, Venue: plain}}

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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "--query", "plain")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if strings.Contains(out, "Highlights") {
		t.Fatalf("expected Highlights column hidden in auto mode when no data, got:\n%s", out)
	}

	exitCodeOn, outOn := runCLIWithDeps(t, deps, "venues", "--query", "plain", "--show-highlights")
	if exitCodeOn != 0 {
		t.Fatalf("expected exit 0 with --show-highlights, got %d\noutput:\n%s", exitCodeOn, outOn)
	}
	if !strings.Contains(outOn, "Highlights") {
		t.Fatalf("expected Highlights column forced on with --show-highlights, got:\n%s", outOn)
	}
}

func TestSearchVenuesBadgePlainModeFallback(t *testing.T) {
	t.Setenv("WOLT_BADGES_PLAIN", "1")
	venue := buildVenue("venue-1", "burger-place", "Burger Street")
	venue.BadgesV2 = []domain.Badge{{Icon: "wolt-plus", Variant: "primary", Text: "Wolt+"}}
	items := []domain.Item{{Title: "Burger Place", TrackID: "1", Link: domain.Link{Target: "venue-1"}, Venue: venue}}

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

	exitCode, out := runCLIWithDeps(t, deps, "venues", "--query", "burger")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "[Wolt+] Burger Place") {
		t.Fatalf("expected plain-mode bracketed prefix, got:\n%s", out)
	}
}

func TestFeedTableAutoHidesEmptyHighlightsColumn(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{
					Title: "Plain Venue",
					Link:  domain.Link{Target: "venue-plain"},
					Venue: &domain.Venue{ID: "venue-plain", Slug: "plain", Currency: "EUR", DeliveryPriceInt: intPtr(0), EstimateRange: "10-20"},
				},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if strings.Contains(out, "Highlights") {
		t.Fatalf("expected Highlights column hidden when no row has data, got:\n%s", out)
	}

	exitCodeForced, outForced := runCLIWithDeps(t, deps, "feed", "--show-highlights")
	if exitCodeForced != 0 {
		t.Fatalf("expected exit 0 with --show-highlights, got %d\noutput:\n%s", exitCodeForced, outForced)
	}
	if !strings.Contains(outForced, "Highlights") {
		t.Fatalf("expected Highlights column forced on with --show-highlights, got:\n%s", outForced)
	}
}

func TestFeedTableShowsHighlightsByDefault(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{
					Title: "Featured Venue",
					Link:  domain.Link{Target: "venue-1"},
					Venue: &domain.Venue{
						ID:               "venue-1",
						Slug:             "featured",
						Name:             "Featured Venue",
						Currency:         "EUR",
						DeliveryPriceInt: intPtr(0),
						EstimateRange:    "10-20",
						BadgesV2: []domain.Badge{
							{Icon: "coupon-fill", Variant: "discount", Text: "20% off"},
						},
						PreviewItems: []any{map[string]any{"name": "Cheeseburger pizza", "formatted_price": "19.90 €"}},
					},
				},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Highlights") {
		t.Fatalf("expected Highlights column on feed by default, got:\n%s", out)
	}
	if !strings.Contains(out, "Cheeseburger pizza 19.90 €") {
		t.Fatalf("expected highlight value in cell, got:\n%s", out)
	}
	if !strings.Contains(out, "% Featured Venue") {
		t.Fatalf("expected discount glyph prefix on venue cell, got:\n%s", out)
	}

	exitCodeOff, outOff := runCLIWithDeps(t, deps, "feed", "--show-highlights=false")
	if exitCodeOff != 0 {
		t.Fatalf("expected exit 0 with highlights off, got %d\noutput:\n%s", exitCodeOff, outOff)
	}
	if strings.Contains(outOff, "Highlights") {
		t.Fatalf("expected Highlights column hidden when --show-highlights=false, got:\n%s", outOff)
	}
}

func TestTopFlattensFeedAndDedupes(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "dinner",
			Title: "Dinner near you",
			Items: []domain.Item{
				{Title: "Noodle Story", Link: domain.Link{Target: "v1"}, Venue: &domain.Venue{ID: "v1", Slug: "noodle-story", Currency: "EUR"}},
				{Title: "Putte's", Link: domain.Link{Target: "v2"}, Venue: &domain.Venue{ID: "v2", Slug: "puttes", Currency: "EUR"}},
			},
		},
		{
			Name:  "brands",
			Title: "Brands",
			Items: []domain.Item{
				{Title: "K-Market", Link: domain.Link{Target: "k-market"}},
			},
		},
		{
			Name:  "fastest",
			Title: "Fastest delivery",
			Items: []domain.Item{
				// Duplicate of v1 — dedupe should drop it.
				{Title: "Noodle Story (duplicate)", Link: domain.Link{Target: "v1"}, Venue: &domain.Venue{ID: "v1", Slug: "noodle-story", Currency: "EUR"}},
				{Title: "KFC", Link: domain.Link{Target: "v3"}, Venue: &domain.Venue{ID: "v3", Slug: "kfc", Currency: "EUR"}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "top", "2", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	venues := asSlicePayload(t, data["venues"])
	if len(venues) != 2 {
		t.Fatalf("expected exactly 2 venues, got %d", len(venues))
	}
	if asMapPayload(t, venues[0])["venue_id"] != "v1" {
		t.Fatalf("expected first venue v1 from upstream order, got %v", venues[0])
	}
	if asMapPayload(t, venues[1])["venue_id"] != "v2" {
		t.Fatalf("expected second venue v2, got %v", venues[1])
	}
	limit, _ := data["limit"].(float64)
	if int(limit) != 2 {
		t.Fatalf("expected limit=2 in payload, got %v", data["limit"])
	}
}

func TestTopExcludesBrandSections(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "Wolt Market", Link: domain.Link{Target: "wm"}},
				{Title: "K-Market", Link: domain.Link{Target: "km"}},
			},
		},
		{
			Name:  "dinner",
			Title: "Dinner",
			Items: []domain.Item{
				{Title: "Putte's", Link: domain.Link{Target: "v1"}, Venue: &domain.Venue{ID: "v1", Slug: "puttes", Currency: "EUR"}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "top", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	venues := asSlicePayload(t, asMapPayload(t, mustJSON(t, out)["data"])["venues"])
	if len(venues) != 1 {
		t.Fatalf("expected only venue sections to contribute, got %d venues", len(venues))
	}
	if asMapPayload(t, venues[0])["slug"] != "puttes" {
		t.Fatalf("expected the venue-section entry, got %v", venues[0])
	}
}

func TestTopRejectsInvalidN(t *testing.T) {
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
	exitCode, out := runCLIWithDeps(t, deps, "top", "fish")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for non-integer N, got %d:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "invalid N") {
		t.Fatalf("expected friendly invalid-N message, got:\n%s", out)
	}
}

func TestVenuesCategoriesSupportsPagination(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				// Many tags → many distinct categories.
				{Title: "Burger Spot", Link: domain.Link{Target: "v1"}, Venue: &domain.Venue{ID: "v1", Tags: []string{"american", "burger", "fast food"}}},
				{Title: "Sushi Bar", Link: domain.Link{Target: "v2"}, Venue: &domain.Venue{ID: "v2", Tags: []string{"sushi", "japanese"}}},
				{Title: "Pizza Place", Link: domain.Link{Target: "v3"}, Venue: &domain.Venue{ID: "v3", Tags: []string{"pizza", "italian"}}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venues", "categories", "--limit", "3", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	categories := asSlicePayload(t, data["categories"])
	if len(categories) != 3 {
		t.Fatalf("expected 3 categories (limit=3), got %d", len(categories))
	}
	totalAny := data["total"]
	totalFloat, _ := totalAny.(float64)
	if int(totalFloat) < 7 {
		t.Fatalf("expected total >= 7 (all distinct tags), got %v", totalAny)
	}
	pagesAny := data["total_pages"]
	pages, _ := pagesAny.(float64)
	if int(pages) < 3 {
		t.Fatalf("expected total_pages >= 3 for limit=3 across 7+ categories, got %v", pagesAny)
	}

	// Page 2 should return a different slice.
	exitCode, outPage2 := runCLIWithDeps(t, deps, "venues", "categories", "--limit", "3", "--page", "2", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0 on page 2, got %d\noutput:\n%s", exitCode, outPage2)
	}
	page2 := asMapPayload(t, mustJSON(t, outPage2)["data"])
	if int(page2["page"].(float64)) != 2 {
		t.Fatalf("expected page=2 echoed, got %v", page2["page"])
	}
	if int(page2["offset"].(float64)) != 3 {
		t.Fatalf("expected offset=3 derived from --page 2 --limit 3, got %v", page2["offset"])
	}
}

func TestVenuesCategoriesTableRendersRows(t *testing.T) {
	sections := []domain.Section{
		{
			Name: "popular",
			Items: []domain.Item{
				{Title: "Burger Spot", Link: domain.Link{Target: "v1"}, Venue: &domain.Venue{ID: "v1", Tags: []string{"burger"}}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
	exitCode, out := runCLIWithDeps(t, deps, "venues", "categories")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Burger") {
		t.Fatalf("expected category row to render with name, got:\n%s", out)
	}
	if !strings.Contains(out, "burger") {
		t.Fatalf("expected category row to render with slug, got:\n%s", out)
	}
}

func TestFeedSummaryFlagPrintsOneLinePerSection(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "FIN_NV_SB_popular-stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "Wolt Market", Link: domain.Link{Target: "woltmarket"}},
				{Title: "K-Market", Link: domain.Link{Target: "k-market"}},
				{Title: "Lidl", Link: domain.Link{Target: "lidl"}},
				{Title: "Alepa", Link: domain.Link{Target: "alepa"}},
			},
		},
		{
			Name:  "dinner-venues",
			Title: "Dinner near you",
			Items: []domain.Item{
				{Title: "Noodle Story Kamppi", Link: domain.Link{Target: "venue-1"}, Venue: &domain.Venue{ID: "venue-1", Slug: "noodle-story", Currency: "EUR"}},
				{Title: "Putte's Bar & Pizza", Link: domain.Link{Target: "venue-2"}, Venue: &domain.Venue{ID: "venue-2", Slug: "puttes", Currency: "EUR"}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed", "--summary")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Feed summary") {
		t.Fatalf("expected Feed summary title, got:\n%s", out)
	}
	for _, want := range []string{"Section", "Kind", "Count", "Top items"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected summary header %q, got:\n%s", want, out)
		}
	}
	// Brand section line carries kind=brands, count=4, top-3 names + ellipsis.
	if !strings.Contains(out, "Popular stores") || !strings.Contains(out, "brands") {
		t.Fatalf("expected brand section row, got:\n%s", out)
	}
	if !strings.Contains(out, "Wolt Market · K-Market · Lidl · …") {
		t.Fatalf("expected top-3 brand names with ellipsis, got:\n%s", out)
	}
	// Venue section: kind=venues, count=2, no ellipsis since len<=3.
	if !strings.Contains(out, "Dinner near you") || !strings.Contains(out, "venues") {
		t.Fatalf("expected venue section row, got:\n%s", out)
	}
	if !strings.Contains(out, "Noodle Story Kamppi · Putte's Bar & Pizza") {
		t.Fatalf("expected joined venue names, got:\n%s", out)
	}
	// Per-section venue tables should not appear in summary mode.
	if strings.Contains(out, "Tagline") {
		t.Fatalf("summary mode should not render per-section venue tables, got:\n%s", out)
	}
}

func TestFeedRendersBrandCarouselAsOneLiner(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "FIN_NV_SB_popular-stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "Wolt Market", Link: domain.Link{Target: "woltmarket-popular-brands:helsinki"}},
				{Title: "K-Market", Link: domain.Link{Target: "k-market:helsinki"}},
				{Title: "Lidl", Link: domain.Link{Target: "lidl:helsinki"}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Popular stores") {
		t.Fatalf("expected section title in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Wolt Market · K-Market · Lidl") {
		t.Fatalf("expected one-line brand summary, got:\n%s", out)
	}
	if strings.Contains(out, "(no venues)") {
		t.Fatalf("brand-only feed should not collapse to '(no venues)', got:\n%s", out)
	}
}

func TestFeedQueryMatchesBrandSection(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "stores",
			Title: "Popular stores",
			Items: []domain.Item{
				{Title: "K-Market", Link: domain.Link{Target: "k-market:helsinki"}},
				{Title: "Lidl", Link: domain.Link{Target: "lidl:helsinki"}},
			},
		},
		{
			Name:  "dinner-venues",
			Title: "Dinner",
			Items: []domain.Item{
				{Title: "Bastard Burgers", Link: domain.Link{Target: "venue-1"}, Venue: &domain.Venue{ID: "venue-1", Slug: "bastard", Currency: "EUR"}},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed", "--query", "lidl", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionsOut := asSlicePayload(t, data["sections"])
	if len(sectionsOut) != 1 {
		t.Fatalf("expected only the matching brand section, got %d sections:\n%s", len(sectionsOut), out)
	}
	first := asMapPayload(t, sectionsOut[0])
	if first["kind"] != "brands" {
		t.Fatalf("expected kind 'brands', got %v", first["kind"])
	}
	brands := asSlicePayload(t, first["brands"])
	if len(brands) != 1 || asMapPayload(t, brands[0])["name"] != "Lidl" {
		t.Fatalf("expected only Lidl to survive --query lidl, got %v", brands)
	}
}

func TestFeedJSONIncludesBadgesAndMenuHighlights(t *testing.T) {
	sections := []domain.Section{
		{
			Name:  "popular",
			Title: "Popular",
			Items: []domain.Item{
				{
					Title: "Featured Venue",
					Link:  domain.Link{Target: "venue-1"},
					Venue: &domain.Venue{
						ID:       "venue-1",
						Slug:     "featured",
						Currency: "EUR",
						BadgesV2: []domain.Badge{{Icon: "wolt-plus", Variant: "primary", Text: "Wolt+"}},
						PreviewItems: []any{
							map[string]any{"name": "Cheeseburger pizza", "formatted_price": "19.90 €"},
						},
					},
				},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			sectionsFunc: func(context.Context, domain.Location) ([]domain.Section, error) {
				return sections, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "feed", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	sectionsOut := asSlicePayload(t, data["sections"])
	row := asMapPayload(t, asSlicePayload(t, asMapPayload(t, sectionsOut[0])["items"])[0])

	badges := asSlicePayload(t, row["badges"])
	if len(badges) != 1 {
		t.Fatalf("expected 1 badge, got %d", len(badges))
	}
	if asMapPayload(t, badges[0])["icon"] != "wolt-plus" {
		t.Fatalf("expected wolt-plus icon, got %v", badges[0])
	}

	highlights := asSlicePayload(t, row["menu_highlights"])
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	first := asMapPayload(t, highlights[0])
	if first["name"] != "Cheeseburger pizza" || first["formatted_price"] != "19.90 €" {
		t.Fatalf("unexpected highlight shape: %v", first)
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
			map[string]any{"id": "item-a", "name": "Alpha", "price": 700, "disabled_info": nil, "promotions": []any{"20% off"}},
			map[string]any{"id": "item-b", "name": "Beta", "price": 900, "disabled_info": nil, "promotions": []any{"10% off"}},
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
	if !strings.Contains(message, "wolt venue menu wolt-market-niittari --query <text>") {
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
				"disabled_info":    nil,
				"promotions":       []any{"10% off"},
				"option_group_ids": []any{"opt-1"},
				"unit_info":        "1 l",
				"images": []any{
					map[string]any{"url": "https://imageproxy.wolt.com/assets/milk", "blurhash": "blur"},
				},
			},
			map[string]any{
				"id":                  "item-2",
				"name":                "Bread",
				"price":               map[string]any{"amount": 249, "currency": "EUR"},
				"category_name":       "Bakery",
				"disabled_info":       map[string]any{"disable_text": "Sold out"},
				"purchasable_balance": 0,
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
		"venue", "menu",
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
	// docs/output-contract.md declares availability and image metadata on
	// VenueItemSearchResult rows; the CLI builds these rows itself, so it must
	// emit them rather than leaving the contract to the MCP surface alone.
	if first["is_available"] != true {
		t.Fatalf("expected is_available true, got %v", first["is_available"])
	}
	if first["image_url"] != "https://imageproxy.wolt.com/assets/milk" {
		t.Fatalf("expected image_url, got %v", first["image_url"])
	}
	if first["unit_info"] != "1 l" {
		t.Fatalf("expected unit_info, got %v", first["unit_info"])
	}
	if _, exists := first["purchasable_balance"]; !exists {
		t.Fatalf("expected purchasable_balance key in row: %v", first)
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
		"venue", "menu",
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
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				return e2eVenueStaticPayload(), nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "hours", e2eVenueSlug, "--timezone", "Europe/Helsinki", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["timezone"] != "Europe/Helsinki" {
		t.Fatalf("expected timezone override Europe/Helsinki, got %v", data["timezone"])
	}
	windows := asSlicePayload(t, data["opening_windows"])
	if len(windows) != 1 {
		t.Fatalf("expected 1 known opening window, got %d", len(windows))
	}
	first := asMapPayload(t, windows[0])
	if first["day"] != "monday" || first["open"] != "10:00" || first["close"] != "20:45" {
		t.Fatalf("unexpected monday window: %v", first)
	}
}

func TestVenueHoursDerivesKnownWindowsFromStaticPayload(t *testing.T) {
	staticPayload := e2eVenueStaticPayload()
	openingTimes := asMapPayload(t, asMapPayload(t, staticPayload["venue_raw"])["opening_times"])
	openingTimes["monday"] = []any{
		map[string]any{"type": "open", "value": float64(39600)},  // 11:00
		map[string]any{"type": "close", "value": float64(74700)}, // 20:45
	}
	openingTimes["saturday"] = []any{
		map[string]any{"type": "open", "value": float64(41400)},  // 11:30
		map[string]any{"type": "close", "value": float64(74700)}, // 20:45
	}
	staticCalls := 0
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(context.Context, string) (map[string]any, error) {
				staticCalls++
				return staticPayload, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "venue", "hours", e2eVenueSlug, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if staticCalls == 0 {
		t.Fatal("expected static venue-page payload fetch")
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	windows := asSlicePayload(t, data["opening_windows"])
	if len(windows) != 2 {
		t.Fatalf("expected 2 upstream-known windows, got %d", len(windows))
	}
	monday := asMapPayload(t, windows[0])
	if monday["day"] != "monday" || monday["open"] != "11:00" || monday["close"] != "20:45" {
		t.Fatalf("expected monday 11:00-20:45, got %v", monday)
	}
	saturday := asMapPayload(t, windows[1])
	if saturday["day"] != "saturday" || saturday["open"] != "11:30" || saturday["close"] != "20:45" {
		t.Fatalf("expected saturday open 11:30, got %v", saturday)
	}
}

func TestExpiredSessionRendersFriendlyAuthHint(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 401}
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 0, Lon: 0},
			WToken:    "stale-token",
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}
	exitCode, out := runCLIWithDeps(t, deps, "account", "--format", "json")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit on 401, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, `session expired`) || !strings.Contains(out, `wolt login`) {
		t.Fatalf("expected friendly upstream-401 hint, got:\n%s", out)
	}
	if !strings.Contains(out, "WOLT_AUTH_REQUIRED") {
		t.Fatalf("expected WOLT_AUTH_REQUIRED error code, got:\n%s", out)
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

	exitCode, out := runCLIWithDeps(t, deps, "venue", "item", "burger-place", "item-missing")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "was not found for venue slug") {
		t.Fatalf("expected not-found error message, got:\n%s", out)
	}
}

func TestConfigureCommandSavesProfile(t *testing.T) {
	cfg := &recordingConfig{loadErr: configstore.ErrConfigNotFound}
	loc := &recordingLocation{location: domain.Location{Lat: 60.1699, Lon: 24.9384}}
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 0, Lon: 0}}},
		Location: loc,
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "login", "--wtoken", "abc.def.ghi")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if !strings.Contains(out, "Logged in") {
		t.Fatalf("expected login summary, got:\n%s", out)
	}
	if loc.seenAddress != "" {
		t.Fatalf("did not expect location lookup during configure, got %q", loc.seenAddress)
	}
	if cfg.saved == nil || len(cfg.saved.Profiles) != 1 {
		t.Fatalf("expected saved config with one profile, got %+v", cfg.saved)
	}
	profile := cfg.saved.Profiles[0]
	if profile.Name != "default" || !profile.IsDefault {
		t.Fatalf("unexpected saved profile: %+v", profile)
	}
}

func TestConfigureCommandSavesNormalizedWToken(t *testing.T) {
	cfg := &recordingConfig{loadErr: configstore.ErrConfigNotFound}
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
		"login",
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
		"login",
		"--profile",
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
