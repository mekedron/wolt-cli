package catalogload

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func optionGroupID(raw any) string {
	group := payloadutil.Map(raw)
	return strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		group["id"],
		group["group_id"],
		group["option_id"],
	)))
}

func TestCategoriesPreservesPartialRootMetadata(t *testing.T) {
	payload := partialCatalogFixture()
	if !IsPartial(payload) {
		t.Fatal("expected partial loading strategy")
	}
	rows := Categories(payload)
	if len(rows) != 2 {
		t.Fatalf("categories = %#v", rows)
	}
	if rows[0].Slug != "fresh" || rows[0].Leaf {
		t.Fatalf("root category = %#v", rows[0])
	}
	if rows[1].Slug != "fish" || !rows[1].Leaf || rows[1].ParentSlug != "fresh" || rows[1].ItemRefsCount != 2 {
		t.Fatalf("leaf category = %#v", rows[1])
	}
	slugs := LoadableCategorySlugs(payload)
	if len(slugs) != 1 || slugs[0] != "fish" {
		t.Fatalf("loadable category slugs = %v", slugs)
	}
}

func TestRootIsPartialDetectsImplicitAndExplicitIncompleteRoots(t *testing.T) {
	tests := []struct {
		name            string
		payload         map[string]any
		materializedIDs []string
		want            bool
	}{
		{
			name: "unlabelled missing reference",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":       "cat-fresh",
					"slug":     "fresh-produce",
					"item_ids": []any{"item-1", "item-2"},
				}},
				"items": []any{map[string]any{"id": "item-1"}},
			},
			materializedIDs: []string{"item-1"},
			want:            true,
		},
		{
			name: "explicit partial with materialized references",
			payload: map[string]any{
				"loading_strategy": "partial",
				"items":            []any{map[string]any{"id": "item-1"}},
			},
			materializedIDs: []string{"item-1"},
			want:            true,
		},
		{
			name: "categories without materialized items",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":   "cat-empty",
					"slug": "empty",
				}},
			},
			want: true,
		},
		{
			name: "complete root",
			payload: map[string]any{
				"categories": []any{map[string]any{
					"id":       "cat-fresh",
					"slug":     "fresh-produce",
					"item_ids": []any{"item-1"},
				}},
				"items": []any{map[string]any{"id": "item-1"}},
			},
			materializedIDs: []string{"item-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RootIsPartial(test.payload, test.materializedIDs); got != test.want {
				t.Fatalf("RootIsPartial() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLoadCategoryHydratesBatchesAndFallsBackToAnonymous(t *testing.T) {
	const totalItems = 81
	itemIDs := sequentialItemRefs(totalItems)
	categoryAuthCalls := 0
	itemBatchSizes := []int{}
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, auth woltgateway.AuthContext) (map[string]any, error) {
			categoryAuthCalls++
			if auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     http.MethodGet,
					StatusCode: http.StatusUnauthorized,
				}
			}
			return map[string]any{"category": map[string]any{
				"slug":     "fish",
				"item_ids": itemIDs,
			}}, nil
		},
		itemsFn: func(_ context.Context, _ string, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemBatchSizes = append(itemBatchSizes, len(ids))
			items := make([]any, 0, len(ids))
			for _, id := range ids {
				items = append(items, map[string]any{"id": id, "name": id, "price": 100})
			}
			return map[string]any{"items": items}, nil
		},
	}
	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"fish",
		"en",
		woltgateway.AuthContext{WToken: "saved"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if categoryAuthCalls != 2 {
		t.Fatalf("category calls = %d, want authenticated + anonymous", categoryAuthCalls)
	}
	if len(itemBatchSizes) != 2 || itemBatchSizes[0] != 80 || itemBatchSizes[1] != 1 {
		t.Fatalf("item batch sizes = %v", itemBatchSizes)
	}
	if !got.Complete || got.ItemCount != totalItems || len(got.Warnings) != 0 {
		t.Fatalf("load = %#v", got)
	}
}

