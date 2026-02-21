package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestAuthStatusJSONWithToken(t *testing.T) {
	seenToken := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				seenToken = auth.WToken
				return map[string]any{
					"user": map[string]any{
						"_id":                     map[string]any{"$oid": "user-1"},
						"country":                 "FIN",
						"is_wolt_plus_subscriber": true,
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "auth", "status", "--wtoken", "abc.def.ghi", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "abc.def.ghi" {
		t.Fatalf("expected token abc.def.ghi, got %q", seenToken)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if !asBoolPayload(data["authenticated"]) {
		t.Fatalf("expected authenticated=true, got %v", data["authenticated"])
	}
	if data["user_id"] != "user-1" {
		t.Fatalf("expected user_id user-1, got %v", data["user_id"])
	}
	if !asBoolPayload(data["wolt_plus_subscriber"]) {
		t.Fatalf("expected wolt_plus_subscriber=true, got %v", data["wolt_plus_subscriber"])
	}
}

func TestProfileStatusJSONWithToken(t *testing.T) {
	seenToken := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				seenToken = auth.WToken
				return map[string]any{
					"user": map[string]any{
						"_id":     map[string]any{"$oid": "user-1"},
						"country": "FIN",
						"wolt_plus": map[string]any{
							"status": "active",
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "profile", "status", "--wtoken", "abc.def.ghi", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "abc.def.ghi" {
		t.Fatalf("expected token abc.def.ghi, got %q", seenToken)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if !asBoolPayload(data["authenticated"]) {
		t.Fatalf("expected authenticated=true, got %v", data["authenticated"])
	}
	if data["user_id"] != "user-1" {
		t.Fatalf("expected user_id user-1, got %v", data["user_id"])
	}
	if !asBoolPayload(data["wolt_plus_subscriber"]) {
		t.Fatalf("expected wolt_plus_subscriber=true, got %v", data["wolt_plus_subscriber"])
	}
}

func TestAuthStatusJSONWithChromeEncodedTokenPayload(t *testing.T) {
	seenToken := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				seenToken = auth.WToken
				return map[string]any{
					"user": map[string]any{
						"_id":     map[string]any{"$oid": "user-1"},
						"country": "FIN",
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	chromePayload := `{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`
	exitCode, out := runCLIWithDeps(t, deps, "auth", "status", "--wtoken", chromePayload, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "abc.def.ghi" {
		t.Fatalf("expected normalized token abc.def.ghi, got %q", seenToken)
	}
}

func TestAuthStatusUsesProfileTokenWhenFlagMissing(t *testing.T) {
	seenToken := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				seenToken = auth.WToken
				return map[string]any{
					"user": map[string]any{
						"_id":     map[string]any{"$oid": "user-1"},
						"country": "FIN",
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 60.1, Lon: 24.9},
			WToken:    `{%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}`,
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "auth", "status", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "abc.def.ghi" {
		t.Fatalf("expected profile token abc.def.ghi, got %q", seenToken)
	}
}

func TestAuthStatusUsesProfileCookieTokenWhenFlagMissing(t *testing.T) {
	seenToken := ""
	seenCookies := []string{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				seenToken = auth.WToken
				seenCookies = append(seenCookies, auth.Cookies...)
				return map[string]any{
					"user": map[string]any{
						"_id":     map[string]any{"$oid": "user-1"},
						"country": "FIN",
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 60.1, Lon: 24.9},
			Cookies: []string{
				"foo=bar; __wtoken={%22accessToken%22:%22abc.def.ghi%22%2C%22expirationTime%22:1771540095000}",
			},
		}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "auth", "status", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenToken != "abc.def.ghi" {
		t.Fatalf("expected token extracted from profile cookie abc.def.ghi, got %q", seenToken)
	}
	if len(seenCookies) != 1 {
		t.Fatalf("expected profile cookies to be forwarded, got %v", seenCookies)
	}
}

func TestAuthStatusAutoRefreshesExpiredTokenAndPersistsProfile(t *testing.T) {
	cfg := &recordingConfig{
		loadCfg: domain.Config{
			Profiles: []domain.Profile{
				{
					Name:          "default",
					IsDefault:     true,
					Location:      domain.Location{Lat: 60.1, Lon: 24.9},
					WToken:        "expired-token",
					WRefreshToken: "refresh-old",
				},
			},
		},
	}
	userMeCalls := 0
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			userMeFunc: func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
				userMeCalls++
				if auth.WToken != "rotated-token" {
					return nil, &woltgateway.UpstreamRequestError{StatusCode: 401, Method: "GET", URL: "https://restaurant-api.wolt.com/v1/user/me"}
				}
				return map[string]any{
					"user": map[string]any{
						"_id":     map[string]any{"$oid": "user-1"},
						"country": "FIN",
					},
				}, nil
			},
			refreshAccessTokenFn: func(_ context.Context, refreshToken string, _ woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
				if refreshToken != "refresh-old" {
					t.Fatalf("expected refresh token refresh-old, got %q", refreshToken)
				}
				return woltgateway.TokenRefreshResult{
					AccessToken:  "rotated-token",
					RefreshToken: "refresh-new",
					ExpiresIn:    1800,
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{
			Name:          "default",
			IsDefault:     true,
			Location:      domain.Location{Lat: 60.1, Lon: 24.9},
			WToken:        "expired-token",
			WRefreshToken: "refresh-old",
		}},
		Location: &mockLocation{},
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "auth", "status", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if userMeCalls != 2 {
		t.Fatalf("expected two user/me attempts (before and after refresh), got %d", userMeCalls)
	}
	if cfg.saved == nil {
		t.Fatal("expected rotated tokens to be persisted in config")
	}
	savedProfile := cfg.saved.Profiles[0]
	if savedProfile.WToken != "rotated-token" {
		t.Fatalf("expected persisted wtoken rotated-token, got %q", savedProfile.WToken)
	}
	if savedProfile.WRefreshToken != "refresh-new" {
		t.Fatalf("expected persisted wrefresh_token refresh-new, got %q", savedProfile.WRefreshToken)
	}

	payload := mustJSON(t, out)
	warnings := asSlicePayload(t, payload["warnings"])
	if len(warnings) == 0 {
		t.Fatalf("expected warning about automatic token refresh, got none")
	}
}

func TestCartShowJSON(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(_ context.Context, _ domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
				if auth.WToken == "" {
					t.Fatalf("expected auth token in basketsPage call")
				}
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1"},
							"items": []any{
								map[string]any{"id": "line-1", "name": "Classics set", "count": 1, "price": 1700, "options": []any{}},
							},
							"telemetry": map[string]any{"basket_total": 1700},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "cart", "show", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["basket_id"] != "basket-1" {
		t.Fatalf("expected basket_id basket-1, got %v", data["basket_id"])
	}
	if asIntPayload(asMapPayload(t, data["total"])["amount"]) != 1700 {
		t.Fatalf("expected total amount 1700, got %v", asMapPayload(t, data["total"])["amount"])
	}
}

func TestCartShowTableWithDetails(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(_ context.Context, _ domain.Location, auth woltgateway.AuthContext) (map[string]any, error) {
				if auth.WToken == "" {
					t.Fatalf("expected auth token in basketsPage call")
				}
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€19.50",
							"venue": map[string]any{"id": "venue-1", "name": "Burger Place", "slug": "burger-place"},
							"items": []any{
								map[string]any{
									"id":    "line-1",
									"name":  "Klassikkojen setti",
									"count": 1,
									"price": 1700,
									"options": []any{
										map[string]any{
											"id":   "drink",
											"name": "Drink",
											"values": []any{
												map[string]any{"id": "cola", "name": "Coke", "count": 1, "price": 200},
											},
										},
										map[string]any{
											"id":   "sauce",
											"name": "Sauce",
											"values": []any{
												map[string]any{"id": "bbq", "name": "BBQ", "count": 2, "price": 0},
											},
										},
									},
								},
							},
							"telemetry": map[string]any{"basket_total": 1950},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "cart", "show", "--wtoken", "token", "--details")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	for _, expected := range []string{
		"Cart summary",
		"Cart items",
		"Klassikkojen setti",
		"Drink: Coke (+€2.00)",
		"Sauce: BBQ x2",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected output to contain %q\noutput:\n%s", expected, out)
		}
	}
}

func TestCartAddJSON(t *testing.T) {
	seenAddPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"name":  "Classics set",
					"price": map[string]any{"amount": 1700, "currency": "EUR"},
					"option_groups": []any{
						map[string]any{"id": "group-1"},
					},
				}, nil
			},
			addToBasketFunc: func(_ context.Context, payload map[string]any, auth woltgateway.AuthContext) (map[string]any, error) {
				seenAddPayload = payload
				if auth.WToken == "" {
					t.Fatalf("expected auth token in addToBasket")
				}
				return map[string]any{"id": "basket-1", "venue_id": "venue-1"}, nil
			},
			basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"count": 2}, nil
			},
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€34.00",
							"venue": map[string]any{"id": "venue-1"},
							"items": []any{
								map[string]any{"id": "line-1", "name": "Classics set", "count": 2, "price": 1700, "options": []any{}},
							},
							"telemetry": map[string]any{"basket_total": 3400},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"cart",
		"add",
		"venue-1",
		"item-1",
		"--count",
		"2",
		"--option",
		"group-1=value-1",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	items := asSlicePayload(t, seenAddPayload["items"])
	if len(items) != 2 {
		t.Fatalf("expected merged payload with existing + new item, got %d", len(items))
	}
	first := asMapPayload(t, items[0])
	if first["id"] != "line-1" || asIntPayload(first["count"]) != 2 {
		t.Fatalf("expected first payload line to preserve existing line-1 x2, got %+v", first)
	}
	second := asMapPayload(t, items[1])
	if second["id"] != "item-1" || asIntPayload(second["count"]) != 2 {
		t.Fatalf("expected second payload line item-1 x2, got %+v", second)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["mutation"] != "add" {
		t.Fatalf("expected add mutation, got %v", data["mutation"])
	}
	if asIntPayload(data["total_items"]) != 2 {
		t.Fatalf("expected total_items 2, got %v", data["total_items"])
	}
}

func TestCartAddUsesVenueSlugAssortmentFallback(t *testing.T) {
	seenAddPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return nil, errors.New("item endpoint unavailable")
			},
			restaurantByIDFunc: func(context.Context, string) (*domain.Restaurant, error) {
				return nil, errors.New("restaurant endpoint unavailable")
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"items": []any{
						map[string]any{
							"id":      "item-1",
							"name":    "Classics set",
							"price":   1700,
							"options": []any{map[string]any{"option_id": "group-drink"}},
						},
					},
					"options": []any{
						map[string]any{
							"id":   "group-drink",
							"name": "Drink",
							"values": []any{
								map[string]any{"id": "value-cola", "name": "Cola", "price": 0},
							},
						},
					},
				}, nil
			},
			addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenAddPayload = payload
				return map[string]any{"id": "basket-1", "venue_id": "venue-1"}, nil
			},
			basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"count": 1}, nil
			},
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1"},
							"items": []any{
								map[string]any{"id": "line-1", "name": "Classics set", "count": 1, "price": 1700, "options": []any{}},
							},
							"telemetry": map[string]any{"basket_total": 1700},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"cart",
		"add",
		"venue-1",
		"item-1",
		"--venue-slug",
		"venue-slug",
		"--option",
		"Drink=Cola",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	items := asSlicePayload(t, seenAddPayload["items"])
	if len(items) != 2 {
		t.Fatalf("expected merged payload with existing + new item, got %d", len(items))
	}
	first := asMapPayload(t, items[1])
	if first["name"] != "Classics set" {
		t.Fatalf("expected inferred name Classics set, got %v", first["name"])
	}
	if asIntPayload(first["price"]) != 1700 {
		t.Fatalf("expected inferred price 1700, got %v", first["price"])
	}
	options := asSlicePayload(t, first["options"])
	if len(options) != 1 {
		t.Fatalf("expected one option group, got %d", len(options))
	}
	group := asMapPayload(t, options[0])
	if group["id"] != "group-drink" {
		t.Fatalf("expected option group id group-drink, got %v", group["id"])
	}
	values := asSlicePayload(t, group["values"])
	if len(values) != 1 || asMapPayload(t, values[0])["id"] != "value-cola" {
		t.Fatalf("expected resolved option value value-cola, got %v", values)
	}
}

func TestCartAddMergesWhenVenueArgIsSlug(t *testing.T) {
	seenAddPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"name":  "New item",
					"price": map[string]any{"amount": 500, "currency": "EUR"},
				}, nil
			},
			addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenAddPayload = payload
				return map[string]any{"id": "basket-1", "venue_id": "venue-1"}, nil
			},
			basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"count": 2}, nil
			},
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€22.00",
							"venue": map[string]any{"id": "venue-1", "slug": "venue-one"},
							"items": []any{
								map[string]any{"id": "item-old", "name": "Old item", "count": 1, "price": 1700, "options": []any{}},
							},
							"telemetry": map[string]any{"basket_total": 2200},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"cart",
		"add",
		"venue-one",
		"item-new",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenAddPayload["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id resolved to venue-1, got %v", seenAddPayload["venue_id"])
	}
	items := asSlicePayload(t, seenAddPayload["items"])
	if len(items) != 2 {
		t.Fatalf("expected merged payload with 2 items, got %d", len(items))
	}
	if asMapPayload(t, items[0])["id"] != "item-old" {
		t.Fatalf("expected existing item first, got %v", asMapPayload(t, items[0])["id"])
	}
	if asMapPayload(t, items[1])["id"] != "item-new" {
		t.Fatalf("expected new item second, got %v", asMapPayload(t, items[1])["id"])
	}
}

