package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestCartShowOptionalVenueFilterAcceptsSlugIDAndURL(t *testing.T) {
	const otherVenueID = "000000000000000000000002"
	wolt := &stubWolt{
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			return scheduledVenueStaticPayload(), nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"id":         "basket-primary",
					"venue_id":   scheduledVenueID,
					"venue_slug": scheduledVenueSlug,
					"items":      []any{map[string]any{"id": "item-1"}},
				},
				map[string]any{
					"id":    "basket-other",
					"venue": map[string]any{"id": otherVenueID, "slug": "other-market"},
					"items": []any{map[string]any{"id": "item-2"}},
				},
			}}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:     "default",
			WToken:   "token",
			Location: domain.Location{Lat: 10.25, Lon: 20.5},
		}},
	})

	for _, input := range []string{
		scheduledVenueSlug,
		scheduledVenueID,
		"https://wolt.com/en/test/venue/" + scheduledVenueSlug,
	} {
		t.Run(input, func(t *testing.T) {
			_, out, err := tc.handleCartShow(context.Background(), nil, CartShowInput{Venue: input})
			if err != nil {
				t.Fatalf("handleCartShow: %v", err)
			}
			rows := asSlice(out.Data["baskets"])
			if len(rows) != 1 || asString(asMap(rows[0])["id"]) != "basket-primary" {
				t.Fatalf("filtered baskets = %#v", rows)
			}
			if out.Filter["venue_id"] != scheduledVenueID || out.Filter["slug"] != scheduledVenueSlug {
				t.Fatalf("filter = %#v", out.Filter)
			}
		})
	}

	_, all, err := tc.handleCartShow(context.Background(), nil, CartShowInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(asSlice(all.Data["baskets"])); got != 2 {
		t.Fatalf("unfiltered baskets = %d, want 2", got)
	}
}

func TestBasketSelectionRejectsConflictingCanonicalVenueID(t *testing.T) {
	const conflictingVenueID = "000000000000000000000099"
	ref := venueRef{
		Input: scheduledVenueSlug,
		ID:    scheduledVenueID,
		Slug:  scheduledVenueSlug,
	}
	page := map[string]any{"baskets": []any{
		map[string]any{
			"id":         "basket-conflict",
			"venue_id":   conflictingVenueID,
			"venue_slug": scheduledVenueSlug,
		},
	}}

	if basket, err := selectVerifiedBasketForVenue(page, ref); err == nil || basket != nil ||
		!strings.Contains(err.Error(), "conflicts with the resolved venue id") {
		t.Fatalf("selection = %#v, error = %v; want canonical venue conflict", basket, err)
	}
	if filtered, err := filterBasketPage(page, ref, scheduledVenueSlug); err == nil || filtered != nil ||
		!strings.Contains(err.Error(), "conflicts with the resolved venue id") {
		t.Fatalf("filter = %#v, error = %v; want canonical venue conflict", filtered, err)
	}
}

func TestCartAddMarksScheduledOnlyVenue(t *testing.T) {
	const itemID = "000000000000000000000101"
	dynamicAuthCalls := []bool{}
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return scheduledVenueStaticPayload(), nil
		},
		venueDynamicFn: func(_ context.Context, gotSlug string, opts woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			if gotSlug != scheduledVenueSlug || opts.Location == nil {
				t.Fatalf("dynamic request = slug %q, opts %#v", gotSlug, opts)
			}
			dynamicAuthCalls = append(dynamicAuthCalls, opts.Auth.HasCredentials())
			if opts.Auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			return scheduledVenueDynamicPayload(), nil
		},
		assortmentItemsFn: func(context.Context, string, []string, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"items": []any{
				map[string]any{
					"id":                  itemID,
					"name":                "Frozen fish fillet",
					"price":               2595,
					"purchasable_balance": 8,
				},
			}}, nil
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"id": "basket-1", "venue_id": scheduledVenueID}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	_, out, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		LocationInput: LocationInput{Lat: 10.25, Lon: 20.5},
		Venue:         scheduledVenueID,
		ItemID:        itemID,
	})
	if err != nil {
		t.Fatalf("handleCartAdd: %v", err)
	}
	if out.OrderAvailability["order_now_available"] != false ||
		out.OrderAvailability["scheduled_order_available"] != true ||
		out.OrderAvailability["scheduled_only"] != true {
		t.Fatalf("order availability = %#v", out.OrderAvailability)
	}
	if !strings.Contains(out.Summary, "scheduled order only") {
		t.Fatalf("summary = %q", out.Summary)
	}
	if len(dynamicAuthCalls) != 2 || !dynamicAuthCalls[0] || dynamicAuthCalls[1] {
		t.Fatalf("dynamic authenticated calls = %v, want [true false]", dynamicAuthCalls)
	}
}

