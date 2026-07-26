package cli

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// normalizeTableWhitespace collapses runs of spaces into single spaces
// so test assertions can target the cell content without depending on
// tabwriter's column padding (which varies with the widest row).
var multiSpaceTable = regexp.MustCompile(`[ \t]+`)

func normalizeTableWhitespace(rendered string) string {
	return multiSpaceTable.ReplaceAllString(rendered, " ")
}

func TestCurrencyAndAmountHelpers(t *testing.T) {
	if got := inferCurrency("€12.34"); got != "EUR" {
		t.Fatalf("expected EUR, got %q", got)
	}
	if got := inferCurrency("$12.34"); got != "USD" {
		t.Fatalf("expected USD, got %q", got)
	}
	if got := formatMinorAmount(1595, "EUR"); got != "€15.95" {
		t.Fatalf("expected €15.95, got %q", got)
	}
	if got := formatMinorAmount(0, "EUR"); got != "€0.00" {
		t.Fatalf("expected €0.00, got %q", got)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"a", "b", "a", "", "c", "b"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("unexpected deduped values: %v", got)
	}
}

func TestSelectBasketWithMeta(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{"id": "basket-1", "venue": map[string]any{"id": "venue-1", "name": "A", "slug": "venue-a"}},
			map[string]any{"id": "basket-2", "venue": map[string]any{"id": "venue-2", "name": "B"}},
		},
	}

	selected, meta, warnings := selectBasketWithMeta(page, "")
	if asString(selected["id"]) != "basket-1" {
		t.Fatalf("expected first basket to be selected, got %v", selected["id"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning when multiple baskets exist")
	}
	if asString(meta["selection_mode"]) != "first-available" {
		t.Fatalf("expected first-available selection mode, got %v", meta["selection_mode"])
	}

	selectedBySlug, metaBySlug, warningsBySlug := selectBasketWithMeta(page, "venue-a")
	if asString(selectedBySlug["id"]) != "basket-1" {
		t.Fatalf("expected basket-1 selected by slug, got %v", selectedBySlug["id"])
	}
	if len(warningsBySlug) != 0 {
		t.Fatalf("expected no warnings for explicit slug selection, got %v", warningsBySlug)
	}
	if asString(metaBySlug["selection_mode"]) != "requested-venue-slug" {
		t.Fatalf("expected requested-venue-slug selection mode, got %v", metaBySlug["selection_mode"])
	}

	topLevelPage := map[string]any{
		"results": []any{
			map[string]any{
				"basket_id":  "basket-3",
				"venue_id":   "venue-3",
				"venue_slug": "venue-c",
			},
		},
	}
	selectedTopLevel, topLevelMeta, topLevelWarnings := selectBasketWithMeta(topLevelPage, "venue-c")
	if asString(selectedTopLevel["basket_id"]) != "basket-3" ||
		asString(topLevelMeta["selection_mode"]) != "requested-venue-slug" ||
		len(topLevelWarnings) != 0 {
		t.Fatalf(
			"top-level results selection = (%#v, %#v, %v)",
			selectedTopLevel,
			topLevelMeta,
			topLevelWarnings,
		)
	}
}

func TestBuildCartStateAndLineDetails(t *testing.T) {
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":    "basket-1",
				"total": "€18.00",
				"venue": map[string]any{"id": "venue-1", "name": "Venue 1", "slug": "venue-1"},
				"telemetry": map[string]any{
					"basket_total": 1800,
				},
				"items": []any{
					map[string]any{
						"id":    "item-1",
						"name":  "Combo",
						"count": 1,
						"price": 1700,
						"options": []any{
							map[string]any{
								"id":   "drink",
								"name": "Drink",
								"values": []any{
									map[string]any{"id": "cola", "name": "Cola", "count": 2, "price": 50},
								},
							},
						},
					},
				},
			},
		},
	}

	data, warnings := buildCartState(page, "")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if asString(data["basket_id"]) != "basket-1" {
		t.Fatalf("expected basket-1, got %v", data["basket_id"])
	}
	if asInt(asMap(data["total"])["amount"]) != 1800 {
		t.Fatalf("expected total amount 1800, got %v", asMap(data["total"])["amount"])
	}

	lines := asSlice(data["lines"])
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	details := cartLineDetails(asMap(lines[0]), "EUR")
	if len(details) != 1 || !strings.Contains(details[0], "Drink: Cola x2 (+€0.50)") {
		t.Fatalf("unexpected line details: %v", details)
	}
}

