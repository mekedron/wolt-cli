package mcpserver

import (
	"context"
	"net/http"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestPublicAssortmentReadsFallBackToAnonymousAfter401(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(context.Context, *ToolCtx, woltgateway.AuthContext) (map[string]any, error)
	}{
		{
			name: "search",
			invoke: func(ctx context.Context, tc *ToolCtx, auth woltgateway.AuthContext) (map[string]any, error) {
				return requestAssortmentSearch(ctx, tc, "venue", "fish", "en", auth)
			},
		},
		{
			name: "items",
			invoke: func(ctx context.Context, tc *ToolCtx, auth woltgateway.AuthContext) (map[string]any, error) {
				return requestAssortmentItems(ctx, tc, "venue", []string{"item"}, auth)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := []bool{}
			tc := newToolCtx(Deps{Wolt: &stubWolt{
				assortmentSearchFn: func(
					_ context.Context,
					_ string,
					_ string,
					_ string,
					auth woltgateway.AuthContext,
				) (map[string]any, error) {
					return assortmentAuthProbe(&calls, auth)
				},
				assortmentItemsFn: func(
					_ context.Context,
					_ string,
					_ []string,
					auth woltgateway.AuthContext,
				) (map[string]any, error) {
					return assortmentAuthProbe(&calls, auth)
				},
			}})
			payload, err := test.invoke(
				context.Background(),
				tc,
				woltgateway.AuthContext{WToken: "stale"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if payload == nil {
				t.Fatal("anonymous retry returned no payload")
			}
			if len(calls) != 2 || !calls[0] || calls[1] {
				t.Fatalf("credentialed calls = %v, want [true false]", calls)
			}
		})
	}
}

func assortmentAuthProbe(calls *[]bool, auth woltgateway.AuthContext) (map[string]any, error) {
	*calls = append(*calls, auth.HasCredentials())
	if auth.HasCredentials() {
		return nil, &woltgateway.UpstreamRequestError{
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
		}
	}
	return map[string]any{"items": []any{}}, nil
}

func TestPublicAssortmentReadsDoNotMaskRateLimitWithAnonymousRetry(t *testing.T) {
	searchCalls := 0
	itemCalls := 0
	rateLimit := func(method string) error {
		return &woltgateway.UpstreamRequestError{
			Method:     method,
			StatusCode: http.StatusTooManyRequests,
		}
	}
	tc := newToolCtx(Deps{Wolt: &stubWolt{
		assortmentSearchFn: func(
			context.Context,
			string,
			string,
			string,
			woltgateway.AuthContext,
		) (map[string]any, error) {
			searchCalls++
			return nil, rateLimit(http.MethodPost)
		},
		assortmentItemsFn: func(
			context.Context,
			string,
			[]string,
			woltgateway.AuthContext,
		) (map[string]any, error) {
			itemCalls++
			return nil, rateLimit(http.MethodPost)
		},
	}})
	auth := woltgateway.AuthContext{WToken: "valid"}

	if _, err := requestAssortmentSearch(context.Background(), tc, "venue", "fish", "en", auth); err == nil {
		t.Fatal("search rate limit was swallowed")
	}
	if _, err := requestAssortmentItems(context.Background(), tc, "venue", []string{"item"}, auth); err == nil {
		t.Fatal("item rate limit was swallowed")
	}
	if searchCalls != 1 || itemCalls != 1 {
		t.Fatalf("calls = search:%d items:%d, want one credentialed call each", searchCalls, itemCalls)
	}
}
