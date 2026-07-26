// Package catalogload contains catalog-loading primitives shared by clients
// that need to handle Wolt's partial grocery assortment backend.
package catalogload

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogitem"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

const itemBatchSize = 80

// Category describes one category advertised by an assortment root payload.
type Category struct {
	ID            string `json:"id,omitempty"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ParentSlug    string `json:"parent_slug,omitempty"`
	Level         int    `json:"level"`
	Leaf          bool   `json:"leaf"`
	ItemRefsCount int    `json:"item_refs_count"`
}

// CategoryLoad is a category response after resolving referenced item ids.
type CategoryLoad struct {
	Payload      map[string]any
	ItemRefCount int
	ItemCount    int
	Complete     bool
	Warnings     []string
}

// IsPartial reports Wolt's explicit partial-assortment loading strategy.
func IsPartial(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(payloadutil.String(payload["loading_strategy"])), "partial")
}

// RootIsPartial reports whether an assortment root is incomplete, including
// payloads where Wolt omits loading_strategy but advertises category item IDs
// that are not materialized in the root response.
func RootIsPartial(payload map[string]any, materializedItemIDs []string) bool {
	if IsPartial(payload) {
		return true
	}

	materialized := make(map[string]struct{}, len(materializedItemIDs))
	for _, itemID := range materializedItemIDs {
		if itemID = strings.TrimSpace(itemID); itemID != "" {
			materialized[strings.ToLower(itemID)] = struct{}{}
		}
	}
	for _, itemID := range ItemIDs(payload) {
		if _, exists := materialized[strings.ToLower(itemID)]; !exists {
			return true
		}
	}
	return len(materialized) == 0 && len(Categories(payload)) > 0
}

// Categories flattens nested root category metadata without requiring items to
// be materialized in the root assortment payload.
func Categories(payload map[string]any) []Category {
	rows := []Category{}
	seen := map[string]struct{}{}
	walkCategories(payload, func(category map[string]any, parentSlug string, level int) {
		children := payloadutil.Slice(category["subcategories"])
		slug := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(category["slug"], category["id"])))
		if slug != "" {
			if _, exists := seen[slug]; !exists {
				seen[slug] = struct{}{}
				rows = append(rows, Category{
					ID:            strings.TrimSpace(payloadutil.String(category["id"])),
					Slug:          slug,
					Name:          firstNonEmpty(category["name"], category["title"], slug),
					ParentSlug:    parentSlug,
					Level:         level,
					Leaf:          len(children) == 0,
					ItemRefsCount: len(payloadutil.Slice(category["item_ids"])),
				})
			}
		}
	})
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Level != rows[j].Level {
			return rows[i].Level < rows[j].Level
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	return rows
}

// LoadableCategorySlugs returns categories that can contribute items while
// preserving their advertised traversal order.
func LoadableCategorySlugs(payload map[string]any) []string {
	slugs := []string{}
	seen := map[string]struct{}{}
	walkCategories(payload, func(category map[string]any, _ string, _ int) {
		children := payloadutil.Slice(category["subcategories"])
		slug := strings.TrimSpace(payloadutil.String(category["slug"]))
		if slug == "" || (len(children) > 0 && len(payloadutil.Slice(category["item_ids"])) == 0) {
			return
		}
		if _, exists := seen[slug]; exists {
			return
		}
		seen[slug] = struct{}{}
		slugs = append(slugs, slug)
	})
	return slugs
}

// ItemIDs returns referenced and materialized item ids in payload order.
func ItemIDs(payload map[string]any) []string {
	itemIDs := categoryItemIDs(payload)
	for _, raw := range payloadutil.Slice(payload["items"]) {
		itemID := payloadItemID(raw)
		if itemID != "" {
			itemIDs = append(itemIDs, itemID)
		}
	}
	return dedupeStrings(itemIDs)
}

// LoadCategory requests one category and hydrates item_ids through the exact
// item endpoint in bounded batches. Public catalog endpoints are retried
// anonymously when a saved session is rejected.
func LoadCategory(
	ctx context.Context,
	api woltgateway.API,
	venueSlug string,
	categorySlug string,
	language string,
	auth woltgateway.AuthContext,
) (CategoryLoad, error) {
	return LoadCategoryWithRetry(ctx, api, venueSlug, categorySlug, language, auth, RetryPolicy{})
}

// LoadCategoryWithRetry is LoadCategory with an explicit transient retry
// policy for callers that load many categories.
func LoadCategoryWithRetry(
	ctx context.Context,
	api woltgateway.API,
	venueSlug string,
	categorySlug string,
	language string,
	auth woltgateway.AuthContext,
	retry RetryPolicy,
) (CategoryLoad, error) {
	if api == nil {
		return CategoryLoad{}, fmt.Errorf("catalog API is unavailable")
	}
	categorySlug = strings.TrimSpace(categorySlug)
	if categorySlug == "" {
		return CategoryLoad{}, fmt.Errorf("category slug is required")
	}
	payload, err := RequestPayload(
		ctx,
		auth,
		retry,
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return api.AssortmentCategoryByVenueSlug(
				ctx,
				venueSlug,
				categorySlug,
				language,
				requestAuth,
			)
		},
	)
	if err != nil {
		return CategoryLoad{}, err
	}
	return hydrateCategoryPayload(ctx, api, venueSlug, categorySlug, auth, retry, payload)
}

type itemHydration struct {
	items             []any
	options           []any
	failedBatches     int
	successfulBatches int
	err               error
}

func hydrateCategoryPayload(
	ctx context.Context,
	api woltgateway.API,
	venueSlug string,
	categorySlug string,
	auth woltgateway.AuthContext,
	retry RetryPolicy,
	payload map[string]any,
) (CategoryLoad, error) {
	itemIDs := categoryItemIDs(payload)
	items := dedupeItems(payloadutil.Slice(payload["items"]))
	if len(itemIDs) == 0 {
		return CategoryLoad{
			Payload:      payload,
			ItemRefCount: len(itemIDs),
			ItemCount:    len(items),
			Complete:     true,
		}, nil
	}

	missingIDs := missingItemIDs(itemIDs, items)
	if len(missingIDs) == 0 {
		merged := cloneMap(payload)
		merged["items"] = orderItemsByReferences(items, itemIDs)
		return CategoryLoad{
			Payload:      merged,
			ItemRefCount: len(itemIDs),
			ItemCount:    len(items),
			Complete:     true,
		}, nil
	}
	referencedItemsPresent := len(itemIDs) - len(missingIDs)
	hydration := loadMissingItems(ctx, api, venueSlug, missingIDs, auth, retry)
	if errors.Is(hydration.err, context.Canceled) ||
		errors.Is(hydration.err, context.DeadlineExceeded) {
		return CategoryLoad{}, hydration.err
	}
	items = orderItemsByReferences(
		dedupeItems(append(items, hydration.items...)),
		itemIDs,
	)
	if hydration.failedBatches > 0 &&
		hydration.successfulBatches == 0 &&
		referencedItemsPresent == 0 {
		return CategoryLoad{}, hydration.err
	}
	merged := cloneMap(payload)
	if len(items) > 0 {
		merged["items"] = items
	}
	if len(hydration.options) > 0 {
		options := payloadutil.MergeOptionGroups(
			payloadutil.MergeOptionGroups(
				payloadutil.Slice(payload["options"]),
				payloadutil.Slice(payload["option_groups"]),
			),
			hydration.options,
		)
		merged["options"] = options
		merged["option_groups"] = options
	}
	remainingMissingIDs := missingItemIDs(itemIDs, items)
	loadedReferencedItems := len(itemIDs) - len(remainingMissingIDs)
	warnings := []string{}
	if hydration.failedBatches > 0 || len(remainingMissingIDs) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"category %q was only partially hydrated: loaded %d of %d referenced items",
			categorySlug,
			loadedReferencedItems,
			len(itemIDs),
		))
	}
	return CategoryLoad{
		Payload:      merged,
		ItemRefCount: len(itemIDs),
		ItemCount:    len(items),
		Complete:     hydration.failedBatches == 0 && len(remainingMissingIDs) == 0,
		Warnings:     warnings,
	}, nil
}

func loadMissingItems(
	ctx context.Context,
	api woltgateway.API,
	venueSlug string,
	missingIDs []string,
	auth woltgateway.AuthContext,
	retry RetryPolicy,
) itemHydration {
	result := itemHydration{}
	for start := 0; start < len(missingIDs); start += itemBatchSize {
		end := start + itemBatchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		batch := missingIDs[start:end]
		batchPayload, batchErr := RequestPayload(
			ctx,
			auth,
			retry,
			func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
				return api.AssortmentItemsByVenueSlug(ctx, venueSlug, batch, requestAuth)
			},
		)
		if batchErr != nil {
			result.failedBatches++
			result.err = batchErr
			if errors.Is(batchErr, context.Canceled) ||
				errors.Is(batchErr, context.DeadlineExceeded) {
				return result
			}
			continue
		}
		result.successfulBatches++
		result.items = append(result.items, payloadutil.Slice(batchPayload["items"])...)
		batchOptions := payloadutil.MergeOptionGroups(
			payloadutil.Slice(batchPayload["options"]),
			payloadutil.Slice(batchPayload["option_groups"]),
		)
		result.options = payloadutil.MergeOptionGroups(
			result.options,
			batchOptions,
		)
	}
	return result
}

func missingItemIDs(itemIDs []string, items []any) []string {
	existing := make(map[string]struct{}, len(items))
	for _, raw := range items {
		item := payloadutil.Map(raw)
		if !materializedItem(item) {
			continue
		}
		existing[strings.ToLower(payloadItemID(item))] = struct{}{}
	}
	missing := make([]string, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		if _, exists := existing[strings.ToLower(itemID)]; !exists {
			missing = append(missing, itemID)
		}
	}
	return missing
}

func categoryItemIDs(payload map[string]any) []string {
	itemIDs := []string{}
	var collect func(any)
	collect = func(raw any) {
		category := payloadutil.Map(raw)
		if category == nil {
			return
		}
		for _, rawID := range payloadutil.Slice(category["item_ids"]) {
			if itemID := strings.TrimSpace(payloadutil.String(rawID)); itemID != "" {
				itemIDs = append(itemIDs, itemID)
			}
		}
		for _, rawChild := range payloadutil.Slice(category["subcategories"]) {
			collect(rawChild)
		}
	}
	for _, raw := range payloadutil.Slice(payload["categories"]) {
		collect(raw)
	}
	for _, raw := range payloadutil.Slice(payload["subcategories"]) {
		collect(raw)
	}
	collect(payload["category"])
	return dedupeStrings(itemIDs)
}

func walkCategories(
	payload map[string]any,
	visit func(category map[string]any, parentSlug string, level int),
) {
	var walk func(map[string]any, string, int)
	walk = func(category map[string]any, parentSlug string, level int) {
		if category == nil {
			return
		}
		visit(category, parentSlug, level)
		slug := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(category["slug"], category["id"])))
		if slug != "" {
			parentSlug = slug
		}
		for _, rawChild := range payloadutil.Slice(category["subcategories"]) {
			walk(payloadutil.Map(rawChild), parentSlug, level+1)
		}
	}
	for _, raw := range payloadutil.Slice(payload["categories"]) {
		walk(payloadutil.Map(raw), "", 0)
	}
	for _, raw := range payloadutil.Slice(payload["subcategories"]) {
		walk(payloadutil.Map(raw), "", 0)
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dedupeItems(values []any) []any {
	indexByID := map[string]int{}
	out := make([]any, 0, len(values))
	for _, raw := range values {
		item := payloadutil.Map(raw)
		itemID := payloadItemID(item)
		if item == nil || itemID == "" {
			continue
		}
		key := strings.ToLower(itemID)
		if index, exists := indexByID[key]; exists {
			out[index] = catalogitem.MergeCurrentItem(
				payloadutil.Map(out[index]),
				item,
			)
			continue
		}
		indexByID[key] = len(out)
		out = append(out, item)
	}
	return out
}

func orderItemsByReferences(items []any, itemIDs []string) []any {
	byID := make(map[string]any, len(items))
	for _, item := range items {
		byID[strings.ToLower(payloadItemID(item))] = item
	}
	ordered := make([]any, 0, len(items))
	added := make(map[string]struct{}, len(items))
	for _, itemID := range itemIDs {
		key := strings.ToLower(strings.TrimSpace(itemID))
		item, exists := byID[key]
		if !exists {
			continue
		}
		ordered = append(ordered, item)
		added[key] = struct{}{}
	}
	for _, item := range items {
		itemID := strings.ToLower(payloadItemID(item))
		if _, exists := added[itemID]; exists {
			continue
		}
		ordered = append(ordered, item)
		added[itemID] = struct{}{}
	}
	return ordered
}

func payloadItemID(raw any) string {
	item := payloadutil.Map(raw)
	return strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(item["id"], item["item_id"])))
}

func materializedItem(item map[string]any) bool {
	if payloadItemID(item) == "" {
		return false
	}
	return strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		item["name"],
		item["title"],
	))) != ""
}

func cloneMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func firstNonEmpty(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(payloadutil.String(value)); text != "" {
			return text
		}
	}
	return ""
}