func TestCartAddFailsClosedWhenBasketSnapshotFails(t *testing.T) {
	const itemID = "000000000000000000000101"
	addCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return scheduledVenueStaticPayload(), nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return nil, &woltgateway.UpstreamRequestError{StatusCode: 429}
		},
		addToBasketFn: func(context.Context, map[string]any, woltgateway.AuthContext) (map[string]any, error) {
			addCalled = true
			return map[string]any{}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueID,
		ItemID:        itemID,
		Price:         2595,
		Currency:      scheduledVenueCurrency,
		Name:          "Synthetic item",
	})
	if err == nil {
		t.Fatal("handleCartAdd succeeded after the basket snapshot failed")
	}
	var classified *classifiedToolError
	if !errors.As(err, &classified) || classified.info.Code != "RATE_LIMITED" {
		t.Fatalf("error = %#v, want classified RATE_LIMITED", err)
	}
	if addCalled {
		t.Fatal("AddToBasket must not be called without a complete basket snapshot")
	}
}

func TestCartAddMergesBasketWithTopLevelVenueIdentity(t *testing.T) {
	const (
		existingItemID = "000000000000000000000101"
		addedItemID    = "000000000000000000000102"
	)
	var captured map[string]any
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return scheduledVenueStaticPayload(), nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"id":         "basket-1",
					"venue_id":   scheduledVenueID,
					"venue_slug": scheduledVenueSlug,
					"items": []any{
						map[string]any{
							"id":      existingItemID,
							"count":   2,
							"name":    "Existing item",
							"price":   500,
							"options": []any{},
						},
					},
				},
			}}, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			captured = payload
			return map[string]any{"id": "basket-1"}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	_, _, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueID,
		ItemID:        addedItemID,
		Price:         700,
		Currency:      scheduledVenueCurrency,
		Name:          "Added item",
	})
	if err != nil {
		t.Fatalf("handleCartAdd: %v", err)
	}
	items := asSlice(captured["items"])
	if len(items) != 2 {
		t.Fatalf("posted items = %#v, want existing and added lines", items)
	}
	if got := asString(asMap(items[0])["id"]); got != existingItemID {
		t.Fatalf("first posted item = %q, want %q", got, existingItemID)
	}
	if got := asString(asMap(items[1])["id"]); got != addedItemID {
		t.Fatalf("second posted item = %q, want %q", got, addedItemID)
	}
}

func TestCartAddUsesSelectedBasketIDWhenResolverIsUnavailable(t *testing.T) {
	const (
		itemID = "000000000000000000000111"
	)
	var captured map[string]any
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static unavailable")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("dynamic unavailable")
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"id":         "basket-1",
					"venue_id":   scheduledVenueID,
					"venue_slug": scheduledVenueSlug,
					"currency":   scheduledVenueCurrency,
					"items":      []any{},
				},
			}}, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			captured = payload
			return map[string]any{"id": "basket-1"}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	_, out, err := tc.handleCartAdd(context.Background(), nil, CartAddInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueSlug,
		ItemID:        itemID,
		Price:         700,
		Currency:      scheduledVenueCurrency,
		Name:          "Added item",
	})
	if err != nil {
		t.Fatalf("handleCartAdd: %v", err)
	}
	if got := asString(captured["venue_id"]); got != scheduledVenueID {
		t.Fatalf("posted venue_id = %q, want %q", got, scheduledVenueID)
	}
	if out.VenueID != scheduledVenueID {
		t.Fatalf("output venue_id = %q, want %q", out.VenueID, scheduledVenueID)
	}
}

