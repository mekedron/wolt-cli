package mcpserver

import (
	"context"
	"strings"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestVenueMenuSchemaRequiresLeafCategorySlug(t *testing.T) {
	_, client := connectInMemory(t, Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	found := false
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		if tool.Name != "wolt_venue_menu" {
			continue
		}
		found = true
		properties := asMap(asMap(tool.InputSchema)["properties"])
		categorySchema := asMap(properties["category"])
		description := asString(categorySchema["description"])
		if !strings.Contains(description, "leaf category slug") {
			t.Fatalf("category description = %q", description)
		}
	}
	if !found {
		t.Fatal("tool wolt_venue_menu was not listed")
	}
}

func TestVenueMenuReportsPartialCatalogMetadataInsteadOfZeroItems(t *testing.T) {
	wolt := partialCatalogStub(t)
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue: "000000000000000000000001",
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if strings.TrimSpace(out.Summary) == "0 items" || !strings.HasPrefix(out.Summary, "partial catalog:") {
		t.Fatalf("summary = %q", out.Summary)
	}
	catalog := asMap(out.Data["catalog"])
	if catalog["status"] != "partial" || catalog["complete"] != false || catalog["selection"] != "metadata_only" {
		t.Fatalf("catalog = %#v", catalog)
	}
	available := asSlice(catalog["available_categories"])
	if len(available) != 2 {
		t.Fatalf("available_categories = %#v", available)
	}
	if len(asSlice(out.Data["categories"])) != 2 {
		t.Fatalf("categories = %#v", out.Data["categories"])
	}
	if !containsWarning(out.Warnings, "pass category=<leaf slug>") {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestVenueMenuLoadsPartialCategoryThroughResolvedSlug(t *testing.T) {
	wolt := partialCatalogStub(t)
	categoryCalls := 0
	wolt.assortmentCategoryFn = func(
		_ context.Context,
		slug string,
		category string,
		language string,
		_ woltgateway.AuthContext,
	) (map[string]any, error) {
		categoryCalls++
		if slug != "partial-catalog-market" || category != "fish" || language != "en" {
			t.Fatalf("category request = slug %q category %q language %q", slug, category, language)
		}
		return map[string]any{"category": map[string]any{
			"id":       "cat-fish",
			"slug":     "fish",
			"name":     "Fish",
			"item_ids": []any{"item-1", "item-2"},
		}}, nil
	}
	wolt.assortmentItemsFn = func(
		_ context.Context,
		slug string,
		itemIDs []string,
		_ woltgateway.AuthContext,
	) (map[string]any, error) {
		if slug != "partial-catalog-market" || len(itemIDs) != 2 {
			t.Fatalf("item hydration = slug %q ids %v", slug, itemIDs)
		}
		return map[string]any{
			"currency": "EUR",
			"items": []any{
				map[string]any{"id": "item-1", "name": "Mackerel", "price": 1890},
				map[string]any{"id": "item-2", "name": "Salmon", "price": 2590},
			},
		}, nil
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue:    "000000000000000000000001",
		Category: "fish",
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if categoryCalls != 1 {
		t.Fatalf("category calls = %d, want 1", categoryCalls)
	}
	if got := len(asSlice(out.Data["items"])); got != 2 {
		t.Fatalf("items = %d, want 2 (%#v)", got, out.Data["items"])
	}
	catalog := asMap(out.Data["catalog"])
	if catalog["status"] != "partial" || catalog["selection"] != "category" || catalog["selection_complete"] != true {
		t.Fatalf("catalog = %#v", catalog)
	}
	loaded := asSlice(catalog["loaded_category_slugs"])
	if len(loaded) != 1 || loaded[0] != "fish" {
		t.Fatalf("loaded_category_slugs = %#v", loaded)
	}
}

func TestVenueMenuSelectionsUseEndpointItemsAndRootMetadata(t *testing.T) {
	const (
		venueID       = "000000000000000000000001"
		venueSlug     = "selection-market"
		selectedID    = "item-selected"
		outOfScopeID  = "item-out-of-scope"
		campaignLabel = "25% off selected items"
	)
	rootPayload := map[string]any{
		"currency":  "EUR",
		"wolt_plus": true,
		"categories": []any{
			map[string]any{
				"id":       "cat-selected",
				"slug":     "selected",
				"name":     "Selected",
				"item_ids": []any{selectedID},
			},
			map[string]any{
				"id":       "cat-other",
				"slug":     "other",
				"name":     "Other",
				"item_ids": []any{outOfScopeID},
			},
		},
		"items": []any{
			map[string]any{"id": selectedID, "name": "Stale root copy", "price": 400},
			map[string]any{"id": outOfScopeID, "name": "Out-of-scope root item", "price": 700},
		},
		"venue_raw": map[string]any{
			"discounts": []any{
				map[string]any{
					"effects": map[string]any{
						"item_discount": map[string]any{
							"fraction": 0.25,
							"include":  map[string]any{"items": []any{selectedID}},
						},
					},
					"effect_item_badge": map[string]any{"text": campaignLabel},
				},
			},
		},
	}
	selectedPayload := map[string]any{
		"category": map[string]any{
			"id":       "cat-selected",
			"slug":     "selected",
			"name":     "Selected",
			"item_ids": []any{selectedID},
		},
		"items": []any{
			map[string]any{"id": selectedID, "name": "Current endpoint copy", "price": 1200},
		},
	}

	tests := []struct {
		name          string
		input         VenueMenuInput
		wantSelection string
	}{
		{
			name:          "category",
			input:         VenueMenuInput{Venue: venueID, Category: "selected"},
			wantSelection: "category",
		},
		{
			name:          "query",
			input:         VenueMenuInput{Venue: venueID, Query: "current"},
			wantSelection: "search",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{"venue": map[string]any{
						"id":   venueID,
						"slug": venueSlug,
					}}, nil
				},
				assortmentFn: func(context.Context, string) (map[string]any, error) {
					return rootPayload, nil
				},
				assortmentCategoryFn: func(
					context.Context,
					string,
					string,
					string,
					woltgateway.AuthContext,
				) (map[string]any, error) {
					return selectedPayload, nil
				},
				assortmentSearchFn: func(
					context.Context,
					string,
					string,
					string,
					woltgateway.AuthContext,
				) (map[string]any, error) {
					return selectedPayload, nil
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

			_, out, err := tc.handleVenueMenu(context.Background(), nil, test.input)
			if err != nil {
				t.Fatalf("handleVenueMenu: %v", err)
			}
			items := asSlice(out.Data["items"])
			if len(items) != 1 {
				t.Fatalf("items = %#v, want only the endpoint-selected item", items)
			}
			item := asMap(items[0])
			if item["item_id"] != selectedID || item["name"] != "Current endpoint copy" {
				t.Fatalf("selected item = %#v", item)
			}
			basePrice := asMap(item["base_price"])
			originalPrice := asMap(item["original_price"])
			if basePrice["amount"] != 900 || basePrice["currency"] != "EUR" ||
				originalPrice["amount"] != 1200 {
				t.Fatalf("campaign-adjusted prices = base %#v original %#v", basePrice, originalPrice)
			}
			discounts := asSlice(item["discounts"])
			if len(discounts) != 1 || discounts[0] != campaignLabel {
				t.Fatalf("discounts = %#v", discounts)
			}
			if out.Data["wolt_plus"] != true || out.Data["currency"] != "EUR" {
				t.Fatalf("root metadata was not preserved: %#v", out.Data)
			}
			catalog := asMap(out.Data["catalog"])
			if catalog["status"] != "complete" ||
				catalog["complete"] != true ||
				catalog["selection"] != test.wantSelection {
				t.Fatalf("catalog = %#v", catalog)
			}
		})
	}
}

func TestVenueMenuEmptySelectedCategoryDoesNotExposeRootCategories(t *testing.T) {
	wolt := partialCatalogStub(t)
	wolt.assortmentCategoryFn = func(
		context.Context,
		string,
		string,
		string,
		woltgateway.AuthContext,
	) (map[string]any, error) {
		return map[string]any{"category": map[string]any{
			"id":   "cat-fish",
			"slug": "fish",
			"name": "Fish",
		}}, nil
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue:    "000000000000000000000001",
		Category: "fish",
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if len(asSlice(out.Data["items"])) != 0 || len(asSlice(out.Data["categories"])) != 0 {
		t.Fatalf("empty selected category leaked root data: %#v", out.Data)
	}
	catalog := asMap(out.Data["catalog"])
	if catalog["selection"] != "category" || catalog["selection_complete"] != true {
		t.Fatalf("catalog = %#v", catalog)
	}
	if !containsWarning(out.Warnings, `category "fish" returned no menu items`) {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestVenueMenuPartialQueryUsesItemSearchBackend(t *testing.T) {
	wolt := partialCatalogStub(t)
	searchCalls := 0
	wolt.assortmentSearchFn = func(
		_ context.Context,
		slug string,
		query string,
		language string,
		_ woltgateway.AuthContext,
	) (map[string]any, error) {
		searchCalls++
		if slug != "partial-catalog-market" || query != "mackerel" || language != "en" {
			t.Fatalf("search request = slug %q query %q language %q", slug, query, language)
		}
		return map[string]any{
			"currency": "EUR",
			"items": []any{
				map[string]any{"id": "item-1", "name": "Mackerel", "price": 1890},
			},
		}, nil
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue: "000000000000000000000001",
		Query: "mackerel",
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if searchCalls != 1 || len(asSlice(out.Data["items"])) != 1 {
		t.Fatalf("search calls=%d items=%#v", searchCalls, out.Data["items"])
	}
	catalog := asMap(out.Data["catalog"])
	if catalog["selection"] != "search" || catalog["selection_complete"] != true {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestVenueMenuCombinesCategoryAndQuery(t *testing.T) {
	wolt := partialCatalogStub(t)
	searchCalls := 0
	wolt.assortmentSearchFn = func(
		context.Context,
		string,
		string,
		string,
		woltgateway.AuthContext,
	) (map[string]any, error) {
		searchCalls++
		return nil, nil
	}
	wolt.assortmentCategoryFn = func(
		context.Context,
		string,
		string,
		string,
		woltgateway.AuthContext,
	) (map[string]any, error) {
		return map[string]any{
			"category": map[string]any{
				"id":       "leaf",
				"slug":     "leaf",
				"name":     "Leaf",
				"item_ids": []any{"item-1", "item-2"},
			},
		}, nil
	}
	wolt.assortmentItemsFn = func(
		context.Context,
		string,
		[]string,
		woltgateway.AuthContext,
	) (map[string]any, error) {
		return map[string]any{
			"currency": "EUR",
			"items": []any{
				map[string]any{
					"id":    "item-1",
					"name":  "冷凍魚",
					"price": 500,
					"translations": map[string]any{
						"fr": map[string]any{"name": "Poisson surgelé"},
					},
				},
				map[string]any{"id": "item-2", "name": "Beta item", "price": 600},
			},
		}, nil
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{
		Venue:    "000000000000000000000001",
		Category: "leaf",
		Query:    "poisson",
	})
	if err != nil {
		t.Fatalf("handleVenueMenu: %v", err)
	}
	if searchCalls != 0 {
		t.Fatalf("global search calls = %d, want 0", searchCalls)
	}
	items := asSlice(out.Data["items"])
	if len(items) != 1 || asString(asMap(items[0])["name"]) != "冷凍魚" {
		t.Fatalf("items = %#v", items)
	}
	catalog := asMap(out.Data["catalog"])
	if catalog["selection"] != "category" ||
		catalog["items_returned"] != 1 {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestVenueMenuCatalogStatusPrecedesUserFilteringAndPagination(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "example-venue"
	)
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{
					"id":       venueID,
					"slug":     venueSlug,
					"currency": "EUR",
				},
			}, nil
		},
		assortmentFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"currency": "EUR",
				"items": []any{
					map[string]any{"id": "item-1", "name": "Example item", "price": 500},
				},
			}, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	tests := map[string]VenueMenuInput{
		"query with no matches":  {Venue: venueID, Query: "missing"},
		"offset after last item": {Venue: venueID, Offset: 10},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, out, err := tc.handleVenueMenu(context.Background(), nil, input)
			if err != nil {
				t.Fatalf("handleVenueMenu: %v", err)
			}
			if len(asSlice(out.Data["items"])) != 0 {
				t.Fatalf("items = %#v, want empty page", out.Data["items"])
			}
			catalog := asMap(out.Data["catalog"])
			if catalog["status"] != "complete" || catalog["complete"] != true {
				t.Fatalf("catalog = %#v", catalog)
			}
			if name == "query with no matches" &&
				containsWarning(out.Warnings, "no menu items were discovered") {
				t.Fatalf("empty query result was reported as an upstream failure: %v", out.Warnings)
			}
		})
	}
}

func TestVenueMenuEmptyRootPreservesAuthoritativeSelectionCompleteness(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "empty-root-venue"
	)
	tests := []struct {
		name          string
		input         VenueMenuInput
		wantSelection string
	}{
		{
			name:          "search",
			input:         VenueMenuInput{Venue: venueID, Query: "result"},
			wantSelection: "search",
		},
		{
			name:          "category",
			input:         VenueMenuInput{Venue: venueID, Category: "requested-leaf"},
			wantSelection: "category",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selectedPayload := map[string]any{
				"items": []any{
					map[string]any{
						"id":    "selected-item",
						"name":  "Selected result",
						"price": 500,
					},
				},
			}
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{"venue": map[string]any{
						"id":   venueID,
						"slug": venueSlug,
					}}, nil
				},
				assortmentFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{}, nil
				},
				assortmentCategoryFn: func(
					context.Context,
					string,
					string,
					string,
					woltgateway.AuthContext,
				) (map[string]any, error) {
					return selectedPayload, nil
				},
				assortmentSearchFn: func(
					context.Context,
					string,
					string,
					string,
					woltgateway.AuthContext,
				) (map[string]any, error) {
					return selectedPayload, nil
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

			_, out, err := tc.handleVenueMenu(context.Background(), nil, test.input)
			if err != nil {
				t.Fatalf("handleVenueMenu: %v", err)
			}
			if got := len(asSlice(out.Data["items"])); got != 1 {
				t.Fatalf("items = %d, want 1: %#v", got, out.Data["items"])
			}
			catalog := asMap(out.Data["catalog"])
			if catalog["status"] != "unavailable" ||
				catalog["complete"] != false ||
				catalog["selection"] != test.wantSelection ||
				catalog["selection_complete"] != true {
				t.Fatalf("catalog = %#v", catalog)
			}
			if !containsWarning(out.Warnings, "catalog unavailable:") {
				t.Fatalf("warnings = %v, want explicit empty-root warning", out.Warnings)
			}
		})
	}
}

func TestVenueMenuClassifiesRootByMaterializedItems(t *testing.T) {
	const (
		venueID   = "000000000000000000000001"
		venueSlug = "catalog-shape-market"
	)
	tests := []struct {
		name                  string
		payload               map[string]any
		wantStatus            string
		wantComplete          bool
		wantSelection         string
		wantSelectionComplete bool
		wantItems             int
	}{
		{
			name: "unlabelled reference-only root",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":       "cat-one",
					"slug":     "one",
					"name":     "One",
					"item_ids": []any{"item-1"},
				}},
			},
			wantStatus:            "partial",
			wantSelection:         "metadata_only",
			wantSelectionComplete: false,
		},
		{
			name: "unlabelled categories-only root",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":   "cat-empty",
					"slug": "empty",
					"name": "Empty",
				}},
			},
			wantStatus:            "partial",
			wantSelection:         "metadata_only",
			wantSelectionComplete: false,
		},
		{
			name: "explicit partial root with preview row",
			payload: map[string]any{
				"loading_strategy": "partial",
				"categories": []any{map[string]any{
					"id":       "cat-one",
					"slug":     "one",
					"name":     "One",
					"item_ids": []any{"item-1", "item-2"},
				}},
				"items": []any{
					map[string]any{"id": "item-1", "name": "Preview item", "price": 500},
				},
			},
			wantStatus:            "partial",
			wantSelection:         "full",
			wantSelectionComplete: false,
			wantItems:             1,
		},
		{
			name: "nested materialized row",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":   "cat-nested",
					"slug": "nested",
					"name": "Nested",
					"items": []any{
						map[string]any{
							"id":       "item-nested",
							"name":     "Nested item",
							"price":    700,
							"category": "Nested",
						},
					},
				}},
			},
			wantStatus:            "complete",
			wantComplete:          true,
			wantSelection:         "full",
			wantSelectionComplete: true,
			wantItems:             1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{"venue": map[string]any{
						"id":   venueID,
						"slug": venueSlug,
					}}, nil
				},
				assortmentFn: func(context.Context, string) (map[string]any, error) {
					return test.payload, nil
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

			_, out, err := tc.handleVenueMenu(context.Background(), nil, VenueMenuInput{Venue: venueID})
			if err != nil {
				t.Fatalf("handleVenueMenu: %v", err)
			}
			if got := len(asSlice(out.Data["items"])); got != test.wantItems {
				t.Fatalf("items = %d, want %d (%#v)", got, test.wantItems, out.Data["items"])
			}
			catalog := asMap(out.Data["catalog"])
			if catalog["status"] != test.wantStatus ||
				catalog["complete"] != test.wantComplete ||
				catalog["selection"] != test.wantSelection ||
				catalog["selection_complete"] != test.wantSelectionComplete {
				t.Fatalf("catalog = %#v", catalog)
			}
		})
	}
}

func partialCatalogStub(t *testing.T) *stubWolt {
	t.Helper()
	return &stubWolt{
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{
				"id":       "000000000000000000000001",
				"slug":     "partial-catalog-market",
				"currency": "EUR",
			}}, nil
		},
		assortmentFn: func(_ context.Context, slug string) (map[string]any, error) {
			if slug != "partial-catalog-market" {
				t.Fatalf("assortment slug = %q", slug)
			}
			return map[string]any{
				"loading_strategy": "partial",
				"currency":         "EUR",
				"categories": []any{
					map[string]any{
						"id":   "cat-fresh",
						"slug": "fresh",
						"name": "Fresh",
						"subcategories": []any{
							map[string]any{
								"id":       "cat-fish",
								"slug":     "fish",
								"name":     "Fish",
								"item_ids": []any{"item-1", "item-2"},
							},
						},
					},
				},
			}, nil
		},
	}
}