func TestCartRemoveJSON(t *testing.T) {
	seenPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€34.00",
							"venue": map[string]any{"id": "venue-1"},
							"items": []any{
								map[string]any{
									"id":    "item-1",
									"name":  "Classics set",
									"count": 2,
									"price": 1700,
									"options": []any{
										map[string]any{
											"id": "group-1",
											"values": []any{
												map[string]any{"id": "value-1", "count": 1, "price": 0},
											},
										},
									},
									"substitution_settings": map[string]any{"is_allowed": true},
								},
							},
						},
					},
				}, nil
			},
			addToBasketFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenPayload = payload
				return map[string]any{"id": "basket-1", "venue_id": "venue-1"}, nil
			},
			basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"count": 1}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "cart", "remove", "item-1", "--count", "1", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}

	items := asSlicePayload(t, seenPayload["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item payload, got %d", len(items))
	}
	if asIntPayload(asMapPayload(t, items[0])["count"]) != 1 {
		t.Fatalf("expected remaining item count 1, got %v", asMapPayload(t, items[0])["count"])
	}

	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["mutation"] != "remove" {
		t.Fatalf("expected remove mutation, got %v", data["mutation"])
	}
	if asIntPayload(data["removed_count"]) != 1 {
		t.Fatalf("expected removed_count=1, got %v", data["removed_count"])
	}
}

