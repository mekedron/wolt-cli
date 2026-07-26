package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

const (
	scheduledVenueID       = "000000000000000000000001"
	scheduledVenueSlug     = "scheduled-market"
	scheduledVenueName     = "Scheduled Market"
	scheduledVenueURL      = "https://wolt.com/en/test/venue/scheduled-market"
	scheduledVenueCurrency = "EUR"
	scheduledVenueTimezone = "Etc/UTC"
	fixtureLocationLat     = 10.25
	fixtureLocationLon     = 20.5
	fixtureOpeningAt       = "2030-01-15T10:00:00Z"
	fixtureClosingAt       = "2030-01-15T11:00:00Z"
)

type venueOutputHandlerCase struct {
	name   string
	invoke func(*ToolCtx) (map[string]any, []string, error)
}

func venueOutputHandlerCases(venue string) []venueOutputHandlerCase {
	return []venueOutputHandlerCase{
		{
			name: "venue detail",
			invoke: func(tc *ToolCtx) (map[string]any, []string, error) {
				_, out, err := tc.handleVenueDetail(context.Background(), nil, VenueDetailInput{
					LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
					Venue:         venue,
				})
				return out.Venue, out.Warnings, err
			},
		},
		{
			name: "venue resolver",
			invoke: func(tc *ToolCtx) (map[string]any, []string, error) {
				_, out, err := tc.handleResolveVenue(context.Background(), nil, ResolveVenueInput{
					LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
					Venue:         venue,
				})
				return out.Venue, out.Warnings, err
			},
		},
	}
}

func TestVenueDetailAndHoursResolveSlugIDAndURLWithoutRestaurantEndpoint(t *testing.T) {
	wolt := &stubWolt{
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			switch input {
			case scheduledVenueSlug, scheduledVenueID:
				return scheduledVenueStaticPayload(), nil
			default:
				return nil, errors.New("not found")
			}
		},
		venueDynamicFn: func(_ context.Context, input string, opts woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			if input != scheduledVenueSlug {
				t.Fatalf("dynamic slug = %q, want %q", input, scheduledVenueSlug)
			}
			if opts.Location == nil ||
				opts.Location.Lat != fixtureLocationLat ||
				opts.Location.Lon != fixtureLocationLon {
				t.Fatalf("dynamic location = %#v", opts.Location)
			}
			return scheduledVenueDynamicPayload(), nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	for _, input := range []string{
		scheduledVenueSlug,
		scheduledVenueID,
		scheduledVenueURL,
	} {
		t.Run(input, func(t *testing.T) {
			_, out, err := tc.handleVenueDetail(context.Background(), nil, VenueDetailInput{
				LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
				Venue:         input,
			})
			if err != nil {
				t.Fatalf("handleVenueDetail: %v", err)
			}
			if out.Venue["venue_id"] != scheduledVenueID || out.Venue["slug"] != scheduledVenueSlug {
				t.Fatalf("unexpected identity: %#v", out.Venue)
			}
			availability := asMap(out.Venue["availability"])
			if availability["order_now_available"] != false ||
				availability["scheduled_order_available"] != true ||
				availability["scheduled_only"] != true {
				t.Fatalf("unexpected availability: %#v", availability)
			}

			_, hours, err := tc.handleVenueHours(context.Background(), nil, VenueHoursInput{Venue: input})
			if err != nil {
				t.Fatalf("handleVenueHours: %v", err)
			}
			if hours.Hours["timezone"] != scheduledVenueTimezone {
				t.Fatalf("timezone = %v", hours.Hours["timezone"])
			}
			if got := len(asSlice(hours.Hours["opening_windows"])); got != 7 {
				t.Fatalf("opening windows = %d, want 7", got)
			}
		})
	}
}

func TestVenueDetailDirectReferencesDoNotRequireLocation(t *testing.T) {
	for _, input := range []string{
		scheduledVenueSlug,
		scheduledVenueID,
		scheduledVenueURL,
	} {
		t.Run(input, func(t *testing.T) {
			expectedStaticInput := scheduledVenueSlug
			if input == scheduledVenueID {
				expectedStaticInput = scheduledVenueID
			}
			wolt := &stubWolt{
				venueStaticFn: func(_ context.Context, reference string) (map[string]any, error) {
					if reference != expectedStaticInput {
						t.Fatalf("static reference = %q, want %q", reference, expectedStaticInput)
					}
					return scheduledVenueStaticPayload(), nil
				},
				venueDynamicFn: func(
					_ context.Context,
					reference string,
					options woltgateway.VenuePageDynamicOptions,
				) (map[string]any, error) {
					if reference != scheduledVenueSlug {
						t.Fatalf("dynamic reference = %q, want %q", reference, scheduledVenueSlug)
					}
					if options.Location != nil {
						t.Fatalf("dynamic location = %#v, want nil", options.Location)
					}
					return scheduledVenueDynamicPayload(), nil
				},
				searchFn: func(context.Context, domain.Location, string) (map[string]any, error) {
					t.Fatal("direct venue detail must not require search")
					return nil, nil
				},
				itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
					t.Fatal("direct venue detail must not require discovery")
					return nil, nil
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt})

			_, out, err := tc.handleVenueDetail(context.Background(), nil, VenueDetailInput{Venue: input})
			if err != nil {
				t.Fatalf("handleVenueDetail: %v", err)
			}
			if out.Venue["venue_id"] != scheduledVenueID || out.Venue["slug"] != scheduledVenueSlug {
				t.Fatalf("unexpected identity: %#v", out.Venue)
			}
			if out.LocationSource != "" || out.Location != nil {
				t.Fatalf("location = %#v from %q, want unavailable", out.Location, out.LocationSource)
			}
			if !containsWarning(out.Warnings, "no location was available") {
				t.Fatalf("warnings = %v, want location warning", out.Warnings)
			}
			availability := asMap(out.Venue["availability"])
			if availability["delivers_to_location"] != nil {
				t.Fatalf("delivers_to_location = %#v, want unknown", availability["delivers_to_location"])
			}
			encoded, marshalErr := json.Marshal(out)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var output map[string]any
			if unmarshalErr := json.Unmarshal(encoded, &output); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			if _, exists := output["location"]; exists {
				t.Fatalf("serialized output contains unavailable location: %s", encoded)
			}
			if _, exists := output["location_source"]; exists {
				t.Fatalf("serialized output contains unavailable location_source: %s", encoded)
			}
		})
	}
}

