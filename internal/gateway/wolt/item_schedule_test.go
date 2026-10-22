package wolt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// routeHTTPClient answers by URL path suffix and counts calls per route.
type routeHTTPClient struct {
	routes map[string]string
	calls  map[string]int
}

func (c *routeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	for suffix, body := range c.routes {
		if strings.HasSuffix(req.URL.Path, suffix) {
			c.calls[suffix]++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

const scheduleTestStatic = `{"venue":{"slug":"trishuli","timezone":"Europe/Helsinki"}}`

// The dynamic page reports the venue's current time with its UTC offset.
const scheduleTestDynamic = `{"venue":{"open_status":{"now":"2026-10-22T15:51:00.123456+03:00"}}}`

// Monday-only lunch (10:30-15:00), an all-week item and a Friday-to-Saturday
// overnight window (22:00-02:00).
const scheduleTestAssortment = `{"items":[
 {"id":"lunch","name":"LAMB METHI","disabled_info":null,
  "available_times":[{"days":[1],"start_time":37800000,"end_time":54000000}]},
 {"id":"always","name":"Soup","disabled_info":null,
  "available_times":[{"days":[1,2,3,4,5,6,7],"start_time":0,"end_time":86400000}]},
 {"id":"late","name":"Night snack","disabled_info":null,
  "available_times":[{"days":[5],"start_time":79200000,"end_time":7200000}]}
]}`

func scheduleTestClient(t *testing.T, routes map[string]string, now string) (*Client, *routeHTTPClient) {
	t.Helper()
	helsinki, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", now, helsinki)
	if err != nil {
		t.Fatalf("parse now: %v", err)
	}
	httpClient := &routeHTTPClient{routes: routes}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			VenuePage:        "https://example.test/pages/venue/slug/",
			VenuePageDynamic: "https://example.test/venue/slug/",
			Assortment:       "https://example.test/assortment/v1/venues/slug/",
		}),
		// The clock runs in UTC, as on CI; windows are venue-local.
		WithClock(at.UTC),
	)
	return client, httpClient
}

func scheduleTestItems(t *testing.T, payload map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, raw := range payload["items"].([]any) {
		item := raw.(map[string]any)
		out[item["id"].(string)] = item
	}
	return out
}

func TestAssortmentDisablesItemsOutsideTheirSaleWindow(t *testing.T) {
	client, httpClient := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment": scheduleTestAssortment,
		"/trishuli/dynamic/":   scheduleTestDynamic,
		"/trishuli/static":     scheduleTestStatic,
	}, "2026-10-22 15:51") // Thursday

	payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
	if err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	items := scheduleTestItems(t, payload)
	disabled, ok := items["lunch"]["disabled_info"].(map[string]any)
	if !ok {
		t.Fatalf("lunch disabled_info = %#v, want a map", items["lunch"]["disabled_info"])
	}
	if got, want := disabled["disable_text"], "not sold now; available Mon 10:30-15:00 venue time"; got != want {
		t.Fatalf("disable_text = %q, want %q", got, want)
	}
	if disabled["source"] != itemScheduleSource {
		t.Fatalf("source = %#v", disabled["source"])
	}
	if items["always"]["disabled_info"] != nil || items["late"]["disabled_info"] == nil {
		t.Fatalf("always = %#v, late = %#v", items["always"]["disabled_info"], items["late"]["disabled_info"])
	}

	if _, err := client.AssortmentByVenueSlug(context.Background(), "trishuli"); err != nil {
		t.Fatalf("second AssortmentByVenueSlug() error = %v", err)
	}
	if got := httpClient.calls["/trishuli/dynamic/"]; got != 1 {
		t.Fatalf("dynamic calls = %d, want 1 (the zone is cached per venue)", got)
	}
	if got := httpClient.calls["/trishuli/static"]; got != 0 {
		t.Fatalf("static calls = %d, want 0", got)
	}
}

func TestAssortmentKeepsItemsInsideTheirSaleWindow(t *testing.T) {
	cases := map[string]struct {
		now     string
		enabled []string
	}{
		"monday lunch in venue time":   {now: "2026-10-19 10:30", enabled: []string{"lunch", "always"}},
		"monday lunch end is excluded": {now: "2026-10-19 15:00", enabled: []string{"always"}},
		"overnight window before 0:00": {now: "2026-10-23 23:15", enabled: []string{"always", "late"}},
		"overnight window after 0:00":  {now: "2026-10-24 01:59", enabled: []string{"always", "late"}},
		"overnight window has ended":   {now: "2026-10-24 02:00", enabled: []string{"always"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client, _ := scheduleTestClient(t, map[string]string{
				"/trishuli/assortment": scheduleTestAssortment,
				"/trishuli/static":     scheduleTestStatic,
			}, tc.now)
			payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
			if err != nil {
				t.Fatalf("AssortmentByVenueSlug() error = %v", err)
			}
			enabled := []string{}
			for _, id := range []string{"lunch", "always", "late"} {
				if scheduleTestItems(t, payload)[id]["disabled_info"] == nil {
					enabled = append(enabled, id)
				}
			}
			if strings.Join(enabled, ",") != strings.Join(tc.enabled, ",") {
				t.Fatalf("enabled = %v, want %v", enabled, tc.enabled)
			}
		})
	}
}