func TestCartRemoveAllSingleItemFallsBackToClear(t *testing.T) {
	seenBasketIDs := []string{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1"},
							"items": []any{
								map[string]any{
									"id":      "item-1",
									"name":    "Classics set",
									"count":   1,
									"price":   1700,
									"options": []any{},
								},
							},
						},
					},
				}, nil
			},
			deleteBasketsFunc: func(_ context.Context, basketIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
				seenBasketIDs = append(seenBasketIDs, basketIDs...)
				return map[string]any{}, nil
			},
			basketCountFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{"count": 0}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "cart", "remove", "item-1", "--all", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if len(seenBasketIDs) != 1 || seenBasketIDs[0] != "basket-1" {
		t.Fatalf("expected basket clear fallback to basket-1, got %v", seenBasketIDs)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["mutation"] != "clear" {
		t.Fatalf("expected clear mutation, got %v", data["mutation"])
	}
}

func TestCartClearJSON(t *testing.T) {
	seenBasketIDs := []string{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{"id": "basket-1", "venue": map[string]any{"id": "venue-1"}, "items": []any{}},
						map[string]any{"id": "basket-2", "venue": map[string]any{"id": "venue-2"}, "items": []any{}},
					},
				}, nil
			},
			deleteBasketsFunc: func(_ context.Context, basketIDs []string, _ woltgateway.AuthContext) (map[string]any, error) {
				seenBasketIDs = append(seenBasketIDs, basketIDs...)
				return map[string]any{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "cart", "clear", "--all", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if len(seenBasketIDs) != 2 || seenBasketIDs[0] != "basket-1" || seenBasketIDs[1] != "basket-2" {
		t.Fatalf("expected clear of basket-1 and basket-2, got %v", seenBasketIDs)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["cleared_baskets"]) != 2 {
		t.Fatalf("expected cleared_baskets=2, got %v", data["cleared_baskets"])
	}
}

func TestCheckoutPreviewJSON(t *testing.T) {
	seenPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1", "country": "FIN"},
							"items": []any{
								map[string]any{
									"id":    "item-1",
									"count": 1,
									"price": 1700,
									"options": []any{
										map[string]any{
											"id": "group-1",
											"values": []any{
												map[string]any{"id": "value-1", "count": 1},
											},
										},
									},
								},
							},
						},
					},
				}, nil
			},
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"sections": []any{
						map[string]any{
							"categories": []any{
								map[string]any{
									"id":       "cat-1",
									"item_ids": []any{"item-1"},
								},
							},
							"options": []any{
								map[string]any{
									"id": "group-1",
									"values": []any{
										map[string]any{"id": "value-1", "price": 150},
									},
								},
							},
						},
					},
				}, nil
			},
			checkoutPreviewFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenPayload = payload
				return map[string]any{
					"payable_amount": 1819,
					"payment_breakdown": map[string]any{
						"total": map[string]any{"formatted_amount": "€18.19"},
					},
					"checkout_rows": []any{
						map[string]any{
							"template": "price_total_amount_row",
							"label":    "Total",
							"price_total_amount": map[string]any{
								"formatted_amount": "€18.19",
							},
						},
					},
					"delivery_configs": []any{},
					"offers":           map[string]any{"selectable": []any{}, "applied": []any{}},
					"tip_config":       map[string]any{"min_amount": 50},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "preview", "--wtoken", "token", "--tip", "200", "--promo-code", "promo-1", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	plan := asMapPayload(t, seenPayload["purchase_plan"])
	if asIntPayload(plan["courier_tip"]) != 200 {
		t.Fatalf("expected tip 200, got %v", plan["courier_tip"])
	}
	menuItems := asSlicePayload(t, plan["menu_items"])
	if len(menuItems) != 1 {
		t.Fatalf("expected one menu item, got %d", len(menuItems))
	}
	firstItem := asMapPayload(t, menuItems[0])
	if firstItem["category_id"] != "cat-1" {
		t.Fatalf("expected category_id cat-1, got %v", firstItem["category_id"])
	}
	options := asSlicePayload(t, firstItem["options"])
	if len(options) != 1 {
		t.Fatalf("expected one option group, got %d", len(options))
	}
	selectedValues := asSlicePayload(t, asMapPayload(t, options[0])["values"])
	if len(selectedValues) != 1 {
		t.Fatalf("expected one selected option value, got %d", len(selectedValues))
	}
	if asIntPayload(asMapPayload(t, selectedValues[0])["price"]) != 150 {
		t.Fatalf("expected option value price 150, got %v", asMapPayload(t, selectedValues[0])["price"])
	}
	promoIDs := asSlicePayload(t, plan["use_promo_discount_ids"])
	if len(promoIDs) != 1 || promoIDs[0] != "promo-1" {
		t.Fatalf("expected promo id promo-1, got %v", promoIDs)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["basket_id"] != "basket-1" {
		t.Fatalf("expected basket_id basket-1, got %v", data["basket_id"])
	}
	if data["venue_id"] != "venue-1" {
		t.Fatalf("expected venue_id venue-1, got %v", data["venue_id"])
	}
	selection := asMapPayload(t, data["selection"])
	if selection["selection_mode"] != "first-available" {
		t.Fatalf("expected selection_mode first-available, got %v", selection["selection_mode"])
	}
	warnings := asSlicePayload(t, payload["warnings"])
	for _, warning := range warnings {
		if strings.Contains(asStringPayload(warning), "multiple baskets found") {
			t.Fatalf("did not expect multiple-baskets warning for single basket, got %v", warnings)
		}
	}
	if asIntPayload(asMapPayload(t, data["payable_amount"])["amount"]) != 1819 {
		t.Fatalf("expected payable amount 1819, got %v", asMapPayload(t, data["payable_amount"])["amount"])
	}
}

func TestCheckoutPreviewUsesVenuePayloadCategoryFallback(t *testing.T) {
	seenPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1", "country": "FIN", "slug": "venue-1-slug"},
							"items": []any{
								map[string]any{
									"id":      "item-1",
									"count":   1,
									"price":   1700,
									"options": []any{},
								},
							},
						},
					},
				}, nil
			},
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"id":   "item-1",
					"name": "Fallback Burger",
				}, nil
			},
			assortmentBySlugFunc: func(context.Context, string) (map[string]any, error) {
				return map[string]any{
					"categories": []any{
						map[string]any{
							"id":       "cat-fallback",
							"item_ids": []any{"item-1"},
						},
					},
				}, nil
			},
			checkoutPreviewFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenPayload = payload
				return map[string]any{
					"payable_amount": 1700,
					"checkout_rows":  []any{},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "preview", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	plan := asMapPayload(t, seenPayload["purchase_plan"])
	menuItems := asSlicePayload(t, plan["menu_items"])
	if len(menuItems) != 1 {
		t.Fatalf("expected one menu item, got %d", len(menuItems))
	}
	firstItem := asMapPayload(t, menuItems[0])
	if firstItem["category_id"] != "cat-fallback" {
		t.Fatalf("expected fallback category_id cat-fallback, got %v", firstItem["category_id"])
	}
}