func TestVenueDynamicReadsRetryAnonymouslyAfterOptionalAuth401(t *testing.T) {
	for _, test := range venueOutputHandlerCases(scheduledVenueSlug) {
		t.Run(test.name, func(t *testing.T) {
			authCalls := []bool{}
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return scheduledVenueStaticPayload(), nil
				},
				venueDynamicFn: func(
					_ context.Context,
					slug string,
					options woltgateway.VenuePageDynamicOptions,
				) (map[string]any, error) {
					if slug != scheduledVenueSlug {
						t.Fatalf("dynamic slug = %q, want %q", slug, scheduledVenueSlug)
					}
					authCalls = append(authCalls, options.Auth.HasCredentials())
					if options.Auth.HasCredentials() {
						return nil, &woltgateway.UpstreamRequestError{StatusCode: 401}
					}
					return scheduledVenueDynamicPayload(), nil
				},
			}
			tc := newToolCtx(Deps{
				Wolt: wolt,
				Profiles: &stubProfiles{profile: domain.Profile{
					Name:   "default",
					WToken: "stale-token",
				}},
			})

			venue, warnings, err := test.invoke(tc)
			if err != nil {
				t.Fatalf("dynamic venue read: %v", err)
			}
			if len(authCalls) != 2 || !authCalls[0] || authCalls[1] {
				t.Fatalf("authenticated calls = %v, want [true false]", authCalls)
			}
			availability := asMap(venue["availability"])
			if availability["scheduled_order_available"] != true {
				t.Fatalf("availability = %#v", availability)
			}
			if containsWarning(warnings, "availability could not be loaded") {
				t.Fatalf("warnings = %v", warnings)
			}
		})
	}
}