func TestLoadCategoryDetectsMissingReferencedItemWithEqualItemCount(t *testing.T) {
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"category": map[string]any{
				"slug":     "pantry",
				"item_ids": []any{"item-a", "item-b"},
			}}, nil
		},
		itemsFn: func(_ context.Context, _ string, _ []string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"items": []any{
				map[string]any{"id": "item-a", "name": "Item A"},
				map[string]any{"id": "item-unrelated", "name": "Unrelated"},
			}}, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"pantry",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || got.ItemCount != 2 || len(got.Warnings) != 1 {
		t.Fatalf("load = %#v", got)
	}
	if got.Warnings[0] != `category "pantry" was only partially hydrated: loaded 1 of 2 referenced items` {
		t.Fatalf("warning = %q", got.Warnings[0])
	}
}

func TestLoadCategoryOrdersItemsByReference(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		hydrated []any
		wantIDs  []string
	}{
		{
			name: "mixed existing and hydrated",
			payload: map[string]any{
				"category": map[string]any{
					"slug":     "pantry",
					"item_ids": []any{"item-a", "item-b", "item-c"},
				},
				"items": []any{
					map[string]any{"id": "item-c", "name": "Item C"},
					map[string]any{"id": "item-extra", "name": "Extra"},
				},
			},
			hydrated: []any{
				map[string]any{"id": "item-a", "name": "Item A"},
				map[string]any{"id": "item-b", "name": "Item B"},
			},
			wantIDs: []string{"item-a", "item-b", "item-c", "item-extra"},
		},
		{
			name: "already materialized",
			payload: map[string]any{
				"category": map[string]any{
					"slug":     "pantry",
					"item_ids": []any{"item-a", "item-b", "item-c"},
				},
				"items": []any{
					map[string]any{"id": "item-c", "name": "Item C"},
					map[string]any{"id": "item-a", "name": "Item A"},
					map[string]any{"id": "item-b", "name": "Item B"},
				},
			},
			wantIDs: []string{"item-a", "item-b", "item-c"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &catalogStub{
				categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
					return test.payload, nil
				},
				itemsFn: func(_ context.Context, _ string, _ []string, _ woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{"items": test.hydrated}, nil
				},
			}

			got, err := LoadCategory(
				context.Background(),
				api,
				"store",
				"pantry",
				"en",
				woltgateway.AuthContext{},
			)
			if err != nil {
				t.Fatal(err)
			}
			items, _ := got.Payload["items"].([]any)
			gotIDs := make([]string, 0, len(items))
			for _, item := range items {
				gotIDs = append(gotIDs, payloadItemID(item))
			}
			if !slices.Equal(gotIDs, test.wantIDs) {
				t.Fatalf("item ids = %v, want %v", gotIDs, test.wantIDs)
			}
		})
	}
}

func TestLoadCategoryMergesOptionGroupsAcrossBatches(t *testing.T) {
	const totalItems = itemBatchSize + 1
	itemIDs := sequentialItemRefs(totalItems)
	itemCalls := 0
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"category": map[string]any{
				"slug":     "meals",
				"item_ids": itemIDs,
			}}, nil
		},
		itemsFn: func(_ context.Context, _ string, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemCalls++
			items := make([]any, 0, len(ids))
			for _, id := range ids {
				items = append(items, map[string]any{"id": id, "name": id})
			}
			if itemCalls == 1 {
				return map[string]any{
					"items": items,
					"options": []any{
						map[string]any{"id": "group-first", "name": "First"},
					},
				}, nil
			}
			return map[string]any{
				"items": items,
				"option_groups": []any{
					map[string]any{"id": "group-first", "name": "Duplicate"},
					map[string]any{"group_id": "group-second", "name": "Second"},
				},
			}, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"meals",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Warnings) != 0 {
		t.Fatalf("load = %#v", got)
	}
	options, _ := got.Payload["option_groups"].([]any)
	if len(options) != 2 {
		t.Fatalf("option groups = %#v", options)
	}
	if optionGroupID(options[0]) != "group-first" || optionGroupID(options[1]) != "group-second" {
		t.Fatalf("option group order = %#v", options)
	}
}

