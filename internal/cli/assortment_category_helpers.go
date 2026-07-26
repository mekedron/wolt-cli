package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/catalogload"
)

const assortmentCategoryConcurrency = 8

func assortmentCatalogRetryPolicy() catalogload.RetryPolicy {
	return catalogload.RetryPolicy{
		Attempts: 2,
		Delay:    120 * time.Millisecond,
		MaxDelay: 5 * time.Second,
	}
}

func loadAssortmentCategory(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	categorySlug string,
	language string,
	auth woltgateway.AuthContext,
) (catalogload.CategoryLoad, error) {
	return catalogload.LoadCategoryWithRetry(
		ctx,
		deps.Wolt,
		venueSlug,
		categorySlug,
		language,
		auth,
		assortmentCatalogRetryPolicy(),
	)
}

func loadAssortmentCategoryPayloads(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	language string,
	auth woltgateway.AuthContext,
	assortmentPayload map[string]any,
) ([]map[string]any, []string, bool, error) {
	slugs := catalogload.LoadableCategorySlugs(assortmentPayload)
	if len(slugs) == 0 {
		return nil, nil, true, nil
	}
	return loadAssortmentCategoryPayloadsParallel(ctx, deps, venueSlug, language, auth, slugs)
}

type assortmentCategoryLoadResult struct {
	index    int
	payload  map[string]any
	warnings []string
	complete bool
	err      error
}

func loadAssortmentCategoryPayloadsParallel(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	language string,
	auth woltgateway.AuthContext,
	slugs []string,
) ([]map[string]any, []string, bool, error) {
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
				categoryLoad, err := loadAssortmentCategory(
					ctx,
					deps,
					venueSlug,
					categorySlug,
					language,
					auth,
				)
				if err != nil {
					results <- assortmentCategoryLoadResult{index: idx, err: err}
					continue
				}
				if len(categoryLoad.Payload) == 0 {
					results <- assortmentCategoryLoadResult{index: idx}
					continue
				}
				results <- assortmentCategoryLoadResult{
					index:    idx,
					payload:  categoryLoad.Payload,
					warnings: categoryLoad.Warnings,
					complete: categoryLoad.Complete,
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

	orderedResults := make([]assortmentCategoryLoadResult, len(slugs))
	loadedCount := 0
	complete := true
	var contextErr error
	for result := range results {
		if errors.Is(result.err, context.Canceled) ||
			errors.Is(result.err, context.DeadlineExceeded) {
			if contextErr == nil {
				contextErr = result.err
			}
			continue
		}
		if result.err != nil {
			complete = false
			continue
		}
		if len(result.payload) == 0 {
			complete = false
			continue
		}
		if !result.complete {
			complete = false
		}
		orderedResults[result.index] = result
		loadedCount++
	}
	if contextErr != nil {
		return nil, nil, false, contextErr
	}
	for _, result := range orderedResults {
		if len(result.payload) == 0 {
			continue
		}
		payloads = append(payloads, result.payload)
		warnings = append(warnings, result.warnings...)
	}

	if loadedCount == 0 {
		warnings = append(warnings, "assortment category endpoints unavailable for full menu fallback")
	} else if loadedCount < len(slugs) {
		warnings = append(
			warnings,
			"full menu fallback is partially limited upstream; some category pages were unavailable",
		)
	}
	if loadedCount < len(slugs) {
		complete = false
	}
	return payloads, warnings, complete, nil
}

func requestAssortmentItemsPayload(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	return catalogload.RequestPayload(
		ctx,
		auth,
		assortmentCatalogRetryPolicy(),
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return deps.Wolt.AssortmentItemsByVenueSlug(ctx, venueSlug, itemIDs, requestAuth)
		},
	)
}

func requestAssortmentItemsSearchPayload(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	return catalogload.RequestPayload(
		ctx,
		auth,
		assortmentCatalogRetryPolicy(),
		func(ctx context.Context, requestAuth woltgateway.AuthContext) (map[string]any, error) {
			return deps.Wolt.AssortmentItemsSearchByVenueSlug(
				ctx,
				venueSlug,
				query,
				language,
				requestAuth,
			)
		},
	)
}