func TestCheckoutPreviewFallsBackCategoryToItemID(t *testing.T) {
	seenPayload := map[string]any{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1", "country": "FIN"},
							"items": []any{
								map[string]any{
									"id":      "693f837c465e0fe77eef4630",
									"count":   1,
									"price":   1700,
									"options": []any{},
								},
							},
						},
					},
				}, nil
			},
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"id": "693f837c465e0fe77eef4630",
				}, nil
			},
			checkoutPreviewFunc: func(_ context.Context, payload map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				seenPayload = payload
				return map[string]any{
					"payable_amount": 1700,
					"checkout_rows":  []any{},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "preview", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}

	plan := asMapPayload(t, seenPayload["purchase_plan"])
	menuItems := asSlicePayload(t, plan["menu_items"])
	if len(menuItems) != 1 {
		t.Fatalf("expected one menu item, got %d", len(menuItems))
	}
	firstItem := asMapPayload(t, menuItems[0])
	if firstItem["category_id"] != "693f837c465e0fe77eef4630" {
		t.Fatalf("expected category fallback to item id, got %v", firstItem["category_id"])
	}

	payload := mustJSON(t, out)
	warnings := asSlicePayload(t, payload["warnings"])
	found := false
	for _, warning := range warnings {
		if strings.Contains(asStringPayload(warning), "falling back to item id") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fallback warning, got %v", warnings)
	}
}