func TestResolveVenueAcceptsDynamicIdentityWithoutStaticOrDiscovery(t *testing.T) {
	searchCalled := false
	discoveryCalled := false
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static page unavailable")
		},
		venueDynamicFn: func(_ context.Context, slug string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			if slug != scheduledVenueSlug {
				t.Fatalf("dynamic slug = %q, want %q", slug, scheduledVenueSlug)
			}
			return scheduledVenueDynamicPayload(), nil
		},
		searchFn: func(context.Context, domain.Location, string) (map[string]any, error) {
			searchCalled = true
			return nil, errors.New("search must not be required")
		},
		itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
			discoveryCalled = true
			return nil, errors.New("discovery must not be required")
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleResolveVenue(context.Background(), nil, ResolveVenueInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueSlug,
	})
	if err != nil {
		t.Fatalf("handleResolveVenue: %v", err)
	}
	if searchCalled || discoveryCalled {
		t.Fatalf("resolved dynamic identity still used fallbacks: search=%v discovery=%v", searchCalled, discoveryCalled)
	}
	if out.Venue["venue_id"] != scheduledVenueID || out.Venue["slug"] != scheduledVenueSlug {
		t.Fatalf("unexpected identity: %#v", out.Venue)
	}
	if !containsWarning(out.Warnings, "static details could not be loaded") {
		t.Fatalf("warnings = %v, want static-details warning", out.Warnings)
	}
}

func TestVenueHandlersKeepExactSearchCandidateWhenStaticPageUnavailable(t *testing.T) {
	for _, test := range venueOutputHandlerCases(scheduledVenueName) {
		t.Run(test.name, func(t *testing.T) {
			wolt := &stubWolt{
				venueStaticFn: func(context.Context, string) (map[string]any, error) {
					return nil, errors.New("static page unavailable")
				},
				venueDynamicFn: func(_ context.Context, input string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
					if input == scheduledVenueName {
						return nil, errors.New("exact name is not a slug")
					}
					if input != scheduledVenueSlug {
						t.Fatalf("dynamic input = %q, want %q", input, scheduledVenueSlug)
					}
					payload := scheduledVenueDynamicPayload()
					delete(payload["venue"].(map[string]any), "id")
					return payload, nil
				},
				searchFn: func(_ context.Context, _ domain.Location, query string) (map[string]any, error) {
					if query != scheduledVenueName {
						t.Fatalf("search query = %q, want %q", query, scheduledVenueName)
					}
					return scheduledVenueSearchPayload(), nil
				},
				itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
					t.Fatal("discovery must not run after an exact search match")
					return nil, nil
				},
			}
			tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

			venue, warnings, err := test.invoke(tc)
			if err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			if venue["venue_id"] != scheduledVenueID || venue["slug"] != scheduledVenueSlug {
				t.Fatalf("unexpected identity: %#v", venue)
			}
			if venue["name"] != scheduledVenueName || venue["canonical_url"] != scheduledVenueURL {
				t.Fatalf("search candidate metadata was not retained: %#v", venue)
			}
			availability := asMap(venue["availability"])
			if availability["telemetry_status"] != "scheduled_order__without_time" {
				t.Fatalf("search candidate availability was not retained: %#v", availability)
			}
			if !containsWarning(warnings, "static details could not be loaded") {
				t.Fatalf("warnings = %v, want static-details warning", warnings)
			}
		})
	}
}

func TestVenueHoursPreservesUpstreamErrorClassification(t *testing.T) {
	tc := newToolCtx(Deps{
		Wolt: &stubWolt{
			venueStaticFn: func(context.Context, string) (map[string]any, error) {
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 503, Body: "must not leak"}
			},
		},
		Profiles: &stubProfiles{},
	})

	_, _, err := tc.handleVenueHours(context.Background(), nil, VenueHoursInput{Venue: "example-market"})
	var classified *classifiedToolError
	if !errors.As(err, &classified) {
		t.Fatalf("error = %T %v, want classified upstream error", err, err)
	}
	if classified.info.Code != "UPSTREAM_TEMPORARY" || !classified.info.Retryable {
		t.Fatalf("classification = %+v", classified.info)
	}
	if strings.Contains(classified.info.Message, "must not leak") {
		t.Fatalf("upstream body leaked: %q", classified.info.Message)
	}
}

