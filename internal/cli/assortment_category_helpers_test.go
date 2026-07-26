package cli

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

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

type venueContentProbeAPI struct {
	testWoltAPI
	status int
	calls  []woltgateway.AuthContext
}

func (p *venueContentProbeAPI) VenueContentByVenueSlug(
	_ context.Context,
	_ string,
	_ string,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	p.calls = append(p.calls, auth)
	return nil, &woltgateway.UpstreamRequestError{
		Method:     http.MethodGet,
		StatusCode: p.status,
	}
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
		"example-market",
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
		"example-market",
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

func TestLoadVenueContentDoesNotRetryTransientErrorsAnonymously(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			probe := &venueContentProbeAPI{status: status}
			auth := woltgateway.AuthContext{WToken: "saved-access"}

			payloads, warnings := loadVenueContentPayloads(
				context.Background(),
				Dependencies{Wolt: probe},
				"example-market",
				auth,
				1,
			)
			if len(payloads) != 0 || len(warnings) != 1 {
				t.Fatalf("payloads=%#v warnings=%v", payloads, warnings)
			}
			if len(probe.calls) != 2 {
				t.Fatalf("calls=%d, want bounded transient retry", len(probe.calls))
			}
			for _, requestAuth := range probe.calls {
				if !requestAuth.HasCredentials() {
					t.Fatalf("status %d caused anonymous retry: %#v", status, probe.calls)
				}
			}
		})
	}
}

func TestLoadAssortmentCategoryPayloadsLoadsAllCategories(t *testing.T) {
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

	payloads, warnings, complete, err := loadAssortmentCategoryPayloads(
		context.Background(),
		Dependencies{Wolt: probe},
		"example-market",
		"en",
		woltgateway.AuthContext{},
		assortmentPayload,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Fatal("expected complete category selection")
	}
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

func TestLoadAssortmentCategoryPayloadsCarriesPartialCompleteness(t *testing.T) {
	probe := &assortmentRequestProbeAPI{
		categoryFn: func(categorySlug string, _ woltgateway.AuthContext) (map[string]any, error) {
			itemID := categorySlug + "-item"
			item := map[string]any{"id": itemID}
			if categorySlug == "complete" {
				item["name"] = "Complete item"
			}
			return map[string]any{
				"category": map[string]any{
					"slug":     categorySlug,
					"item_ids": []any{itemID},
				},
				"items": []any{item},
			}, nil
		},
		itemsFn: func(itemIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"items": []any{map[string]any{"id": itemIDs[0]}},
			}, nil
		},
	}
	root := map[string]any{
		"categories": []any{
			map[string]any{"slug": "complete"},
			map[string]any{"slug": "partial"},
		},
	}

	payloads, warnings, complete, err := loadAssortmentCategoryPayloads(
		context.Background(),
		Dependencies{Wolt: probe},
		"example-market",
		"en",
		woltgateway.AuthContext{},
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if complete || len(payloads) != 2 || len(warnings) == 0 {
		t.Fatalf(
			"aggregate load complete=%v payloads=%d warnings=%v",
			complete,
			len(payloads),
			warnings,
		)
	}
}

func TestLoadAssortmentCategoryPayloadsPropagatesContextErrors(t *testing.T) {
	assortmentPayload := map[string]any{
		"categories": []any{
			map[string]any{"slug": "cat-a", "subcategories": []any{}},
			map[string]any{"slug": "cat-b", "subcategories": []any{}},
		},
	}
	for _, test := range []struct {
		name       string
		contextErr error
	}{
		{name: "canceled", contextErr: context.Canceled},
		{name: "deadline", contextErr: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			if errors.Is(test.contextErr, context.DeadlineExceeded) {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()

			_, warnings, complete, err := loadAssortmentCategoryPayloads(
				ctx,
				Dependencies{Wolt: &assortmentRequestProbeAPI{}},
				"example-market",
				"en",
				woltgateway.AuthContext{},
				assortmentPayload,
			)
			if !errors.Is(err, test.contextErr) {
				t.Fatalf("error = %v, want %v", err, test.contextErr)
			}
			if complete {
				t.Fatal("canceled category selection reported complete")
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want no partial-data warning", warnings)
			}
		})
	}
}

func TestBuildVenueCategoriesDataUsesSharedCatalogShape(t *testing.T) {
	data := buildVenueCategoriesData("venue-1", map[string]any{
		"loading_strategy": " partial ",
		"categories": []any{
			map[string]any{
				"id":    "root-id",
				"slug":  "root",
				"title": "Root",
				"subcategories": []any{
					map[string]any{
						"id":       "leaf-id",
						"slug":     "leaf",
						"name":     "Leaf",
						"item_ids": []any{"item-1", "item-2"},
					},
				},
			},
		},
	})
	if data["venue_id"] != "venue-1" || data["loading_strategy"] != "partial" {
		t.Fatalf("category metadata = %#v", data)
	}
	rows := asSlice(data["categories"])
	if len(rows) != 2 {
		t.Fatalf("categories = %#v", rows)
	}
	root := asMap(rows[0])
	if root["id"] != "root-id" ||
		root["slug"] != "root" ||
		root["name"] != "Root" ||
		root["parent_slug"] != nil ||
		root["level"] != 0 ||
		root["leaf"] != false ||
		root["item_refs_count"] != 0 {
		t.Fatalf("root category = %#v", root)
	}
	leaf := asMap(rows[1])
	if leaf["id"] != "leaf-id" ||
		leaf["slug"] != "leaf" ||
		leaf["name"] != "Leaf" ||
		leaf["parent_slug"] != "root" ||
		leaf["level"] != 1 ||
		leaf["leaf"] != true ||
		leaf["item_refs_count"] != 2 {
		t.Fatalf("leaf category = %#v", leaf)
	}
}