func TestCheckoutPreviewMultipleBasketsSelectionWarning(t *testing.T) {
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			basketsPageFunc: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"baskets": []any{
						map[string]any{
							"id":    "basket-1",
							"total": "€17.00",
							"venue": map[string]any{"id": "venue-1", "country": "FIN"},
							"items": []any{
								map[string]any{"id": "item-1", "count": 1, "price": 1700, "options": []any{}},
							},
						},
						map[string]any{
							"id":    "basket-2",
							"total": "€12.00",
							"venue": map[string]any{"id": "venue-2", "country": "FIN"},
							"items": []any{
								map[string]any{"id": "item-2", "count": 1, "price": 1200, "options": []any{}},
							},
						},
					},
				}, nil
			},
			venueItemPageFunc: func(context.Context, string, string) (map[string]any, error) {
				return map[string]any{
					"sections": []any{
						map[string]any{
							"categories": []any{
								map[string]any{
									"id":       "cat-1",
									"item_ids": []any{"item-1", "item-2"},
								},
							},
						},
					},
				}, nil
			},
			checkoutPreviewFunc: func(_ context.Context, _ map[string]any, _ woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"payable_amount": 1700,
					"checkout_rows":  []any{},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1, Lon: 24.9}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "checkout", "preview", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["basket_id"] != "basket-1" {
		t.Fatalf("expected first basket to be selected, got %v", data["basket_id"])
	}
	selection := asMapPayload(t, data["selection"])
	if asIntPayload(selection["basket_count"]) != 2 {
		t.Fatalf("expected basket_count 2, got %v", selection["basket_count"])
	}
	if selection["selection_mode"] != "first-available" {
		t.Fatalf("expected selection_mode first-available, got %v", selection["selection_mode"])
	}
	warnings := asSlicePayload(t, payload["warnings"])
	found := false
	for _, warning := range warnings {
		if strings.Contains(asStringPayload(warning), "multiple baskets found") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected warning for multiple baskets selection, got %v", warnings)
	}
}