func TestResolveVenueUsesSearchForClosedExactName(t *testing.T) {
	searchUsed := false
	wolt := &stubWolt{
		itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
			t.Fatal("discovery fallback must not run when exact search succeeds")
			return nil, nil
		},
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			if input == scheduledVenueSlug {
				return scheduledVenueStaticPayload(), nil
			}
			return nil, errors.New("not a slug")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return scheduledVenueDynamicPayload(), nil
		},
		searchFn: func(_ context.Context, _ domain.Location, query string) (map[string]any, error) {
			searchUsed = true
			if query != scheduledVenueName {
				t.Fatalf("search query = %q", query)
			}
			return scheduledVenueSearchPayload(), nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleResolveVenue(context.Background(), nil, ResolveVenueInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         scheduledVenueName,
	})
	if err != nil {
		t.Fatalf("handleResolveVenue: %v", err)
	}
	if !searchUsed {
		t.Fatal("venue search fallback was not used")
	}
	if out.Venue["venue_id"] != scheduledVenueID || out.Venue["slug"] != scheduledVenueSlug {
		t.Fatalf("unexpected identity: %#v", out.Venue)
	}
	if out.Venue["canonical_url"] != scheduledVenueURL {
		t.Fatalf("canonical_url = %v", out.Venue["canonical_url"])
	}
}

