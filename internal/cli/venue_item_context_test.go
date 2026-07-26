package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestVenueMenuQueryUsesSharedItemContractAndNestedURLContext(t *testing.T) {
	withIsolatedSlugCache(t)
	const (
		venueID   = "000000000000000000000051"
		venueSlug = "example-market"
		itemID    = "000000000000000000000052"
	)
	inputURL := "https://wolt.com/en/example/venue/example-market/categories/drinks"
	api := &testWoltAPI{
		venuePageStaticFn: func(_ context.Context, reference string) (map[string]any, error) {
			if reference != venueSlug {
				t.Fatalf("VenuePageStatic reference = %q, want %q", reference, venueSlug)
			}
			return map[string]any{
				"venue": map[string]any{
					"id":       venueID,
					"slug":     venueSlug,
					"currency": "EUR",
				},
			}, nil
		},
		assortmentSearchFn: func(
			_ context.Context,
			slug string,
			query string,
			_ string,
			_ woltgateway.AuthContext,
		) (map[string]any, error) {
			if slug != venueSlug || query != "tea" {
				t.Fatalf("search = (%q, %q)", slug, query)
			}
			return map[string]any{
				"items": []any{
					map[string]any{
						"item_id":             itemID,
						"name":                "Iced Tea",
						"description":         "Cold brewed tea",
						"price":               map[string]any{"amount": 350},
						"category_id":         "drinks",
						"category_name":       "Drinks",
						"purchasable_balance": 5,
						"option_group_ids":    []any{"size"},
					},
				},
			}, nil
		},
	}
	cmd := newVenueMenuCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{inputURL, "--query", "tea", "--include-options", "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue menu query: %v\n%s", err, output.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	data := asMap(envelope["data"])
	items := asSlice(data["items"])
	if len(items) != 1 {
		t.Fatalf("items = %#v", data["items"])
	}
	item := asMap(items[0])
	if asString(item["description"]) != "Cold brewed tea" ||
		asString(item["category_id"]) != "drinks" ||
		asString(item["category_name"]) != "Drinks" ||
		asString(item["venue_id"]) != venueID ||
		asString(item["venue_slug"]) != venueSlug {
		t.Fatalf("shared item contract fields = %#v", item)
	}
	if asString(item["canonical_url"]) != "https://wolt.com/en/example/venue/example-market" {
		t.Fatalf("canonical_url = %#v", item["canonical_url"])
	}
	if asString(asMap(item["base_price"])["currency"]) != "EUR" ||
		asMap(item["price"]) == nil ||
		len(asSlice(item["option_group_ids"])) != 1 {
		t.Fatalf("price/options context = %#v", item)
	}
}

func TestVenueMenuQueryFiltersByExactCategorySlugFromRootMetadata(t *testing.T) {
	withIsolatedSlugCache(t)
	const (
		venueID      = "000000000000000000000061"
		venueSlug    = "example-market"
		itemID       = "000000000000000000000062"
		categoryID   = "cat-fresh"
		categorySlug = "fresh-produce"
	)
	assortmentCalls := 0
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{
					"id":       venueID,
					"slug":     venueSlug,
					"currency": "EUR",
				},
			}, nil
		},
		assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
			assortmentCalls++
			return map[string]any{
				"categories": []any{
					map[string]any{
						"id":       categoryID,
						"slug":     categorySlug,
						"name":     "Fresh produce",
						"item_ids": []any{itemID},
					},
				},
			}, nil
		},
		assortmentSearchFn: func(
			context.Context,
			string,
			string,
			string,
			woltgateway.AuthContext,
		) (map[string]any, error) {
			return map[string]any{
				"items": []any{
					map[string]any{
						"id":          itemID,
						"name":        "Apples",
						"price":       500,
						"category_id": categoryID,
					},
				},
			}, nil
		},
	}
	cmd := newVenueMenuCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{
		venueSlug,
		"--query", "apples",
		"--category", "FRESH-PRODUCE",
		"--format", "json",
	})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue menu query with category: %v\n%s", err, output.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	items := asSlice(asMap(envelope["data"])["items"])
	if len(items) != 1 || asString(asMap(items[0])["item_id"]) != itemID {
		t.Fatalf("items = %#v, want category slug match", items)
	}
	if _, exists := asMap(items[0])["category_slug"]; exists {
		t.Fatalf("category_slug leaked into the public item row: %#v", items[0])
	}
	if assortmentCalls != 1 {
		t.Fatalf("assortment metadata calls = %d, want 1", assortmentCalls)
	}
}