func TestProfileAddressesJSON(t *testing.T) {
	cfg := &recordingConfig{
		loadCfg: domain.Config{
			Profiles: []domain.Profile{
				{
					Name:      "default",
					IsDefault: true,
					Location:  domain.Location{Lat: 60.1484, Lon: 24.6913},
					WToken:    "token",
				},
			},
		},
	}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			deliveryInfoListFunc: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"results": []any{
						map[string]any{
							"id":         "addr-1",
							"label_type": "other",
							"location": map[string]any{
								"address": "Iivisniemenkatu 2",
							},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.1484, Lon: 24.6913}, WToken: "token"}},
		Location: &mockLocation{},
		Config:   cfg,
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "profile", "addresses", "--format", "json", "--wtoken", "token")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	addresses := asSlicePayload(t, data["addresses"])
	if len(addresses) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addresses))
	}
	if !strings.Contains(out, "profile_default_address_id") {
		t.Fatalf("expected profile_default_address_id in payload")
	}
}

func TestProfilePaymentsRequiresAuth(t *testing.T) {
	deps := cli.Dependencies{
		Wolt:     &mockWolt{},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60, Lon: 24}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "profile", "payments", "--format", "json")
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d\noutput:\n%s", exitCode, out)
	}
	payload := mustJSON(t, out)
	errPayload := asMapPayload(t, payload["error"])
	if errPayload["code"] != "WOLT_AUTH_REQUIRED" {
		t.Fatalf("expected WOLT_AUTH_REQUIRED, got %v", errPayload["code"])
	}
}

