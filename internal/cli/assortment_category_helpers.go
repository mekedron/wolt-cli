package cli

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

const assortmentItemsBatchSize = 80
const assortmentCategoryConcurrency = 8

func collectAssortmentCategorySlugs(assortmentPayload map[string]any) []string {
	slugs := []string{}
	seen := map[string]struct{}{}

	var walkCategory func(category map[string]any)
	walkCategory = func(category map[string]any) {
		if category == nil {
			return
		}
		subcategories := asSlice(category["subcategories"])
		slug := strings.TrimSpace(asString(category["slug"]))
		shouldIncludeSlug := len(subcategories) == 0 || len(asSlice(category["item_ids"])) > 0
		if shouldIncludeSlug && slug != "" {
			if _, exists := seen[slug]; !exists {
				seen[slug] = struct{}{}
				slugs = append(slugs, slug)
			}
		}
		for _, rawSubcategory := range subcategories {
			walkCategory(asMap(rawSubcategory))
		}
	}

	for _, rawCategory := range asSlice(assortmentPayload["categories"]) {
		walkCategory(asMap(rawCategory))
	}
	for _, rawSubcategory := range asSlice(assortmentPayload["subcategories"]) {
		walkCategory(asMap(rawSubcategory))
	}
	return slugs
}

func loadAssortmentCategoryPayloads(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	language string,
	auth woltgateway.AuthContext,
	assortmentPayload map[string]any,
	targetItemCount int,
) ([]map[string]any, []string) {
	slugs := collectAssortmentCategorySlugs(assortmentPayload)
	if len(slugs) == 0 {
		return nil, nil
	}
	if targetItemCount > 0 {
		return loadAssortmentCategoryPayloadsSequential(
			ctx,
			deps,
			venueSlug,
			language,
			auth,
			slugs,
			targetItemCount,
		)
	}
	return loadAssortmentCategoryPayloadsParallel(ctx, deps, venueSlug, language, auth, slugs)
}

func loadAssortmentCategoryPayloadsSequential(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	language string,
	auth woltgateway.AuthContext,
	slugs []string,
	targetItemCount int,
) ([]map[string]any, []string) {
	payloads := make([]map[string]any, 0, len(slugs))
	warnings := []string{}
	loadedCount := 0
	collectedItemIDs := map[string]struct{}{}
	reachedTargetItemCount := false
	for _, categorySlug := range slugs {
		categoryPayload, err := requestAssortmentCategoryPayload(ctx, deps, venueSlug, categorySlug, language, auth)
		if err != nil {
			continue
		}
		if len(categoryPayload) == 0 {
			continue
		}
		loadedCount++
		hydratedPayload := hydrateAssortmentCategoryItems(ctx, deps, venueSlug, categoryPayload, auth)
		payloads = append(payloads, hydratedPayload)
		for _, itemID := range payloadItemIDs(hydratedPayload) {
			collectedItemIDs[itemID] = struct{}{}
		}
		if len(collectedItemIDs) >= targetItemCount {
			reachedTargetItemCount = true
			break
		}
	}
	if loadedCount == 0 {
		warnings = append(warnings, "assortment category endpoints unavailable for full menu fallback")
	} else if loadedCount < len(slugs) && !reachedTargetItemCount {
		warnings = append(
			warnings,
			"full menu fallback is partially limited upstream; some category pages were unavailable",
		)
	}
	return payloads, warnings
}

type assortmentCategoryLoadResult struct {
	index   int
	payload map[string]any
}

func loadAssortmentCategoryPayloadsParallel(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	language string,
	auth woltgateway.AuthContext,
	slugs []string,
) ([]map[string]any, []string) {
	payloads := make([]map[string]any, 0, len(slugs))
	warnings := []string{}
	workerCount := assortmentCategoryConcurrency
	if len(slugs) < workerCount {
		workerCount = len(slugs)
	}

	jobs := make(chan int)
	results := make(chan assortmentCategoryLoadResult, len(slugs))
	workers := sync.WaitGroup{}
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for idx := range jobs {
				categorySlug := slugs[idx]
				categoryPayload, err := requestAssortmentCategoryPayload(
					ctx,
					deps,
					venueSlug,
					categorySlug,
					language,
					auth,
				)
				if err != nil || len(categoryPayload) == 0 {
					results <- assortmentCategoryLoadResult{index: idx}
					continue
				}
				hydratedPayload := hydrateAssortmentCategoryItems(ctx, deps, venueSlug, categoryPayload, auth)
				results <- assortmentCategoryLoadResult{
					index:   idx,
					payload: hydratedPayload,
				}
			}
		}()
	}
	for idx := range slugs {
		jobs <- idx
	}
	close(jobs)
	workers.Wait()
	close(results)

	orderedPayloads := make([]map[string]any, len(slugs))
	loadedCount := 0
	for result := range results {
		if len(result.payload) == 0 {
			continue
		}
		orderedPayloads[result.index] = result.payload
		loadedCount++
	}
	for _, payload := range orderedPayloads {
		if len(payload) == 0 {
			continue
		}
		payloads = append(payloads, payload)
	}

	if loadedCount == 0 {
		warnings = append(warnings, "assortment category endpoints unavailable for full menu fallback")
	} else if loadedCount < len(slugs) {
		warnings = append(
			warnings,
			"full menu fallback is partially limited upstream; some category pages were unavailable",
		)
	}
	return payloads, warnings
}