func TestLoadCategoryMergesExistingAndHydratedOptionGroups(t *testing.T) {
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"category": map[string]any{
					"slug":     "meals",
					"item_ids": []any{"item-1"},
				},
				"options": []any{
					map[string]any{"id": "existing-option"},
				},
				"option_groups": []any{
					map[string]any{"group_id": "existing-group", "name": "Existing group"},
				},
			}, nil
		},
		itemsFn: func(_ context.Context, _ string, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"items": []any{
					map[string]any{"id": ids[0], "name": ids[0]},
				},
				"option_groups": []any{
					map[string]any{
						"id":   "existing-option",
						"name": "Hydrated option",
						"values": []any{
							map[string]any{"id": "large", "price": 125},
						},
					},
					map[string]any{"id": "hydrated-group", "name": "Hydrated group"},
				},
			}, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"meals",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"options", "option_groups"} {
		options, _ := got.Payload[key].([]any)
		if len(options) != 3 {
			t.Fatalf("%s = %#v, want existing and hydrated option groups", key, options)
		}
		gotIDs := []string{
			optionGroupID(options[0]),
			optionGroupID(options[1]),
			optionGroupID(options[2]),
		}
		wantIDs := []string{"existing-option", "existing-group", "hydrated-group"}
		for index := range wantIDs {
			if gotIDs[index] != wantIDs[index] {
				t.Fatalf("%s ids = %v, want %v", key, gotIDs, wantIDs)
			}
		}
		hydrated := options[0].(map[string]any)
		values, _ := hydrated["values"].([]any)
		if hydrated["name"] != "Hydrated option" || len(values) != 1 {
			t.Fatalf("%s did not replace ID-only option metadata: %#v", key, hydrated)
		}
	}
}

func TestLoadCategoryPropagatesContextErrorFromLaterBatch(t *testing.T) {
	for _, contextErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(contextErr.Error(), func(t *testing.T) {
			const totalItems = itemBatchSize + 1
			itemIDs := sequentialItemRefs(totalItems)
			itemCalls := 0
			api := &catalogStub{
				categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
					return map[string]any{"category": map[string]any{
						"slug":     "pantry",
						"item_ids": itemIDs,
					}}, nil
				},
				itemsFn: func(_ context.Context, _ string, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
					itemCalls++
					if itemCalls == 2 {
						return nil, contextErr
					}
					items := make([]any, 0, len(ids))
					for _, id := range ids {
						items = append(items, map[string]any{"id": id})
					}
					return map[string]any{"items": items}, nil
				},
			}

			_, err := LoadCategory(
				context.Background(),
				api,
				"store",
				"pantry",
				"en",
				woltgateway.AuthContext{},
			)
			if !errors.Is(err, contextErr) {
				t.Fatalf("error = %v, want %v", err, contextErr)
			}
			if itemCalls != 2 {
				t.Fatalf("item calls = %d, want 2", itemCalls)
			}
		})
	}
}

func TestLoadCategoryDoesNotRetryRateLimitAnonymously(t *testing.T) {
	calls := 0
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			calls++
			return nil, &woltgateway.UpstreamRequestError{
				Method:     http.MethodGet,
				StatusCode: http.StatusTooManyRequests,
			}
		},
	}
	_, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"fish",
		"en",
		woltgateway.AuthContext{WToken: "saved"},
	)
	if err == nil {
		t.Fatal("rate limit was swallowed")
	}
	if calls != 1 {
		t.Fatalf("category calls = %d, want one credentialed call", calls)
	}
}

func TestLoadCategoryReturnsHydrationErrorWhenOnlyUnreferencedItemsExist(t *testing.T) {
	itemCalls := 0
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"category": map[string]any{
					"slug":     "pantry",
					"item_ids": []any{"item-1"},
				},
				"items": []any{
					map[string]any{"id": "unreferenced-extra"},
				},
			}, nil
		},
		itemsFn: func(_ context.Context, _ string, _ []string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemCalls++
			return nil, &woltgateway.UpstreamRequestError{
				Method:     http.MethodPost,
				StatusCode: http.StatusServiceUnavailable,
			}
		},
	}

	_, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"pantry",
		"en",
		woltgateway.AuthContext{},
	)
	var upstream *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("error = %v, want upstream 503", err)
	}
	if itemCalls != 1 {
		t.Fatalf("item calls = %d, want 1", itemCalls)
	}
}

func TestLoadCategoryKeepsPartialSuccessWithWarning(t *testing.T) {
	const totalItems = itemBatchSize + 1
	itemIDs := sequentialItemRefs(totalItems)
	itemCalls := 0
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"category": map[string]any{
				"slug":     "pantry",
				"item_ids": itemIDs,
			}}, nil
		},
		itemsFn: func(_ context.Context, _ string, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemCalls++
			if itemCalls == 2 {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     http.MethodPost,
					StatusCode: http.StatusServiceUnavailable,
				}
			}
			items := make([]any, 0, len(ids))
			for _, id := range ids {
				items = append(items, map[string]any{"id": id, "name": id})
			}
			return map[string]any{"items": items}, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"pantry",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || got.ItemCount != itemBatchSize || len(got.Warnings) != 1 {
		t.Fatalf("load = %#v", got)
	}
}