func runTestCartAdd(t *testing.T, api *testWoltAPI, args ...string) (string, error) {
	t.Helper()
	cmd := newCartAddCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 10, Lon: 20},
			WToken:    "test-token",
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func TestCartAddFailsClosedWhenBasketSnapshotFails(t *testing.T) {
	const (
		venueID = "000000000000000000000011"
		itemID  = "000000000000000000000012"
	)
	addCalls := 0
	basketCalls := 0
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": "example-market"},
			}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			basketCalls++
			return nil, errors.New("basket snapshot unavailable")
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			addCalls++
			return map[string]any{}, nil
		},
	}
	_, err := runTestCartAdd(t, api,
		venueID,
		itemID,
		"--venue-slug", "example-market",
		"--price", "500",
		"--currency", "EUR",
		"--format", "json",
	)
	if err == nil {
		t.Fatal("cart add succeeded without an existing basket snapshot")
	}
	if basketCalls != 1 {
		t.Fatalf("BasketsPage called %d times, want 1", basketCalls)
	}
	if addCalls != 0 {
		t.Fatalf("AddToBasket called %d times, want 0", addCalls)
	}
}

func TestCartAddSelectsMatchingBasketWhenVenueResolutionFails(t *testing.T) {
	const (
		requestedVenueID = "000000000000000000000041"
		otherVenueID     = "000000000000000000000042"
		itemID           = "000000000000000000000043"
		requestedLineID  = "000000000000000000000044"
		otherLineID      = "000000000000000000000045"
	)
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":         "other-basket",
				"venue_id":   otherVenueID,
				"venue_slug": "other-market",
				"currency":   "EUR",
				"items": []any{
					map[string]any{"id": otherLineID, "count": 1, "name": "Other", "price": 700},
				},
			},
			map[string]any{
				"id":         "requested-basket",
				"venue_id":   requestedVenueID,
				"venue_slug": "requested-market",
				"currency":   "EUR",
				"items": []any{
					map[string]any{"id": requestedLineID, "count": 1, "name": "Existing", "price": 600},
				},
			},
		},
	}
	var mutation map[string]any
	itemPageCalls := 0
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static venue resolution unavailable")
		},
		venuePageDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("dynamic venue resolution unavailable")
		},
		venueItemPageFn: func(_ context.Context, venueID, requestedItemID string) (map[string]any, error) {
			itemPageCalls++
			if venueID != requestedVenueID || requestedItemID != itemID {
				t.Fatalf("VenueItemPage(%q, %q)", venueID, requestedItemID)
			}
			return map[string]any{"id": itemID, "name": "New", "price": 500}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return page, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			mutation = payload
			return map[string]any{"id": "requested-basket", "venue_id": requestedVenueID}, nil
		},
	}
	output, err := runTestCartAdd(t, api,
		"requested-market",
		itemID,
		"--price", "500",
		"--currency", "EUR",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("cart add: %v\n%s", err, output)
	}
	if asString(mutation["venue_id"]) != requestedVenueID {
		t.Fatalf("venue_id = %v, want %s", mutation["venue_id"], requestedVenueID)
	}
	if itemPageCalls != 1 {
		t.Fatalf("VenueItemPage called %d times, want 1 with the basket venue id", itemPageCalls)
	}
	items := asSlice(mutation["items"])
	if len(items) != 2 ||
		asString(asMap(items[0])["id"]) != requestedLineID ||
		asString(asMap(items[1])["id"]) != itemID {
		t.Fatalf("mutation merged the wrong basket: %#v", mutation)
	}
	for _, value := range items {
		if asString(asMap(value)["id"]) == otherLineID {
			t.Fatalf("mutation contains a line from another venue: %#v", mutation)
		}
	}
}

func TestCartAddRejectsConflictingOrUnverifiedVenueHints(t *testing.T) {
	const (
		requestedVenueID = "000000000000000000000051"
		otherVenueID     = "000000000000000000000052"
		itemID           = "000000000000000000000053"
	)
	resolveOtherMarket := func(reference string) (map[string]any, error) {
		if reference == "other-market" {
			return map[string]any{
				"venue": map[string]any{"id": otherVenueID, "slug": reference},
			}, nil
		}
		return nil, errors.New("requested venue lookup unavailable")
	}
	tests := []struct {
		name        string
		venue       string
		item        string
		override    string
		wantMessage string
		staticVenue func(string) (map[string]any, error)
	}{
		{
			name:        "venue slug flag conflicts with positional slug",
			venue:       "requested-market",
			item:        itemID,
			override:    "other-market",
			wantMessage: "conflicts with venue",
		},
		{
			name:        "venue slug flag resolves to another id",
			venue:       requestedVenueID,
			item:        itemID,
			override:    "other-market",
			wantMessage: "different venue",
			staticVenue: resolveOtherMarket,
		},
		{
			name:        "item URL resolves to another id",
			venue:       requestedVenueID,
			item:        "https://wolt.com/en/test/city/venue/other-market/itemid-" + itemID,
			wantMessage: "different venue",
			staticVenue: resolveOtherMarket,
		},
		{
			name:        "venue slug association cannot be verified",
			venue:       requestedVenueID,
			item:        itemID,
			override:    "unverified-market",
			wantMessage: "Could not verify",
		},
		{
			name:        "empty positional venue cannot select the first basket",
			venue:       " ",
			item:        itemID,
			override:    "requested-market",
			wantMessage: "venue is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addCalls := 0
			api := &testWoltAPI{
				venuePageStaticFn: func(_ context.Context, reference string) (map[string]any, error) {
					if test.staticVenue != nil {
						return test.staticVenue(reference)
					}
					return nil, errors.New("static venue resolution unavailable")
				},
				venuePageDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
					return nil, errors.New("dynamic venue resolution unavailable")
				},
				addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
					addCalls++
					return map[string]any{}, nil
				},
			}
			args := []string{
				test.venue,
				test.item,
				"--price", "500",
				"--currency", "EUR",
				"--format", "json",
			}
			if test.override != "" {
				args = append(args, "--venue-slug", test.override)
			}
			output, err := runTestCartAdd(t, api, args...)
			if err == nil {
				t.Fatalf("cart add accepted conflicting venue identities\n%s", output)
			}
			if !strings.Contains(output, test.wantMessage) {
				t.Fatalf("unexpected conflict error:\n%s", output)
			}
			if addCalls != 0 {
				t.Fatalf("AddToBasket called %d times, want 0", addCalls)
			}
		})
	}
}