func TestCartRemoveClearsFinalLineWithoutCurrencyOrVenueID(t *testing.T) {
	const itemID = "000000000000000000000121"
	deleted := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"slug": scheduledVenueSlug}}, nil
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"basket_id":  "basket-1",
					"venue_slug": scheduledVenueSlug,
					"items": []any{
						map[string]any{"id": itemID, "count": 1, "price": 500},
					},
				},
			}}, nil
		},
		deleteBasketsFn: func(_ context.Context, ids []string, _ woltgateway.AuthContext) (map[string]any, error) {
			deleted = len(ids) == 1 && ids[0] == "basket-1"
			return map[string]any{}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	_, out, err := tc.handleCartRemove(context.Background(), nil, CartRemoveInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueSlug,
		ItemID:        itemID,
	})
	if err != nil {
		t.Fatalf("handleCartRemove: %v", err)
	}
	if !deleted || out.Removed != 1 {
		t.Fatalf("delete result = deleted:%v output:%#v", deleted, out)
	}
}

func TestCartRemoveUsesSelectedBasketIDForReplacement(t *testing.T) {
	const (
		itemID  = "000000000000000000000131"
		otherID = "000000000000000000000132"
	)
	var captured map[string]any
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static unavailable")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("dynamic unavailable")
		},
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"id":         "basket-1",
					"venue_id":   scheduledVenueID,
					"venue_slug": scheduledVenueSlug,
					"currency":   scheduledVenueCurrency,
					"items": []any{
						map[string]any{"id": itemID, "count": 2, "price": 500},
						map[string]any{"id": otherID, "count": 1, "price": 700},
					},
				},
			}}, nil
		},
		addToBasketFn: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
			captured = payload
			return map[string]any{}, nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	if _, _, err := tc.handleCartRemove(context.Background(), nil, CartRemoveInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueSlug,
		ItemID:        itemID,
		Count:         1,
	}); err != nil {
		t.Fatalf("handleCartRemove: %v", err)
	}
	if got := asString(captured["venue_id"]); got != scheduledVenueID {
		t.Fatalf("posted venue_id = %q, want %q", got, scheduledVenueID)
	}
	if got := len(asSlice(captured["items"])); got != 2 {
		t.Fatalf("posted items = %#v", captured["items"])
	}
}

func TestCartVenueAvailabilityDoesNotRetryRateLimitAnonymously(t *testing.T) {
	dynamicAuthCalls := []bool{}
	tc := newToolCtx(Deps{
		Wolt: &stubWolt{
			venueDynamicFn: func(
				_ context.Context,
				gotSlug string,
				options woltgateway.VenuePageDynamicOptions,
			) (map[string]any, error) {
				if gotSlug != scheduledVenueSlug || options.Location == nil {
					t.Fatalf("dynamic request = slug %q, options %#v", gotSlug, options)
				}
				dynamicAuthCalls = append(dynamicAuthCalls, options.Auth.HasCredentials())
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 429}
			},
		},
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "token",
		}},
	})

	availability, warnings := tc.cartVenueAvailability(
		context.Background(),
		venueRef{ID: scheduledVenueID, Slug: scheduledVenueSlug},
		domain.Location{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
	)
	if availability != nil {
		t.Fatalf("availability = %#v, want nil", availability)
	}
	if len(dynamicAuthCalls) != 1 || !dynamicAuthCalls[0] {
		t.Fatalf("dynamic authenticated calls = %v, want [true]", dynamicAuthCalls)
	}
	if !containsWarning(warnings, "venue order availability could not be loaded") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestCartShowMarksExistingBasketScheduledOnly(t *testing.T) {
	wolt := &stubWolt{
		basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
			return map[string]any{"baskets": []any{
				map[string]any{
					"id":    "basket-1",
					"venue": map[string]any{"id": scheduledVenueID, "slug": scheduledVenueSlug},
				},
			}}, nil
		},
		venueDynamicFn: func(_ context.Context, gotSlug string, opts woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			if gotSlug != scheduledVenueSlug || opts.Location == nil {
				t.Fatalf("dynamic request = slug %q, opts %#v", gotSlug, opts)
			}
			return scheduledVenueDynamicPayload(), nil
		},
	}
	tc := newToolCtx(Deps{
		Wolt: wolt,
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:     "default",
			WToken:   "token",
			Location: domain.Location{Lat: 10.25, Lon: 20.5},
		}},
	})

	_, out, err := tc.handleCartShow(context.Background(), nil, CartShowInput{})
	if err != nil {
		t.Fatalf("handleCartShow: %v", err)
	}
	baskets := asSlice(out.Data["baskets"])
	availability := asMap(asMap(baskets[0])["order_availability"])
	if availability["order_now_available"] != false ||
		availability["scheduled_order_available"] != true ||
		availability["scheduled_only"] != true {
		t.Fatalf("order availability = %#v", availability)
	}
}
