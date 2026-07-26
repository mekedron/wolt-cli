package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

const (
	cliVenueID   = "000000000000000000000071"
	cliVenueSlug = "example-venue"
	cliVenueURL  = "https://wolt.com/en/example/venue/example-venue"
)

func TestVenueShowUsesSupportedPayloadsForSlugIDAndURL(t *testing.T) {
	for _, input := range []string{
		cliVenueSlug,
		cliVenueID,
		cliVenueURL + "/categories/example",
	} {
		t.Run(input, func(t *testing.T) {
			withIsolatedSlugCache(t)
			api := &testWoltAPI{
				venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
					return cliVenueStaticPayload(), nil
				},
				venuePageDynamicFn: func(
					_ context.Context,
					reference string,
					options woltgateway.VenuePageDynamicOptions,
				) (map[string]any, error) {
					if reference != cliVenueSlug {
						t.Fatalf("dynamic reference = %q, want %q", reference, cliVenueSlug)
					}
					if options.Location == nil ||
						options.Location.Lat != 10.25 ||
						options.Location.Lon != 20.5 ||
						options.SelectedDeliveryMethod != "homedelivery" {
						t.Fatalf("dynamic options = %#v", options)
					}
					return cliVenueDynamicPayload(), nil
				},
			}
			cmd := newVenueShowCommand(Dependencies{
				Wolt: api,
				Profiles: &testProfiles{profile: domain.Profile{
					Name:      "default",
					IsDefault: true,
					Location:  domain.Location{Lat: 10.25, Lon: 20.5},
				}},
			})
			output := &bytes.Buffer{}
			cmd.SetOut(output)
			cmd.SetErr(output)
			cmd.SetArgs([]string{
				input,
				"--include", "hours,tags,rating,fees,promotions",
				"--format", "json",
			})

			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("venue show: %v\n%s", err, output.String())
			}
			data := decodeCLIData(t, output)
			if asString(data["venue_id"]) != cliVenueID ||
				asString(data["slug"]) != cliVenueSlug ||
				asString(data["canonical_url"]) != cliVenueURL ||
				asString(data["currency"]) != "EUR" {
				t.Fatalf("venue identity = %#v", data)
			}
			if len(asSlice(data["opening_windows"])) != 2 {
				t.Fatalf("opening_windows = %#v", data["opening_windows"])
			}
		})
	}
}

func TestVenueShowRetriesDynamicAnonymouslyAndSurvivesStaticOutage(t *testing.T) {
	withIsolatedSlugCache(t)
	dynamicAuth := []bool{}
	api := &testWoltAPI{
		venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static unavailable")
		},
		venuePageDynamicFn: func(
			_ context.Context,
			reference string,
			options woltgateway.VenuePageDynamicOptions,
		) (map[string]any, error) {
			if reference != cliVenueSlug {
				t.Fatalf("dynamic reference = %q", reference)
			}
			dynamicAuth = append(dynamicAuth, options.Auth.HasCredentials())
			if options.Auth.HasCredentials() {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			return cliVenueDynamicPayload(), nil
		},
	}
	cmd := newVenueShowCommand(Dependencies{
		Wolt: api,
		Profiles: &testProfiles{profile: domain.Profile{
			Name:      "default",
			IsDefault: true,
			Location:  domain.Location{Lat: 10.25, Lon: 20.5},
			WToken:    "test-token",
		}},
	})
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{cliVenueSlug, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("venue show with static outage: %v\n%s", err, output.String())
	}
	if len(dynamicAuth) != 2 || !dynamicAuth[0] || dynamicAuth[1] {
		t.Fatalf("dynamic auth attempts = %v, want [true false]", dynamicAuth)
	}
	data := decodeCLIData(t, output)
	if asString(data["venue_id"]) != cliVenueID || asString(data["slug"]) != cliVenueSlug {
		t.Fatalf("dynamic identity = %#v", data)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	foundWarning := false
	for _, raw := range asSlice(envelope["warnings"]) {
		if strings.Contains(asString(raw), "static page") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("warnings = %#v", envelope["warnings"])
	}
}

func decodeCLIData(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	return asMap(envelope["data"])
}

func cliVenueStaticPayload() map[string]any {
	return map[string]any{
		"order_minimum": 1000,
		"venue": map[string]any{
			"id":               cliVenueID,
			"slug":             cliVenueSlug,
			"name":             "Example Venue",
			"address":          "100 Example Street",
			"currency":         "EUR",
			"timezone":         "Europe/Paris",
			"share_url":        cliVenueURL,
			"delivery_methods": []any{"homedelivery"},
			"rating": map[string]any{
				"score_raw": 8.6,
				"volume":    200,
			},
		},
		"venue_raw": map[string]any{
			"opening_times": map[string]any{
				"monday": []any{
					map[string]any{"type": "open", "value": 28800},
					map[string]any{"type": "close", "value": 43200},
					map[string]any{"type": "open", "value": 50400},
					map[string]any{"type": "close", "value": 72000},
				},
			},
			"food_tags": []any{"example"},
		},
	}
}

func cliVenueDynamicPayload() map[string]any {
	return map[string]any{
		"venue": map[string]any{
			"id":       cliVenueID,
			"slug":     cliVenueSlug,
			"name":     "Example Venue",
			"timezone": "Europe/Paris",
			"delivery_open_status": map[string]any{
				"is_open":   false,
				"next_open": "2030-01-15T10:00:00Z",
			},
		},
	}
}