func TestResolveVenueURLSearchFallbackUsesNormalizedSlug(t *testing.T) {
	searchCalls := 0
	wolt := &stubWolt{
		venueStaticFn: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static unavailable")
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("dynamic unavailable")
		},
		searchFn: func(_ context.Context, _ domain.Location, query string) (map[string]any, error) {
			searchCalls++
			if query != scheduledVenueSlug {
				t.Fatalf("search query = %q, want normalized slug %q", query, scheduledVenueSlug)
			}
			return scheduledVenueSearchPayload(), nil
		},
		itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
			t.Fatal("discovery fallback must not run when exact search succeeds")
			return nil, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	ref, candidate, err := tc.resolveVenueRefWithSearch(
		context.Background(),
		scheduledVenueURL+"/categories/example",
		domain.Location{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
	)
	if err != nil {
		t.Fatalf("resolveVenueRefWithSearch: %v", err)
	}
	if searchCalls != 1 {
		t.Fatalf("search calls = %d, want 1", searchCalls)
	}
	if ref.ID != scheduledVenueID || ref.Slug != scheduledVenueSlug {
		t.Fatalf("resolved reference = %#v", ref)
	}
	if asString(candidate["venue_id"]) != scheduledVenueID {
		t.Fatalf("candidate = %#v", candidate)
	}
}

func TestResolveVenueFallsBackToExactDiscoveryName(t *testing.T) {
	const (
		venueID = "000000000000000000000002"
		slug    = "fallback-market"
		name    = "Fallback Market"
	)
	searchUsed := false
	discoveryUsed := false
	wolt := &stubWolt{
		searchFn: func(_ context.Context, _ domain.Location, query string) (map[string]any, error) {
			searchUsed = true
			if query != name {
				t.Fatalf("search query = %q", query)
			}
			return map[string]any{}, nil
		},
		itemsFn: func(_ context.Context, location domain.Location) ([]domain.Item, error) {
			discoveryUsed = true
			if location.Lat != fixtureLocationLat || location.Lon != fixtureLocationLon {
				t.Fatalf("discovery location = %#v", location)
			}
			item := domain.Item{
				Title: name,
				Link:  domain.Link{Target: venueID},
				Venue: &domain.Venue{
					ID:       venueID,
					Slug:     slug,
					Name:     name,
					Address:  "100 Example Street",
					Currency: "EUR",
				},
			}
			// The same venue can appear in several discovery sections. Exact
			// fallback must deduplicate it by canonical identity rather than
			// report a false ambiguity.
			return []domain.Item{item, item}, nil
		},
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			if input != slug {
				return nil, errors.New("not a slug")
			}
			return map[string]any{"venue": map[string]any{
				"id":        venueID,
				"slug":      slug,
				"name":      name,
				"address":   "100 Example Street",
				"currency":  "EUR",
				"timezone":  "Etc/UTC",
				"share_url": "https://wolt.com/en/test/venue/" + slug,
			}}, nil
		},
		venueDynamicFn: func(_ context.Context, input string, _ woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			if input == name {
				return nil, errors.New("not a slug")
			}
			if input != slug {
				t.Fatalf("unexpected dynamic input = %q", input)
			}
			return map[string]any{"venue": map[string]any{"id": venueID, "slug": slug}}, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleResolveVenue(context.Background(), nil, ResolveVenueInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         name,
	})
	if err != nil {
		t.Fatalf("handleResolveVenue: %v", err)
	}
	if !searchUsed || !discoveryUsed {
		t.Fatalf("fallback path not exercised: search=%v discovery=%v", searchUsed, discoveryUsed)
	}
	if out.Venue["venue_id"] != venueID || out.Venue["slug"] != slug {
		t.Fatalf("unexpected identity: %#v", out.Venue)
	}
	if out.Venue["canonical_url"] != "https://wolt.com/en/test/venue/"+slug {
		t.Fatalf("canonical_url = %v", out.Venue["canonical_url"])
	}
}

func TestResolveVenueUppercaseObjectIDSkipsSearchAfterDirectResolution(t *testing.T) {
	const (
		venueID   = "ABCDEFABCDEFABCDEFABCDEF"
		venueSlug = "direct-market"
	)
	wolt := &stubWolt{
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			if input != venueID {
				t.Fatalf("static input = %q, want %q", input, venueID)
			}
			return map[string]any{
				"venue": map[string]any{"id": venueID, "slug": venueSlug},
			}, nil
		},
		searchFn: func(context.Context, domain.Location, string) (map[string]any, error) {
			t.Fatal("direct ObjectID resolution must not call search")
			return nil, nil
		},
		itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
			t.Fatal("direct ObjectID resolution must not call discovery")
			return nil, nil
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	ref, candidate, err := tc.resolveVenueRefWithSearch(
		context.Background(),
		venueID,
		domain.Location{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
	)
	if err != nil {
		t.Fatalf("resolveVenueRefWithSearch: %v", err)
	}
	if candidate != nil || ref.ID != venueID || ref.Slug != venueSlug {
		t.Fatalf("resolved reference = %#v, candidate = %#v", ref, candidate)
	}
}

func TestDiscoveryVenueCandidatesOnlyUseObjectIDLinkTargetsAsVenueIDs(t *testing.T) {
	const venueID = "000000000000000000000002"
	tests := []struct {
		name      string
		link      string
		wantVenue string
	}{
		{name: "object id", link: venueID, wantVenue: venueID},
		{name: "slug", link: "fallback-market"},
		{name: "URL", link: "https://wolt.com/en/test/venue/fallback-market"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidates := discoveryVenueCandidates([]domain.Item{{
				Title: "Fallback Market",
				Link:  domain.Link{Target: test.link},
				Venue: &domain.Venue{Slug: "fallback-market"},
			}})
			if len(candidates) != 1 {
				t.Fatalf("candidates = %#v", candidates)
			}
			if got := asString(candidates[0]["venue_id"]); got != test.wantVenue {
				t.Fatalf("venue_id = %q, want %q", got, test.wantVenue)
			}
		})
	}
}

func TestResolveVenueDiscoveryFallbackMatchesSlugIdentity(t *testing.T) {
	const (
		venueID = "000000000000000000000002"
		slug    = "fallback-market"
		name    = "Fallback Market"
	)
	initialStaticLookupMissed := false
	fallbackStaticLookupResolved := false
	searchUsed := false
	discoveryUsed := false
	wolt := &stubWolt{
		searchFn: func(_ context.Context, _ domain.Location, query string) (map[string]any, error) {
			searchUsed = true
			if query != slug {
				t.Fatalf("search query = %q", query)
			}
			return map[string]any{}, nil
		},
		itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
			discoveryUsed = true
			return []domain.Item{{
				Title: name,
				Link:  domain.Link{Target: venueID},
				Venue: &domain.Venue{
					ID:      venueID,
					Slug:    slug,
					Name:    name,
					Address: "100 Example Street",
				},
			}}, nil
		},
		venueStaticFn: func(_ context.Context, input string) (map[string]any, error) {
			if input != slug {
				t.Fatalf("static input = %q", input)
			}
			if !initialStaticLookupMissed {
				initialStaticLookupMissed = true
				return nil, errors.New("first static lookup missed")
			}
			fallbackStaticLookupResolved = true
			return map[string]any{"venue": map[string]any{
				"id":        venueID,
				"slug":      slug,
				"name":      name,
				"address":   "100 Example Street",
				"currency":  "EUR",
				"timezone":  "Etc/UTC",
				"share_url": "https://wolt.com/en/test/venue/" + slug,
			}}, nil
		},
		venueDynamicFn: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("initial dynamic lookup missed")
		},
	}
	tc := newToolCtx(Deps{Wolt: wolt, Profiles: &stubProfiles{}})

	_, out, err := tc.handleResolveVenue(context.Background(), nil, ResolveVenueInput{
		LocationInput: LocationInput{Lat: fixtureLocationLat, Lon: fixtureLocationLon},
		Venue:         slug,
	})
	if err != nil {
		t.Fatalf("handleResolveVenue: %v", err)
	}
	if !initialStaticLookupMissed || !searchUsed || !discoveryUsed || !fallbackStaticLookupResolved {
		t.Fatalf(
			"fallback path not exercised: initial_static_miss=%v search=%v discovery=%v fallback_static=%v",
			initialStaticLookupMissed,
			searchUsed,
			discoveryUsed,
			fallbackStaticLookupResolved,
		)
	}
	if out.Venue["venue_id"] != venueID || out.Venue["slug"] != slug {
		t.Fatalf("unexpected identity: %#v", out.Venue)
	}
}