func TestLoadCategoryDoesNotTreatIDOnlyItemsAsMaterialized(t *testing.T) {
	itemCalls := 0
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"category": map[string]any{
					"slug":     "pantry",
					"item_ids": []any{"item-a"},
				},
				"items": []any{map[string]any{"id": "item-a"}},
			}, nil
		},
		itemsFn: func(_ context.Context, _ string, _ []string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemCalls++
			return map[string]any{
				"items": []any{map[string]any{"id": "ITEM-A"}},
			}, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"pantry",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if itemCalls != 1 || got.Complete || len(got.Warnings) != 1 {
		t.Fatalf("sparse hydration = %#v, calls=%d", got, itemCalls)
	}
}

func TestLoadCategoryMatchesMaterializedIDsCaseInsensitively(t *testing.T) {
	api := &catalogStub{
		categoryFn: func(_ context.Context, _, _ string, _ string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"category": map[string]any{
					"slug":     "pantry",
					"item_ids": []any{"abcdefabcdefabcdefabcdef"},
				},
				"items": []any{
					map[string]any{
						"id":   "ABCDEFABCDEFABCDEFABCDEF",
						"name": "Materialized",
					},
				},
			}, nil
		},
		itemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			t.Fatal("case-only ID difference triggered unnecessary hydration")
			return nil, nil
		},
	}

	got, err := LoadCategory(
		context.Background(),
		api,
		"store",
		"pantry",
		"en",
		woltgateway.AuthContext{},
	)
	if err != nil || !got.Complete || got.ItemCount != 1 {
		t.Fatalf("case-insensitive load = %#v, err=%v", got, err)
	}
}

func TestDedupeItemsMergesLaterRicherDefinition(t *testing.T) {
	items := dedupeItems([]any{
		map[string]any{
			"id":    "abcdefabcdefabcdefabcdef",
			"name":  "Stale",
			"price": 100,
			"options": []any{
				map[string]any{"id": "size"},
			},
		},
		map[string]any{
			"id":    "ABCDEFABCDEFABCDEFABCDEF",
			"name":  "Current",
			"price": 250,
			"option_groups": []any{
				map[string]any{
					"id":   "SIZE",
					"name": "Size",
					"values": []any{
						map[string]any{"id": "large", "name": "Large", "price": 50},
					},
				},
			},
		},
	})
	if len(items) != 1 {
		t.Fatalf("deduped items = %#v", items)
	}
	item := payloadutil.Map(items[0])
	if item["name"] != "Current" || payloadutil.Int(item["price"]) != 250 {
		t.Fatalf("later fields were not authoritative: %#v", item)
	}
	specs := payloadutil.ExtractOptionSpecs(item)
	var spec payloadutil.OptionGroupSpec
	for _, candidate := range specs {
		spec = candidate
	}
	if len(specs) != 1 || spec.Name != "Size" || spec.Values["large"].Name != "Large" {
		t.Fatalf("rich option metadata was lost: %#v", item)
	}
}

func sequentialItemRefs(total int) []any {
	itemIDs := make([]any, 0, total)
	for idx := range total {
		itemIDs = append(itemIDs, fmt.Sprintf("item-%03d", idx))
	}
	return itemIDs
}

func partialCatalogFixture() map[string]any {
	return map[string]any{
		"loading_strategy": "partial",
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
	}
}

type catalogStub struct {
	woltgateway.API
	categoryFn func(context.Context, string, string, string, woltgateway.AuthContext) (map[string]any, error)
	itemsFn    func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error)
}

func (s *catalogStub) AssortmentCategoryByVenueSlug(ctx context.Context, slug, category, language string, auth woltgateway.AuthContext) (map[string]any, error) {
	return s.categoryFn(ctx, slug, category, language, auth)
}
func (s *catalogStub) AssortmentItemsByVenueSlug(ctx context.Context, slug string, ids []string, auth woltgateway.AuthContext) (map[string]any, error) {
	return s.itemsFn(ctx, slug, ids, auth)
}