func payloadItemIDs(payload map[string]any) []string {
	itemIDs := categoryPayloadItemIDs(payload)
	for _, rawItem := range asSlice(payload["items"]) {
		item := asMap(rawItem)
		if item == nil {
			continue
		}
		itemID := strings.TrimSpace(asString(coalesceAny(item["id"], item["item_id"])))
		if itemID == "" {
			continue
		}
		itemIDs = append(itemIDs, itemID)
	}
	return dedupeStrings(itemIDs)
}

func hydrateAssortmentCategoryItems(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	categoryPayload map[string]any,
	auth woltgateway.AuthContext,
) map[string]any {
	if len(asSlice(categoryPayload["items"])) > 0 {
		return categoryPayload
	}
	itemIDs := categoryPayloadItemIDs(categoryPayload)
	if len(itemIDs) == 0 {
		return categoryPayload
	}

	collectedItems := []any{}
	collectedOptions := []any{}
	for _, batch := range batchStrings(itemIDs, assortmentItemsBatchSize) {
		itemsPayload, err := requestAssortmentItemsPayload(ctx, deps, venueSlug, batch, auth)
		if err != nil {
			continue
		}
		collectedItems = append(collectedItems, asSlice(itemsPayload["items"])...)
		if len(collectedOptions) == 0 {
			collectedOptions = asSlice(coalesceAny(itemsPayload["options"], itemsPayload["option_groups"]))
		}
	}
	collectedItems = dedupeItemObjectsByID(collectedItems)
	if len(collectedItems) == 0 {
		return categoryPayload
	}

	merged := map[string]any{}
	for key, value := range categoryPayload {
		merged[key] = value
	}
	merged["items"] = collectedItems
	if len(collectedOptions) > 0 {
		merged["options"] = collectedOptions
		merged["option_groups"] = collectedOptions
	}
	return merged
}

func categoryPayloadItemIDs(categoryPayload map[string]any) []string {
	itemIDs := []string{}
	for _, rawCategory := range asSlice(categoryPayload["categories"]) {
		category := asMap(rawCategory)
		if category == nil {
			continue
		}
		for _, rawItemID := range asSlice(category["item_ids"]) {
			itemID := strings.TrimSpace(asString(rawItemID))
			if itemID == "" {
				continue
			}
			itemIDs = append(itemIDs, itemID)
		}
	}
	for _, rawItemID := range asSlice(asMap(categoryPayload["category"])["item_ids"]) {
		itemID := strings.TrimSpace(asString(rawItemID))
		if itemID == "" {
			continue
		}
		itemIDs = append(itemIDs, itemID)
	}
	return dedupeStrings(itemIDs)
}

func requestAssortmentCategoryPayload(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	categorySlug string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	authCandidates := []woltgateway.AuthContext{auth}
	if auth.HasCredentials() {
		authCandidates = append(authCandidates, woltgateway.AuthContext{})
	}

	var lastErr error
	for _, authCandidate := range authCandidates {
		for attempt := 0; attempt < 2; attempt++ {
			payload, err := deps.Wolt.AssortmentCategoryByVenueSlug(
				ctx,
				venueSlug,
				categorySlug,
				language,
				authCandidate,
			)
			if err == nil {
				return payload, nil
			}
			lastErr = err
			if !shouldRetryUpstreamRequest(err) {
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func requestAssortmentItemsPayload(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	authCandidates := []woltgateway.AuthContext{auth}
	if auth.HasCredentials() {
		authCandidates = append(authCandidates, woltgateway.AuthContext{})
	}

	var lastErr error
	for _, authCandidate := range authCandidates {
		for attempt := 0; attempt < 2; attempt++ {
			payload, err := deps.Wolt.AssortmentItemsByVenueSlug(ctx, venueSlug, itemIDs, authCandidate)
			if err == nil {
				return payload, nil
			}
			lastErr = err
			if !shouldRetryUpstreamRequest(err) {
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func requestAssortmentItemsSearchPayload(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	authCandidates := []woltgateway.AuthContext{auth}
	if auth.HasCredentials() {
		authCandidates = append(authCandidates, woltgateway.AuthContext{})
	}

	var lastErr error
	for _, authCandidate := range authCandidates {
		for attempt := 0; attempt < 2; attempt++ {
			payload, err := deps.Wolt.AssortmentItemsSearchByVenueSlug(ctx, venueSlug, query, language, authCandidate)
			if err == nil {
				return payload, nil
			}
			lastErr = err
			if !shouldRetryUpstreamRequest(err) {
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
	}
	return nil, lastErr
}

func shouldRetryUpstreamRequest(err error) bool {
	if err == nil {
		return false
	}
	var upstreamErr *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstreamErr) {
		return true
	}
	if upstreamErr.StatusCode == 0 {
		return true
	}
	if upstreamErr.StatusCode == 429 {
		return true
	}
	if upstreamErr.StatusCode >= 500 {
		return true
	}
	return false
}

func batchStrings(values []string, batchSize int) [][]string {
	if batchSize <= 0 {
		batchSize = len(values)
	}
	batches := [][]string{}
	for start := 0; start < len(values); start += batchSize {
		end := start + batchSize
		if end > len(values) {
			end = len(values)
		}
		batches = append(batches, values[start:end])
	}
	return batches
}

func dedupeItemObjectsByID(items []any) []any {
	if len(items) == 0 {
		return items
	}
	seen := map[string]struct{}{}
	out := make([]any, 0, len(items))
	for _, rawItem := range items {
		item := asMap(rawItem)
		if item == nil {
			continue
		}
		itemID := strings.TrimSpace(asString(coalesceAny(item["id"], item["item_id"])))
		if itemID == "" {
			continue
		}
		if _, ok := seen[itemID]; ok {
			continue
		}
		seen[itemID] = struct{}{}
		out = append(out, item)
	}
	return out
}