func TestAssortmentSkipsTimezoneLookupWithoutRestrictedItems(t *testing.T) {
	client, httpClient := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment": `{"items":[{"id":"always","disabled_info":null,
			"available_times":[{"days":[1,2,3,4,5,6,7],"start_time":0,"end_time":86400000}]},
			{"id":"plain","disabled_info":null}]}`,
		"/trishuli/static": scheduleTestStatic,
	}, "2026-10-22 15:51")
	if _, err := client.AssortmentByVenueSlug(context.Background(), "trishuli"); err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	if got := httpClient.calls["/trishuli/static"] + httpClient.calls["/trishuli/dynamic/"]; got != 0 {
		t.Fatalf("venue page calls = %d, want 0", got)
	}
}

func TestAssortmentUsesTheZoneOfAVenuePageAlreadyLoaded(t *testing.T) {
	client, httpClient := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment":             scheduleTestAssortment,
		"/6347f894ef53042e6c3eaaa6/static": scheduleTestStatic,
	}, "2026-10-22 15:51")
	// A cart add by venue id loads the static page first; it names the slug.
	if _, err := client.VenuePageStatic(context.Background(), "6347f894ef53042e6c3eaaa6"); err != nil {
		t.Fatalf("VenuePageStatic() error = %v", err)
	}
	payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
	if err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	if scheduleTestItems(t, payload)["lunch"]["disabled_info"] == nil {
		t.Fatal("lunch was not disabled with the zone from the loaded static page")
	}
	if got := httpClient.calls["/trishuli/dynamic/"] + httpClient.calls["/trishuli/static"]; got != 0 {
		t.Fatalf("extra venue page calls = %d, want 0", got)
	}
}

func TestAssortmentFallsBackToTheStaticPageZone(t *testing.T) {
	client, httpClient := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment": scheduleTestAssortment,
		"/trishuli/dynamic/":   `{"venue":{"open_status":null}}`,
		"/trishuli/static":     scheduleTestStatic,
	}, "2026-10-22 15:51")
	payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
	if err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	if scheduleTestItems(t, payload)["lunch"]["disabled_info"] == nil {
		t.Fatal("lunch was not disabled with the static page zone")
	}
	if httpClient.calls["/trishuli/dynamic/"] != 1 || httpClient.calls["/trishuli/static"] != 1 {
		t.Fatalf("calls = %#v, want one dynamic and one static", httpClient.calls)
	}
}

func TestOffsetZoneExpires(t *testing.T) {
	client, httpClient := scheduleTestClient(t, map[string]string{
		"/trishuli/dynamic/": scheduleTestDynamic,
	}, "2026-10-22 15:51")
	if _, ok := client.venueLocation(context.Background(), "trishuli"); !ok {
		t.Fatal("venueLocation() found no zone")
	}
	later := client.clock().Add(offsetZoneTTL + time.Minute)
	client.now = func() time.Time { return later }
	if _, ok := client.venueLocation(context.Background(), "trishuli"); !ok {
		t.Fatal("venueLocation() found no zone after expiry")
	}
	if got := httpClient.calls["/trishuli/dynamic/"]; got != 2 {
		t.Fatalf("dynamic calls = %d, want 2 (the offset is re-read after it expires)", got)
	}
}

func TestAssortmentLeavesItemsUntouchedWhenTimezoneIsUnknown(t *testing.T) {
	client, _ := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment": scheduleTestAssortment,
		"/trishuli/static":     `{"venue":{}}`,
	}, "2026-10-22 15:51")
	payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
	if err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	for id, item := range scheduleTestItems(t, payload) {
		if item["disabled_info"] != nil {
			t.Fatalf("%s disabled_info = %#v, want nil", id, item["disabled_info"])
		}
	}
}

func TestAssortmentKeepsWoltDisabledInfo(t *testing.T) {
	client, _ := scheduleTestClient(t, map[string]string{
		"/trishuli/assortment": `{"items":[{"id":"lunch","disabled_info":{"disable_text":"Sold out"},
			"available_times":[{"days":[1],"start_time":37800000,"end_time":54000000}]}]}`,
		"/trishuli/static": scheduleTestStatic,
	}, "2026-10-22 15:51")
	payload, err := client.AssortmentByVenueSlug(context.Background(), "trishuli")
	if err != nil {
		t.Fatalf("AssortmentByVenueSlug() error = %v", err)
	}
	disabled := scheduleTestItems(t, payload)["lunch"]["disabled_info"].(map[string]any)
	if disabled["disable_text"] != "Sold out" {
		t.Fatalf("disabled_info = %#v, want Wolt's own", disabled)
	}
}