func scheduledVenueStaticPayload() map[string]any {
	openingTimes := map[string]any{}
	for _, day := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		openingTimes[day] = []any{
			map[string]any{"type": "open", "value": 36000},
			map[string]any{"type": "close", "value": 72000},
		}
	}
	return map[string]any{
		"order_minimum": 2000,
		"venue": map[string]any{
			"id":               scheduledVenueID,
			"slug":             scheduledVenueSlug,
			"name":             scheduledVenueName,
			"address":          "100 Example Street",
			"currency":         scheduledVenueCurrency,
			"timezone":         scheduledVenueTimezone,
			"share_url":        scheduledVenueURL,
			"delivery_methods": []any{"homedelivery", "takeaway"},
			"delivery_geo_range": map[string]any{
				"type": "Polygon",
				"coordinates": []any{
					[]any{
						[]any{20.0, 10.0},
						[]any{21.0, 10.0},
						[]any{21.0, 11.0},
						[]any{20.0, 11.0},
						[]any{20.0, 10.0},
					},
				},
			},
		},
		"venue_raw": map[string]any{
			"id":            scheduledVenueID,
			"currency":      scheduledVenueCurrency,
			"opening_times": openingTimes,
		},
	}
}

func scheduledVenueDynamicPayload() map[string]any {
	return map[string]any{
		"venue": map[string]any{
			"id": scheduledVenueID,
			"delivery_open_status": map[string]any{
				"is_open":   false,
				"next_open": fixtureOpeningAt,
			},
			"open_status": map[string]any{"is_open": false},
			"header": map[string]any{
				"delivery_method_default": "DELIVERY_SCHEDULED",
				"delivery_method_statuses": []any{
					map[string]any{
						"delivery_method": "DELIVERY_SCHEDULED",
						"call_to_action":  map[string]any{"enabled": true},
					},
				},
			},
			"delivery_configs": []any{
				map[string]any{"method": "homedelivery", "schedule": "standard"},
				map[string]any{
					"method":   "homedelivery",
					"schedule": "time_slot",
					"tso_schedule": []any{
						map[string]any{
							"day": "fixture-day",
							"time_slots": []any{
								map[string]any{
									"time_slot_start": fixtureOpeningAt,
									"time_slot_end":   fixtureClosingAt,
								},
							},
						},
					},
				},
			},
		},
	}
}

func scheduledVenueSearchPayload() map[string]any {
	return map[string]any{
		"sections": []any{
			map[string]any{
				"name": "venues",
				"items": []any{
					map[string]any{
						"title": scheduledVenueName,
						"link": map[string]any{
							"target": scheduledVenueURL,
						},
						"overlay_v2": map[string]any{
							"primary_text":     "Schedule order",
							"telemetry_status": "scheduled_order__without_time",
						},
						"venue": map[string]any{
							"id":       scheduledVenueID,
							"slug":     scheduledVenueSlug,
							"name":     scheduledVenueName,
							"address":  "100 Example Street",
							"currency": scheduledVenueCurrency,
							"online":   false,
							"status": map[string]any{
								"primary_text":     "Schedule order",
								"telemetry_status": "scheduled_order__without_time",
							},
						},
					},
				},
			},
		},
	}
}
