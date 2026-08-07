package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestItemsCommandRequestsGlobalSearchAndEmitsCompleteMachineRows(t *testing.T) {
	called := false
	api := &testWoltAPI{
		searchItemsFn: func(_ context.Context, location domain.Location, query string, limit int, auth woltgateway.AuthContext) (map[string]any, error) {
			called = true
			if location.Lat != 10.25 || location.Lon != 20.5 || query != "semantic query" || limit != 7 {
				t.Fatalf("search request = location %#v, query %q, limit %d", location, query, limit)
			}
			if auth.WToken != "synthetic-token" {
				t.Fatalf("auth token = %q", auth.WToken)
			}
			return map[string]any{
				"sections": []any{map[string]any{
					"items": []any{
						globalItemCommandTestEntry("item-a", "venue-a", "First semantic match"),
						globalItemCommandTestEntry("item-b", "venue-a", "Second semantic match"),
					},
				}},
			}, nil
		},
	}
	deps := Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:     "default",
			Location: domain.Location{Lat: 10.25, Lon: 20.5},
			WToken:   "synthetic-token",
		}},
	}
	cmd := newItemsCommand(deps)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--query", " semantic query ", "--limit", "7", "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("items command error = %v\n%s", err, out.String())
	}
	if !called {
		t.Fatal("global item search was not called")
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data := asMap(envelope["data"])
	if got := len(asSlice(data["items"])); got != 2 {
		t.Fatalf("machine item count = %d, want 2; data = %#v", got, data)
	}
	if got := len(asSlice(data["venue_groups"])); got != 1 {
		t.Fatalf("venue group count = %d, want 1", got)
	}
}

func TestItemsCommandRejectsInvalidLimitBeforeLocationResolution(t *testing.T) {
	cmd := newItemsCommand(Dependencies{Wolt: &testWoltAPI{}})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--query", "query", "--limit", "201"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("invalid limit did not return an error")
	}
}

func TestGlobalItemSearchTableCapsOnlyPresentationRowsPerVenue(t *testing.T) {
	items := make([]map[string]any, globalItemTableRowsPerVenue+1)
	ranks := make([]int, len(items))
	for index := range items {
		rank := index + 1
		ranks[index] = rank
		items[index] = map[string]any{
			"global_rank": rank,
			"name":        fmt.Sprintf("Ranked item %d", rank),
			"venue_id":    "venue-a",
			"venue_name":  "Synthetic Venue",
		}
	}
	table := buildGlobalItemSearchTable(map[string]any{
		"query":          "query",
		"returned_count": len(items),
		"items":          items,
		"venue_groups": []map[string]any{
			{"venue_id": "venue-a", "venue_name": "Synthetic Venue", "item_ranks": ranks},
		},
	})
	for rank := 1; rank <= globalItemTableRowsPerVenue; rank++ {
		if !strings.Contains(table, fmt.Sprintf("Ranked item %d", rank)) {
			t.Fatalf("table omitted presentation row %d:\n%s", rank, table)
		}
	}
	hiddenName := fmt.Sprintf("Ranked item %d", globalItemTableRowsPerVenue+1)
	if strings.Contains(table, hiddenName) {
		t.Fatalf("table exceeded per-venue presentation cap:\n%s", table)
	}
}

func globalItemCommandTestEntry(itemID, venueID, name string) map[string]any {
	return map[string]any{
		"menu_item": map[string]any{
			"id":           itemID,
			"venue_id":     venueID,
			"venue_name":   "Synthetic Venue",
			"name":         name,
			"price":        875,
			"currency":     "EUR",
			"is_available": true,
		},
	}
}
