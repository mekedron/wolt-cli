package observability

import "testing"

func TestExtractMenuItemsIndexesNestedAndSingularCategories(t *testing.T) {
	const itemID = "item-1"
	tests := map[string]map[string]any{
		"nested categories": {
			"categories": []any{
				map[string]any{
					"id":   "parent",
					"name": "Parent",
					"subcategories": []any{
						map[string]any{
							"id":       "leaf",
							"name":     "Leaf",
							"item_ids": []any{itemID},
						},
					},
				},
			},
		},
		"singular category": {
			"category": map[string]any{
				"id":       "leaf",
				"name":     "Leaf",
				"item_ids": []any{itemID},
			},
		},
	}

	for name, categoryPayload := range tests {
		t.Run(name, func(t *testing.T) {
			categoryPayload["items"] = []any{
				map[string]any{"id": itemID, "name": "Example item", "price": 500},
			}
			items := ExtractMenuItems(categoryPayload, "venue-1", "example-venue")
			if len(items) != 1 {
				t.Fatalf("items = %d, want 1", len(items))
			}
			if items[0]["category_id"] != "leaf" ||
				items[0]["category_name"] != "Leaf" ||
				items[0]["category"] != "Leaf" {
				t.Fatalf("category context = %#v", items[0])
			}
		})
	}
}

func TestExtractMenuItemsUsesStableCategoryContainerPrecedence(t *testing.T) {
	const itemID = "item-1"
	payload := map[string]any{
		"category": map[string]any{
			"id":       "selected",
			"name":     "Selected category",
			"item_ids": []any{itemID},
		},
		"categories": []any{
			map[string]any{
				"id":       "broader",
				"name":     "Broader category",
				"item_ids": []any{itemID},
			},
		},
		"subcategories": []any{
			map[string]any{
				"id":       "other",
				"name":     "Other category",
				"item_ids": []any{itemID},
			},
		},
		"items": []any{
			map[string]any{"id": itemID, "name": "Example item", "price": 500},
		},
	}

	for attempt := 0; attempt < 100; attempt++ {
		items := ExtractMenuItems(payload, "venue-1", "example-venue")
		if len(items) != 1 {
			t.Fatalf("attempt %d: items = %d, want 1", attempt, len(items))
		}
		if items[0]["category_id"] != "selected" ||
			items[0]["category_name"] != "Selected category" {
			t.Fatalf("attempt %d: category context = %#v", attempt, items[0])
		}
	}
}

func TestItemBuildersEnrichCategoryNamesFromOrderedMetadata(t *testing.T) {
	selected := map[string]any{
		"categories": []any{
			map[string]any{"id": "preferred", "name": "Selected category"},
		},
		"items": []any{
			map[string]any{"id": "item-from-metadata", "name": "First item", "price": 500, "category_id": "metadata-only"},
			map[string]any{"id": "item-from-selection", "name": "Second item", "price": 600, "category_id": "preferred"},
			map[string]any{
				"id":            "item-explicit",
				"name":          "Third item",
				"price":         700,
				"category_id":   "explicit",
				"category_name": "Item category",
				"category":      "Item category",
			},
		},
	}
	metadata := map[string]any{
		"categories": []any{
			map[string]any{"id": "metadata-only", "name": "Metadata category"},
			map[string]any{"id": "preferred", "name": "Later category"},
			map[string]any{"id": "explicit", "name": "Metadata must not replace the item"},
		},
	}
	context := ItemVenueContext{MetadataPayloads: []map[string]any{metadata}}

	builders := []struct {
		name  string
		build func() (map[string]any, []string)
	}{
		{
			name: "venue menu",
			build: func() (map[string]any, []string) {
				return BuildVenueMenu("", []map[string]any{selected}, "", false, nil, context)
			},
		},
		{
			name: "item search",
			build: func() (map[string]any, []string) {
				return BuildItemSearchResult(
					"",
					[]map[string]any{selected},
					"",
					nil,
					0,
					nil,
					context,
				)
			},
		},
	}

	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			data, warnings := builder.build()
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			items := data["items"].([]map[string]any)
			assertItemCategory(t, items, "item-from-metadata", "metadata-only", "Metadata category")
			assertItemCategory(t, items, "item-from-selection", "preferred", "Selected category")
			assertItemCategory(t, items, "item-explicit", "explicit", "Item category")
		})
	}
}

func assertItemCategory(t *testing.T, items []map[string]any, itemID string, categoryID string, categoryName string) {
	t.Helper()
	for _, item := range items {
		if item["item_id"] != itemID {
			continue
		}
		if item["category_id"] != categoryID ||
			item["category_name"] != categoryName ||
			item["category"] != categoryName {
			t.Fatalf("item %q category context = %#v", itemID, item)
		}
		return
	}
	t.Fatalf("item %q not found in %#v", itemID, items)
}

func TestFilterItemsByCategoryMatchesNormalizedNameAndID(t *testing.T) {
	items := []map[string]any{
		{
			"item_id":       "item-1",
			"category":      "Fresh produce",
			"category_id":   "cat-fresh",
			"category_slug": "fresh-produce",
			"category_name": "Fresh produce",
		},
	}
	for _, filter := range []string{"fresh produce", "cat-fresh", "fresh-produce"} {
		if got := filterItemsByCategory(items, filter); len(got) != 1 {
			t.Fatalf("filter %q returned %#v, want the category item", filter, got)
		}
	}
	if got := filterItemsByCategory(items, "fresh"); len(got) != 0 {
		t.Fatalf("prefix filter returned %#v, want exact category matching", got)
	}
}

func TestItemBuildersFilterByCategorySlugFromMetadata(t *testing.T) {
	selected := map[string]any{
		"items": []any{
			map[string]any{
				"id":          "item-1",
				"name":        "Apples",
				"price":       500,
				"category_id": "cat-fresh",
			},
		},
	}
	metadata := map[string]any{
		"categories": []any{
			map[string]any{
				"id":   "cat-fresh",
				"slug": "fresh-produce",
				"name": "Fresh produce",
			},
		},
	}
	context := ItemVenueContext{MetadataPayloads: []map[string]any{metadata}}

	builders := []struct {
		name  string
		build func() (map[string]any, []string)
	}{
		{
			name: "venue menu",
			build: func() (map[string]any, []string) {
				return BuildVenueMenu("", []map[string]any{selected}, "fresh-produce", false, nil, context)
			},
		},
		{
			name: "item search",
			build: func() (map[string]any, []string) {
				return BuildItemSearchResult(
					"",
					[]map[string]any{selected},
					"fresh-produce",
					nil,
					0,
					nil,
					context,
				)
			},
		},
	}

	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			data, warnings := builder.build()
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			items := data["items"].([]map[string]any)
			if len(items) != 1 || items[0]["item_id"] != "item-1" {
				t.Fatalf("items = %#v, want item-1", items)
			}
			if _, exists := items[0]["category_slug"]; exists {
				t.Fatalf("category_slug leaked into the public item row: %#v", items[0])
			}
		})
	}
}
