package cli

import (
	"context"
	"net/http"
	"sync"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type assortmentRequestProbeAPI struct {
	testWoltAPI
	mu            sync.Mutex
	categoryCalls []woltgateway.AuthContext
	itemsCalls    []woltgateway.AuthContext
	searchCalls   []woltgateway.AuthContext
	categoryFn    func(categorySlug string, auth woltgateway.AuthContext) (map[string]any, error)
	itemsFn       func(itemIDs []string, auth woltgateway.AuthContext) (map[string]any, error)
	searchFn      func(query string, language string, auth woltgateway.AuthContext) (map[string]any, error)
}

func (m *assortmentRequestProbeAPI) AssortmentCategoryByVenueSlug(
	_ context.Context,
	_ string,
	categorySlug string,
	_ string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	m.mu.Lock()
	m.categoryCalls = append(m.categoryCalls, auth)
	m.mu.Unlock()
	if m.categoryFn != nil {
		return m.categoryFn(categorySlug, auth)
	}
	return map[string]any{}, nil
}

func (m *assortmentRequestProbeAPI) AssortmentItemsByVenueSlug(
	_ context.Context,
	_ string,
	itemIDs []string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	m.mu.Lock()
	m.itemsCalls = append(m.itemsCalls, auth)
	m.mu.Unlock()
	if m.itemsFn != nil {
		return m.itemsFn(itemIDs, auth)
	}
	return map[string]any{}, nil
}

func (m *assortmentRequestProbeAPI) AssortmentItemsSearchByVenueSlug(
	_ context.Context,
	_ string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	m.mu.Lock()
	m.searchCalls = append(m.searchCalls, auth)
	m.mu.Unlock()
	if m.searchFn != nil {
		return m.searchFn(query, language, auth)
	}
	return map[string]any{}, nil
}

func TestRequestAssortmentCategoryPayloadFallsBackAfterSingle401(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		categoryFn: func(_ string, auth woltgateway.AuthContext) (map[string]any, error) {
			if auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     http.MethodGet,
					URL:        "https://example.test/category",
					StatusCode: 401,
				}
			}
			return map[string]any{"items": []any{}}, nil
		},
	}

	payload, err := requestAssortmentCategoryPayload(
		context.Background(),
		Dependencies{Wolt: probe},
		"wolt-market-niittari",
		"viikon-parhaat-12",
		"en",
		woltgateway.AuthContext{WToken: "expired-token"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
	if len(probe.categoryCalls) != 2 {
		t.Fatalf("expected two calls (auth then anon), got %d", len(probe.categoryCalls))
	}
	if !probe.categoryCalls[0].HasCredentials() {
		t.Fatal("expected first category call to use credentials")
	}
	if probe.categoryCalls[1].HasCredentials() {
		t.Fatal("expected second category call to be anonymous")
	}
}

func TestRequestAssortmentItemsPayloadFallsBackAfterSingle401(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		itemsFn: func(_ []string, auth woltgateway.AuthContext) (map[string]any, error) {
			if auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     http.MethodPost,
					URL:        "https://example.test/items",
					StatusCode: 401,
				}
			}
			return map[string]any{"items": []any{}}, nil
		},
	}

	payload, err := requestAssortmentItemsPayload(
		context.Background(),
		Dependencies{Wolt: probe},
		"wolt-market-niittari",
		[]string{"item-1", "item-2"},
		woltgateway.AuthContext{WToken: "expired-token"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
	if len(probe.itemsCalls) != 2 {
		t.Fatalf("expected two calls (auth then anon), got %d", len(probe.itemsCalls))
	}
	if !probe.itemsCalls[0].HasCredentials() {
		t.Fatal("expected first items call to use credentials")
	}
	if probe.itemsCalls[1].HasCredentials() {
		t.Fatal("expected second items call to be anonymous")
	}
}

func TestRequestAssortmentItemsSearchPayloadFallsBackAfterSingle401(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		searchFn: func(_ string, _ string, auth woltgateway.AuthContext) (map[string]any, error) {
			if auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{
					Method:     http.MethodPost,
					URL:        "https://example.test/items/search",
					StatusCode: 401,
				}
			}
			return map[string]any{"items": []any{}}, nil
		},
	}

	payload, err := requestAssortmentItemsSearchPayload(
		context.Background(),
		Dependencies{Wolt: probe},
		"wolt-market-niittari",
		"milk",
		"en",
		woltgateway.AuthContext{WToken: "expired-token"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
	if len(probe.searchCalls) != 2 {
		t.Fatalf("expected two calls (auth then anon), got %d", len(probe.searchCalls))
	}
	if !probe.searchCalls[0].HasCredentials() {
		t.Fatal("expected first search call to use credentials")
	}
	if probe.searchCalls[1].HasCredentials() {
		t.Fatal("expected second search call to be anonymous")
	}
}

func TestLoadAssortmentCategoryPayloadsStopsAtTargetCount(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		categoryFn: func(categorySlug string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemID := categorySlug + "-item"
			return map[string]any{
				"category": map[string]any{
					"slug":     categorySlug,
					"item_ids": []any{itemID},
				},
				"items": []any{
					map[string]any{"id": itemID, "name": categorySlug},
				},
			}, nil
		},
	}
	assortmentPayload := map[string]any{
		"categories": []any{
			map[string]any{"slug": "cat-a", "subcategories": []any{}},
			map[string]any{"slug": "cat-b", "subcategories": []any{}},
			map[string]any{"slug": "cat-c", "subcategories": []any{}},
		},
	}

	payloads, warnings := loadAssortmentCategoryPayloads(
		context.Background(),
		Dependencies{Wolt: probe},
		"wolt-market-niittari",
		"en",
		woltgateway.AuthContext{},
		assortmentPayload,
		2,
	)
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected two payloads due target item count, got %d", len(payloads))
	}
	if len(probe.categoryCalls) != 2 {
		t.Fatalf("expected two category calls due target item count, got %d", len(probe.categoryCalls))
	}
}

func TestLoadAssortmentCategoryPayloadsParallelLoadsAllWhenNoTarget(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		categoryFn: func(categorySlug string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemID := categorySlug + "-item"
			return map[string]any{
				"category": map[string]any{
					"slug":     categorySlug,
					"item_ids": []any{itemID},
				},
				"items": []any{
					map[string]any{"id": itemID, "name": categorySlug},
				},
			}, nil
		},
	}
	assortmentPayload := map[string]any{
		"categories": []any{
			map[string]any{"slug": "cat-a", "subcategories": []any{}},
			map[string]any{"slug": "cat-b", "subcategories": []any{}},
			map[string]any{"slug": "cat-c", "subcategories": []any{}},
		},
	}

	payloads, warnings := loadAssortmentCategoryPayloads(
		context.Background(),
		Dependencies{Wolt: probe},
		"wolt-market-niittari",
		"en",
		woltgateway.AuthContext{},
		assortmentPayload,
		0,
	)
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(payloads) != 3 {
		t.Fatalf("expected all payloads in parallel mode, got %d", len(payloads))
	}
	if len(probe.categoryCalls) != 3 {
		t.Fatalf("expected three category calls in parallel mode, got %d", len(probe.categoryCalls))
	}
}