func TestVenueCatalogCommandsNeverEmitUnverifiedSlugAsVenueID(t *testing.T) {
	const (
		venueSlug = "unresolved-market"
		itemID    = "000000000000000000000401"
	)
	itemPageCalls := 0
	newDependencies := func() Dependencies {
		return Dependencies{
			Wolt: &testWoltAPI{
				venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
					return nil, errors.New("static unavailable")
				},
				venuePageDynamicFn: func(
					context.Context,
					string,
					woltgateway.VenuePageDynamicOptions,
				) (map[string]any, error) {
					return nil, errors.New("dynamic unavailable")
				},
				assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
					return map[string]any{
						"categories": []any{
							map[string]any{"id": "produce", "slug": "produce", "name": "Produce"},
						},
						"items": []any{
							map[string]any{"id": itemID, "name": "Example item", "price": 100},
						},
					}, nil
				},
				assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{
						"items": []any{
							map[string]any{
								"id":                  itemID,
								"name":                "Example item",
								"price":               100,
								"purchasable_balance": 5,
								"options": []any{
									map[string]any{
										"id": "size",
										"values": []any{
											map[string]any{"id": "large", "name": "Large", "price": 25},
										},
									},
								},
							},
						},
					}, nil
				},
				venueItemPageFn: func(context.Context, string, string) (map[string]any, error) {
					itemPageCalls++
					return nil, errors.New("must not call an id-only endpoint with an unverified slug")
				},
			},
			Profiles: &testProfiles{profile: domain.Profile{Name: "default", IsDefault: true}},
		}
	}

	t.Run("categories", func(t *testing.T) {
		withIsolatedSlugCache(t)
		cmd := newVenueCategoriesCommand(newDependencies())
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{venueSlug, "--format", "json"})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("venue categories: %v\n%s", err, output.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatalf("decode categories: %v", err)
		}
		data := asMap(envelope["data"])
		if asString(data["venue_id"]) != "" {
			t.Fatalf("unverified venue_id = %#v", data["venue_id"])
		}
	})

	t.Run("menu", func(t *testing.T) {
		withIsolatedSlugCache(t)
		cmd := newVenueMenuCommand(newDependencies())
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{venueSlug, "--format", "json"})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("venue menu: %v\n%s", err, output.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatalf("decode menu: %v", err)
		}
		data := asMap(envelope["data"])
		if asString(data["venue_id"]) != "" {
			t.Fatalf("unverified venue_id = %#v", data["venue_id"])
		}
		if asString(data["venue_slug"]) != venueSlug {
			t.Fatalf("venue_slug = %#v, want %q", data["venue_slug"], venueSlug)
		}
	})

	t.Run("item", func(t *testing.T) {
		withIsolatedSlugCache(t)
		itemPageCalls = 0
		cmd := newItemShowCommand(newDependencies())
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{venueSlug, itemID, "--format", "json"})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("venue item: %v\n%s", err, output.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatalf("decode item: %v", err)
		}
		data := asMap(envelope["data"])
		if itemPageCalls != 0 || asString(data["venue_id"]) != "" ||
			asString(data["venue_slug"]) != venueSlug {
			t.Fatalf("unverified item identity = %#v, item endpoint calls=%d", data, itemPageCalls)
		}
		groups := asSlice(data["option_groups"])
		if len(groups) != 1 || len(asSlice(asMap(groups[0])["values"])) != 1 {
			t.Fatalf("rich option metadata = %#v", groups)
		}
	})
}