func TestProfileFavoritesListJSON(t *testing.T) {
	seenLocation := domain.Location{}
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			favoriteVenuesFunc: func(_ context.Context, location domain.Location, _ woltgateway.AuthContext) (map[string]any, error) {
				seenLocation = location
				return map[string]any{
					"sections": []any{
						map[string]any{
							"items": []any{
								map[string]any{
									"title": "Rioni Espoo",
									"link":  map[string]any{"target": "https://wolt.com/en/fin/espoo/restaurant/rioni-espoo"},
									"venue": map[string]any{
										"id":        "5a8426f188b5de000b8857bb",
										"slug":      "rioni-espoo",
										"name":      "Rioni Espoo",
										"address":   "Espoonlahdenkatu 8",
										"favourite": true,
										"rating":    map[string]any{"score": 9.0},
									},
								},
							},
						},
					},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.14889, Lon: 24.6911577}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(t, deps, "profile", "favorites", "--wtoken", "token", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenLocation.Lat != 60.14889 || seenLocation.Lon != 24.6911577 {
		t.Fatalf("expected profile location to be forwarded, got %+v", seenLocation)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected favorites count 1, got %v", data["count"])
	}
	rows := asSlicePayload(t, data["favorites"])
	if len(rows) != 1 {
		t.Fatalf("expected one favorite row, got %d", len(rows))
	}
	first := asMapPayload(t, rows[0])
	if first["venue_id"] != "5a8426f188b5de000b8857bb" {
		t.Fatalf("unexpected venue id: %v", first["venue_id"])
	}
}

func TestProfileFavoritesAddBySlugJSON(t *testing.T) {
	seenVenueID := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			venuePageStaticFunc: func(_ context.Context, slug string) (map[string]any, error) {
				if slug != "rioni-espoo" {
					t.Fatalf("expected slug rioni-espoo, got %q", slug)
				}
				return map[string]any{
					"venue": map[string]any{
						"id":   "5a8426f188b5de000b8857bb",
						"slug": "rioni-espoo",
						"name": "Rioni Espoo",
					},
				}, nil
			},
			favoriteVenueAddFn: func(_ context.Context, venueID string, auth woltgateway.AuthContext) (map[string]any, error) {
				seenVenueID = venueID
				if auth.WToken != "token" {
					t.Fatalf("expected token to be forwarded")
				}
				return map[string]any{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.14889, Lon: 24.6911577}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"profile",
		"favorites",
		"add",
		"https://wolt.com/en/fin/espoo/restaurant/rioni-espoo",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenVenueID != "5a8426f188b5de000b8857bb" {
		t.Fatalf("expected venue id 5a8426f188b5de000b8857bb, got %q", seenVenueID)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["action"] != "add" {
		t.Fatalf("expected action add, got %v", data["action"])
	}
	if !asBoolPayload(data["is_favorite"]) {
		t.Fatalf("expected is_favorite=true, got %v", data["is_favorite"])
	}
}

func TestProfileFavoritesRemoveByIDJSON(t *testing.T) {
	seenVenueID := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			favoriteVenueRemFn: func(_ context.Context, venueID string, auth woltgateway.AuthContext) (map[string]any, error) {
				seenVenueID = venueID
				if auth.WToken != "token" {
					t.Fatalf("expected token to be forwarded")
				}
				return map[string]any{}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.14889, Lon: 24.6911577}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"profile",
		"favorites",
		"remove",
		"5a8426f188b5de000b8857bb",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenVenueID != "5a8426f188b5de000b8857bb" {
		t.Fatalf("expected venue id 5a8426f188b5de000b8857bb, got %q", seenVenueID)
	}
	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["action"] != "remove" {
		t.Fatalf("expected action remove, got %v", data["action"])
	}
	if asBoolPayload(data["is_favorite"]) {
		t.Fatalf("expected is_favorite=false, got %v", data["is_favorite"])
	}
}

func TestProfileOrdersListJSON(t *testing.T) {
	seenLimit := 0
	seenPageToken := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			orderHistoryFunc: func(_ context.Context, auth woltgateway.AuthContext, options woltgateway.OrderHistoryOptions) (map[string]any, error) {
				if auth.WToken != "token" {
					t.Fatalf("expected token to be forwarded")
				}
				seenLimit = options.Limit
				seenPageToken = options.PageToken
				return map[string]any{
					"orders": []any{
						map[string]any{
							"purchase_id":     "purchase-1",
							"received_at":     "15/02/2026, 10:06",
							"status":          "delivered",
							"venue_name":      "Burger King Iso Omena",
							"total_amount":    "€15.38",
							"is_active":       false,
							"payment_time_ts": 1771142803530,
							"items": []any{
								map[string]any{"name": "LONG CHICKEN®"},
								map[string]any{"name": "WHOPPER® Jr."},
							},
						},
						map[string]any{
							"purchase_id":     "purchase-2",
							"received_at":     "03/02/2026, 11:07",
							"status":          "deferred_payment_failed",
							"venue_name":      "KFC Iso Omena",
							"total_amount":    "--",
							"is_active":       false,
							"payment_time_ts": 1770109674437,
							"items":           "Twister Lunchbox",
						},
					},
					"next_page_token": "2025-12-03T14:40:50.585Z",
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.14889, Lon: 24.6911577}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"profile",
		"orders",
		"--wtoken",
		"token",
		"--limit",
		"25",
		"--status",
		"delivered",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenLimit != 25 {
		t.Fatalf("expected limit 25, got %d", seenLimit)
	}
	if seenPageToken != "" {
		t.Fatalf("expected empty page token on first request, got %q", seenPageToken)
	}

	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if asIntPayload(data["count"]) != 1 {
		t.Fatalf("expected one filtered order, got %v", data["count"])
	}
	if asStringPayload(data["next_page_token"]) != "2025-12-03T14:40:50.585Z" {
		t.Fatalf("expected next_page_token in payload, got %v", data["next_page_token"])
	}
	orders := asSlicePayload(t, data["orders"])
	if len(orders) != 1 {
		t.Fatalf("expected one order in payload, got %d", len(orders))
	}
	first := asMapPayload(t, orders[0])
	if first["purchase_id"] != "purchase-1" {
		t.Fatalf("unexpected purchase id: %v", first["purchase_id"])
	}
	if first["status"] != "delivered" {
		t.Fatalf("unexpected order status: %v", first["status"])
	}
	if first["items_summary"] != "LONG CHICKEN®, WHOPPER® Jr." {
		t.Fatalf("unexpected items summary: %v", first["items_summary"])
	}
}

func TestProfileOrdersShowJSON(t *testing.T) {
	seenPurchaseID := ""
	deps := cli.Dependencies{
		Wolt: &mockWolt{
			orderHistoryShowFn: func(_ context.Context, purchaseID string, auth woltgateway.AuthContext) (map[string]any, error) {
				seenPurchaseID = purchaseID
				if auth.WToken != "token" {
					t.Fatalf("expected token to be forwarded")
				}
				return map[string]any{
					"order_id":           "purchase-1",
					"order_number":       "413",
					"status":             "delivered",
					"creation_time":      "15/02/2026, 10:06",
					"delivery_time":      "15/02/2026, 10:37",
					"delivery_method":    "homedelivery",
					"currency":           "EUR",
					"venue_id":           "5bbdcb7d556891000bf66ed8",
					"venue_name":         "Burger King Iso Omena",
					"venue_full_address": "Piispansilta 11, 02230, Espoo",
					"venue_phone":        "+358403546482",
					"venue_country":      "FIN",
					"venue_product_line": "restaurant",
					"total_price":        1538,
					"items_price":        2395,
					"delivery_price":     101,
					"service_fee":        101,
					"subtotal":           1538,
					"credits":            0,
					"tokens":             0,
					"items": []any{
						map[string]any{
							"id":         "item-1",
							"name":       "LONG CHICKEN®",
							"count":      1,
							"price":      825,
							"end_amount": 825,
							"options":    []any{},
						},
					},
					"payments": []any{
						map[string]any{
							"name":         "Edenred",
							"amount":       1538,
							"payment_time": "15/02/2026, 10:06",
							"method": map[string]any{
								"type":     "edenred",
								"id":       "payment-1",
								"provider": "edenred",
							},
						},
					},
					"delivery_location": map[string]any{
						"alias":  "Home",
						"street": "Iivisniemenkatu 2, 36",
						"city":   "Espoo",
					},
					"delivery_comment": "Leave order at the door",
					"discounts": []any{
						map[string]any{"title": "Discounts", "amount": 958},
					},
					"surcharges": []any{},
				}, nil
			},
		},
		Profiles: &mockProfiles{profile: domain.Profile{Name: "default", IsDefault: true, Location: domain.Location{Lat: 60.14889, Lon: 24.6911577}}},
		Location: &mockLocation{},
		Config:   &mockConfig{},
		Version:  "1.1.1",
	}

	exitCode, out := runCLIWithDeps(
		t,
		deps,
		"profile",
		"orders",
		"show",
		"purchase-1",
		"--wtoken",
		"token",
		"--format",
		"json",
	)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d\noutput:\n%s", exitCode, out)
	}
	if seenPurchaseID != "purchase-1" {
		t.Fatalf("expected purchase id purchase-1, got %q", seenPurchaseID)
	}

	payload := mustJSON(t, out)
	data := asMapPayload(t, payload["data"])
	if data["order_id"] != "purchase-1" {
		t.Fatalf("expected order_id purchase-1, got %v", data["order_id"])
	}
	totals := asMapPayload(t, data["totals"])
	total := asMapPayload(t, totals["total"])
	if asIntPayload(total["amount"]) != 1538 {
		t.Fatalf("expected total amount 1538, got %v", total["amount"])
	}
	if asStringPayload(total["formatted_amount"]) != "€15.38" {
		t.Fatalf("expected formatted total €15.38, got %v", total["formatted_amount"])
	}
	items := asSlicePayload(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one item in detailed order payload, got %d", len(items))
	}
}

func asBoolPayload(value any) bool {
	b, ok := value.(bool)
	return ok && b
}