func TestCartRemovePreservesOtherLinesInReplacementPayload(t *testing.T) {
	const (
		venueID = "000000000000000000000021"
		itemID  = "000000000000000000000022"
		otherID = "000000000000000000000023"
	)
	page := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":         "basket-1",
				"venue_id":   venueID,
				"venue_slug": "example-market",
				"currency":   "EUR",
				"items": []any{
					map[string]any{"id": itemID, "count": 2, "name": "A", "price": 500},
					map[string]any{"id": otherID, "count": 1, "name": "B", "price": 700},
				},
			},
		},
	}
	var mutation map[string]any
	api := &testWoltAPI{
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return page, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			mutation = payload
			return map[string]any{}, nil
		},
	}
	cmd := newCartRemoveCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 10, Lon: 20},
			WToken:    "test-token",
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{itemID, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("cart remove: %v\n%s", err, output.String())
	}
	items := asSlice(mutation["items"])
	if len(items) != 2 || asInt(asMap(items[0])["count"]) != 1 ||
		asString(asMap(items[1])["id"]) != otherID {
		t.Fatalf("replacement payload lost or miscounted lines: %#v", mutation)
	}
	if asString(mutation["venue_id"]) != venueID {
		t.Fatalf("venue_id = %v, want %s", mutation["venue_id"], venueID)
	}
}

func TestCartRemoveClearsSingleLineWithoutVenueIdentityOrCurrency(t *testing.T) {
	const itemID = "000000000000000000000032"
	deleteCalls := 0
	api := &testWoltAPI{
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{
				"baskets": []any{
					map[string]any{
						"basket_id": "basket-2",
						"items": []any{
							map[string]any{"id": itemID, "count": 1, "name": "A", "price": 500},
						},
					},
				},
			}, nil
		},
		deleteBasketsFn: func(_ context.Context, basketIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
			deleteCalls++
			if len(basketIDs) != 1 || basketIDs[0] != "basket-2" {
				t.Fatalf("DeleteBaskets ids = %v", basketIDs)
			}
			return map[string]any{}, nil
		},
	}
	cmd := newCartRemoveCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 10, Lon: 20},
			WToken:    "test-token",
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{itemID, "--all", "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("cart remove --all: %v\n%s", err, output.String())
	}
	if deleteCalls != 1 {
		t.Fatalf("DeleteBaskets called %d times, want 1", deleteCalls)
	}
}

func TestBuildItemDetailTableFormatsGroups(t *testing.T) {
	data := map[string]any{
		"name":        "Combo",
		"item_id":     "item-1",
		"venue_id":    "venue-1",
		"description": "Test item",
		"price": map[string]any{
			"formatted_amount": "€10.00",
		},
		"option_groups": []any{
			map[string]any{
				"group_id": "group-drink",
				"name":     "Drink",
				"required": true,
				"min":      1,
				"max":      1,
				"values": []any{
					map[string]any{"value_id": "cola", "name": "Cola"},
				},
			},
		},
		"upsell_items": []any{},
	}

	rendered := normalizeTableWhitespace(buildItemDetailTable(data))
	for _, expected := range []string{
		"Option groups 1",
		"Upsell items 0",
		"Option groups\nGroup ID Name Required Min Max Values",
		"group-drink Drink yes 1 1 cola (Cola)",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestTokenPreviewAndExpiryFormatting(t *testing.T) {
	if got := tokenPreview("abcdefghijklmnop"); got != "abcdef...klmnop" {
		t.Fatalf("unexpected token preview: %q", got)
	}
	if got := tokenPreview("short"); got != "short" {
		t.Fatalf("unexpected short token preview: %q", got)
	}
	if got := tokenExpiryRFC3339("bad-token"); got != "" {
		t.Fatalf("expected empty expiry for invalid token, got %q", got)
	}
}
