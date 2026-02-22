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
const dynamicVenuePromotionFetchBudget = 20
const dynamicVenuePromotionRateLimitRetryBudget = 1
const staticVenueWoltPlusFetchBudget = 25
const staticVenueWoltPlusRequestPause = 120 * time.Millisecond

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
	sectionsItems := make([][]any, 0, len(asSlice(data["sections"])))
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		if section == nil {
			continue
		}
		sectionsItems = append(sectionsItems, asSlice(section["items"]))
	}
	// Round-robin rows across sections so enrichment budget covers more sections.
	rows := []any{}
	for depth := 0; ; depth++ {
		added := false
		for _, items := range sectionsItems {
			if depth < len(items) {
				rows = append(rows, items[depth])
				added = true
			}
		}
		if !added {
			break
		}
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
		maxRating    float64
		hasRating    bool
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
		if rating, ok := rowRating(row); ok {
			if !info.hasRating || rating > info.maxRating {
				info.maxRating = rating
				info.hasRating = true
			}
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
			if values[i].info.hasRating != values[j].info.hasRating {
				return values[i].info.hasRating
			}
			if values[i].info.hasRating && values[i].info.maxRating != values[j].info.maxRating {
				return values[i].info.maxRating > values[j].info.maxRating
			}
			if values[i].info.firstIndex != values[j].info.firstIndex {
				return values[i].info.firstIndex < values[j].info.firstIndex
			}
			return values[i].info.count > values[j].info.count
		})
	}
	sortCandidates(primary)
	sortCandidates(secondary)

	cachedLabels := map[string][]string{}
	cachedWoltPlus := map[string]bool{}
	attempted := map[string]struct{}{}
	staticAttempted := map[string]struct{}{}
	lastDynamicRequestAt := time.Time{}
	lastStaticRequestAt := time.Time{}
	rateLimitRetryBudget := dynamicVenuePromotionRateLimitRetryBudget

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
	resolveWoltPlus := func(slug string) bool {
		if value, exists := cachedWoltPlus[slug]; exists {
			return value
		}
		if _, attempted := staticAttempted[slug]; attempted {
			return false
		}
		staticAttempted[slug] = struct{}{}
		if lastStaticRequestAt != (time.Time{}) {
			wait := staticVenueWoltPlusRequestPause - time.Since(lastStaticRequestAt)
			if wait > 0 {
				select {
				case <-ctx.Done():
					return false
				case <-time.After(wait):
				}
			}
		}
		payload, err := deps.Wolt.VenuePageStatic(ctx, slug)
		lastStaticRequestAt = time.Now()
		if err != nil || len(payload) == 0 {
			return false
		}
		isWoltPlus := observability.ExtractVenueWoltPlus(payload)
		cachedWoltPlus[slug] = isWoltPlus
		return isWoltPlus
	}

	for _, entry := range primary {
		labels := resolveLabels(entry.slug)
		for _, row := range entry.info.rows {
			if len(labels) > 0 {
				row["promotions"] = mergeVenuePromotionLabels(asSlice(row["promotions"]), labels)
			}
			if !asBool(row["wolt_plus"]) && resolveWoltPlus(entry.slug) {
				row["wolt_plus"] = true
			}
		}
	}

	secondaryLimit := len(secondary)
	if secondaryLimit > dynamicVenuePromotionSecondaryLimit {
		secondaryLimit = dynamicVenuePromotionSecondaryLimit
	}
	for i := 0; i < secondaryLimit; i++ {
		entry := secondary[i]
		labels := resolveLabels(entry.slug)
		for _, row := range entry.info.rows {
			if len(labels) > 0 {
				row["promotions"] = mergeVenuePromotionLabels(asSlice(row["promotions"]), labels)
			}
			if !asBool(row["wolt_plus"]) && resolveWoltPlus(entry.slug) {
				row["wolt_plus"] = true
			}
		}
	}

	staticCandidates := make([]candidate, 0, len(slugInfos))
	for slug, info := range slugInfos {
		needsWoltPlus := false
		for _, row := range info.rows {
			if !asBool(row["wolt_plus"]) {
				needsWoltPlus = true
				break
			}
		}
		if !needsWoltPlus {
			continue
		}
		staticCandidates = append(staticCandidates, candidate{slug: slug, info: info})
	}
	sort.Slice(staticCandidates, func(i, j int) bool {
		if staticCandidates[i].info.hasRating != staticCandidates[j].info.hasRating {
			return staticCandidates[i].info.hasRating
		}
		if staticCandidates[i].info.hasRating && staticCandidates[i].info.maxRating != staticCandidates[j].info.maxRating {
			return staticCandidates[i].info.maxRating > staticCandidates[j].info.maxRating
		}
		return staticCandidates[i].info.firstIndex < staticCandidates[j].info.firstIndex
	})
	staticLimit := len(staticCandidates)
	if staticLimit > staticVenueWoltPlusFetchBudget {
		staticLimit = staticVenueWoltPlusFetchBudget
	}
	for i := 0; i < staticLimit; i++ {
		entry := staticCandidates[i]
		if !resolveWoltPlus(entry.slug) {
			continue
		}
		for _, row := range entry.info.rows {
			row["wolt_plus"] = true
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

func rowRating(row map[string]any) (float64, bool) {
	raw, exists := row["rating"]
	if !exists {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}
