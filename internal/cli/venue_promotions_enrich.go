package cli

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

const dynamicVenuePromotionSecondaryLimit = 0
const dynamicVenuePromotionRequestPause = 300 * time.Millisecond
const dynamicVenuePromotionRetryDelay = 2500 * time.Millisecond
const dynamicVenuePromotionMax429Retries = 0
const dynamicVenuePromotionFetchBudget = 12
const dynamicVenuePromotionRateLimitRetryBudget = 1
const dynamicVenuePromotionNoLocationRetryBudget = 2

func enrichVenueSearchRowsWithDynamicPromotions(
	ctx context.Context,
	deps Dependencies,
	data map[string]any,
	location *domain.Location,
	auth woltgateway.AuthContext,
) {
	rows := asSlice(data["items"])
	enrichVenueRowsWithDynamicPromotions(ctx, deps, rows, location, auth)
}

func enrichDiscoverFeedRowsWithDynamicPromotions(
	ctx context.Context,
	deps Dependencies,
	data map[string]any,
	location *domain.Location,
	auth woltgateway.AuthContext,
) {
	rows := []any{}
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		rows = append(rows, asSlice(section["items"])...)
	}
	enrichVenueRowsWithDynamicPromotions(ctx, deps, rows, location, auth)
}

func enrichVenueRowsWithDynamicPromotions(
	ctx context.Context,
	deps Dependencies,
	rows []any,
	location *domain.Location,
	auth woltgateway.AuthContext,
) {
	if len(rows) == 0 {
		return
	}

	type slugInfo struct {
		rows         []map[string]any
		count        int
		firstIndex   int
		hasPromotion bool
	}

	slugInfos := map[string]*slugInfo{}
	for idx, raw := range rows {
		row := asMap(raw)
		if row == nil {
			continue
		}
		slug := strings.TrimSpace(asString(row["slug"]))
		if slug == "" {
			continue
		}
		info, exists := slugInfos[slug]
		if !exists {
			info = &slugInfo{firstIndex: idx}
			slugInfos[slug] = info
		}
		info.rows = append(info.rows, row)
		info.count++
		if len(asSlice(row["promotions"])) > 0 {
			info.hasPromotion = true
		}
	}

	if len(slugInfos) == 0 {
		return
	}

	type candidate struct {
		slug string
		info *slugInfo
	}
	primary := make([]candidate, 0, len(slugInfos))
	secondary := make([]candidate, 0, len(slugInfos))
	for slug, info := range slugInfos {
		entry := candidate{slug: slug, info: info}
		if info.hasPromotion {
			primary = append(primary, entry)
			continue
		}
		secondary = append(secondary, entry)
	}
	sortCandidates := func(values []candidate) {
		sort.Slice(values, func(i, j int) bool {
			if values[i].info.count != values[j].info.count {
				return values[i].info.count > values[j].info.count
			}
			return values[i].info.firstIndex < values[j].info.firstIndex
		})
	}
	sortCandidates(primary)
	sortCandidates(secondary)

	cachedLabels := map[string][]string{}
	attempted := map[string]struct{}{}
	lastDynamicRequestAt := time.Time{}
	rateLimitRetryBudget := dynamicVenuePromotionRateLimitRetryBudget
	noLocationRetryBudget := dynamicVenuePromotionNoLocationRetryBudget

	resolveLabels := func(slug string) []string {
		labels, hasLabels := cachedLabels[slug]
		if !hasLabels {
			if _, seen := attempted[slug]; !seen {
				if len(attempted) >= dynamicVenuePromotionFetchBudget {
					return nil
				}
				attempted[slug] = struct{}{}
				payload, err := fetchDynamicVenuePayloadWithRetry(
					ctx,
					deps,
					slug,
					location,
					auth,
					&lastDynamicRequestAt,
				)
				if err != nil && isTooManyRequests(err) && rateLimitRetryBudget > 0 {
					rateLimitRetryBudget--
					payload, err = fetchDynamicVenuePayloadWithRetry(
						ctx,
						deps,
						slug,
						location,
						auth,
						&lastDynamicRequestAt,
					)
				}
				if err != nil && isTooManyRequests(err) && location != nil && noLocationRetryBudget > 0 {
					noLocationRetryBudget--
					payload, err = fetchDynamicVenuePayloadWithRetry(
						ctx,
						deps,
						slug,
						nil,
						auth,
						&lastDynamicRequestAt,
					)
				}
				if err == nil && len(payload) > 0 {
					labels = observability.ExtractVenuePromotionLabels(payload)
					cachedLabels[slug] = labels
					hasLabels = true
				}
			}
		}
		if !hasLabels {
			return nil
		}
		return labels
	}

	for _, entry := range primary {
		labels := resolveLabels(entry.slug)
		if len(labels) == 0 {
			continue
		}
		for _, row := range entry.info.rows {
			row["promotions"] = mergeVenuePromotionLabels(asSlice(row["promotions"]), labels)
		}
	}

	secondaryLimit := len(secondary)
	if secondaryLimit > dynamicVenuePromotionSecondaryLimit {
		secondaryLimit = dynamicVenuePromotionSecondaryLimit
	}
	for i := 0; i < secondaryLimit; i++ {
		entry := secondary[i]
		labels := resolveLabels(entry.slug)
		if len(labels) == 0 {
			continue
		}
		for _, row := range entry.info.rows {
			row["promotions"] = mergeVenuePromotionLabels(asSlice(row["promotions"]), labels)
		}
	}
}

func fetchDynamicVenuePayloadWithRetry(
	ctx context.Context,
	deps Dependencies,
	slug string,
	location *domain.Location,
	auth woltgateway.AuthContext,
	lastRequestAt *time.Time,
) (map[string]any, error) {
	var payload map[string]any
	var err error
	options := woltgateway.VenuePageDynamicOptions{
		Location: location,
		Auth:     auth,
	}
	for attempt := 0; attempt <= dynamicVenuePromotionMax429Retries; attempt++ {
		if *lastRequestAt != (time.Time{}) {
			wait := dynamicVenuePromotionRequestPause - time.Since(*lastRequestAt)
			if wait > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(wait):
				}
			}
		}

		payload, err = deps.Wolt.VenuePageDynamic(
			ctx,
			slug,
			options,
		)
		*lastRequestAt = time.Now()
		if err != nil && isUnauthorized(err) && options.Auth.HasCredentials() {
			// Dynamic venue endpoint rejects some bearer tokens; retry anonymously.
			options.Auth = woltgateway.AuthContext{}
			attempt--
			continue
		}
		if err == nil || !isTooManyRequests(err) || attempt == dynamicVenuePromotionMax429Retries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dynamicVenuePromotionRetryDelay):
		}
	}
	if err != nil && isTooManyRequests(err) {
		// Apply cooldown for subsequent venue dynamic calls in this command run.
		*lastRequestAt = time.Now().Add(dynamicVenuePromotionRetryDelay)
	}
	return payload, err
}

func isTooManyRequests(err error) bool {
	var upstreamErr *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	return upstreamErr.StatusCode == 429
}

func isUnauthorized(err error) bool {
	var upstreamErr *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	return upstreamErr.StatusCode == 401
}

func mergeVenuePromotionLabels(existing []any, extra []string) []any {
	out := []any{}
	seen := map[string]struct{}{}
	appendLabel := func(raw string) {
		label := strings.TrimSpace(raw)
		if label == "" {
			return
		}
		if _, exists := seen[label]; exists {
			return
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}

	for _, rawLabel := range existing {
		appendLabel(asString(rawLabel))
	}
	for _, label := range extra {
		appendLabel(label)
	}

	return out
}