func TestVenueMenuCategoryUsesOnlyCategoryItemsAndCanonicalMetadata(t *testing.T) {
	withIsolatedSlugCache(t)
	const (
		venueID       = "000000000000000000000061"
		requestedSlug = "requested-market"
		canonicalSlug = "canonical-market"
		selectedID    = "000000000000000000000062"
		unrelatedID   = "000000000000000000000063"
	)
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{
				"id":       venueID,
				"slug":     canonicalSlug,
				"currency": "EUR",
			}}, nil
		},
		venuePageDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{
				"id":   venueID,
				"slug": canonicalSlug,
			}}, nil
		},
		assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"currency": "EUR",
				"items": []any{
					map[string]any{"id": unrelatedID, "name": "Unrelated root item", "price": 800},
				},
			}, nil
		},
		assortmentCategoryFn: func(
			_ context.Context,
			slug string,
			category string,
			_ string,
			_ woltgateway.AuthContext,
		) (map[string]any, error) {
			if slug != canonicalSlug || category != "selected" {
				t.Fatalf("category request = slug %q category %q", slug, category)
			}
			return map[string]any{
				"category": map[string]any{
					"id":   "category-selected",
					"slug": "selected",
					"name": "Selected",
				},
				"items": []any{
					map[string]any{"id": selectedID, "name": "Selected item", "price": 500},
				},
			}, nil
		},
	}
	cmd := newVenueMenuCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{requestedSlug, "--category", "selected", "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue menu category: %v\n%s", err, output.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	data := asMap(envelope["data"])
	items := asSlice(data["items"])
	if len(items) != 1 || asString(asMap(items[0])["item_id"]) != selectedID {
		t.Fatalf("selected category leaked root items: %#v", items)
	}
	if asString(data["venue_id"]) != venueID ||
		asString(data["venue_slug"]) != canonicalSlug ||
		asString(data["currency"]) != "EUR" {
		t.Fatalf("canonical venue metadata = %#v", data)
	}
}

func TestVenueMenuFullCatalogPrefersHydratedCategoryItem(t *testing.T) {
	withIsolatedSlugCache(t)
	const (
		venueID   = "000000000000000000000081"
		venueSlug = "partial-example-market"
		itemID    = "000000000000000000000082"
		cheapID   = "000000000000000000000083"
	)
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{
				"id":       venueID,
				"slug":     venueSlug,
				"currency": "EUR",
			}}, nil
		},
		assortmentBySlugFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"loading_strategy": "partial",
				"currency":         "EUR",
				"categories": []any{
					map[string]any{
						"id":       "selected-category",
						"slug":     "selected",
						"name":     "Selected",
						"item_ids": []any{itemID},
					},
					map[string]any{
						"id":       "later-category",
						"slug":     "later",
						"name":     "Later",
						"item_ids": []any{cheapID},
					},
				},
				"items": []any{
					map[string]any{"id": itemID, "name": "Stale root preview", "price": 400},
				},
			}, nil
		},
		assortmentCategoryFn: func(
			_ context.Context,
			_ string,
			category string,
			_ string,
			_ woltgateway.AuthContext,
		) (map[string]any, error) {
			if category == "later" {
				return map[string]any{
					"category": map[string]any{
						"id":       "later-category",
						"slug":     "later",
						"name":     "Later",
						"item_ids": []any{cheapID},
					},
					"items": []any{
						map[string]any{"id": cheapID, "name": "Cheapest later item", "price": 100},
					},
				}, nil
			}
			return map[string]any{
				"category": map[string]any{
					"id":       "selected-category",
					"slug":     "selected",
					"name":     "Selected",
					"item_ids": []any{itemID},
				},
				"items": []any{
					map[string]any{"id": itemID, "name": "Current category item", "price": 500},
				},
			}, nil
		},
	}
	cmd := newVenueMenuCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{venueSlug, "--full-catalog", "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue menu full catalog: %v\n%s", err, output.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	items := asSlice(asMap(envelope["data"])["items"])
	if len(items) != 2 || asString(asMap(items[0])["name"]) != "Current category item" {
		t.Fatalf("full catalog selected stale root data: %#v", items)
	}

	limited := newVenueMenuCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
		}},
	})
	limitedOutput := &bytes.Buffer{}
	limited.SetOut(limitedOutput)
	limited.SetErr(limitedOutput)
	limited.SetArgs([]string{
		venueSlug,
		"--full-catalog",
		"--sort", "price",
		"--limit", "1",
		"--format", "json",
	})
	if err := limited.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue menu limited full catalog: %v\n%s", err, limitedOutput.String())
	}
	envelope = map[string]any{}
	if err := json.Unmarshal(limitedOutput.Bytes(), &envelope); err != nil {
		t.Fatalf("decode limited output: %v\n%s", err, limitedOutput.String())
	}
	items = asSlice(asMap(envelope["data"])["items"])
	if len(items) != 1 || asString(asMap(items[0])["item_id"]) != cheapID {
		t.Fatalf("limit was applied before cross-category sorting: %#v", items)
	}
}
