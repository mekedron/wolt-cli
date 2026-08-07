package mcpserver

import (
	"context"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestHandleSearchItemsUsesSharedGlobalContract(t *testing.T) {
	api := &stubWolt{
		searchItemsFn: func(_ context.Context, location domain.Location, query string, limit int, auth woltgateway.AuthContext) (map[string]any, error) {
			if location.Lat != 10.25 || location.Lon != 20.5 || query != "semantic query" || limit != domain.GlobalItemSearchDefaultLimit {
				t.Fatalf("search request = location %#v, query %q, limit %d", location, query, limit)
			}
			if auth.WToken != "synthetic-token" {
				t.Fatalf("auth token = %q", auth.WToken)
			}
			return map[string]any{
				"sections": []any{map[string]any{
					"items": []any{map[string]any{
						"menu_item": map[string]any{
							"id":           "item-a",
							"venue_id":     "venue-a",
							"venue_name":   "Synthetic Venue",
							"name":         "Server-ranked semantic match",
							"price":        875,
							"currency":     "EUR",
							"is_available": true,
						},
					}},
				}},
			}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: api,
		Profiles: &stubProfiles{profile: domain.Profile{
			WToken: "synthetic-token",
		}},
	})

	_, result, err := tc.handleSearchItems(context.Background(), nil, SearchItemsInput{
		LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
		Query:         " semantic query ",
	})
	if err != nil {
		t.Fatalf("handleSearchItems() error = %v", err)
	}
	if result.LocationSource != "explicit" || result.Data["completeness"] != "unknown" {
		t.Fatalf("result metadata = %#v", result)
	}
	items := asSlice(result.Data["items"])
	if len(items) != 1 || asString(asMap(items[0])["name"]) != "Server-ranked semantic match" {
		t.Fatalf("items = %#v", items)
	}
}

func TestHandleSearchItemsValidatesLimitBeforeLocation(t *testing.T) {
	tc := newToolCtx(Deps{Wolt: &stubWolt{}})
	_, _, err := tc.handleSearchItems(context.Background(), nil, SearchItemsInput{
		Query: "query",
		Limit: domain.GlobalItemSearchMaxLimit + 1,
	})
	if err == nil {
		t.Fatal("invalid limit did not return an error")
	}
}
