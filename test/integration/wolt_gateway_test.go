package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type staticHTTPClient struct {
	routes map[string][]byte
}

func (c *staticHTTPClient) Do(req *http.Request) (*http.Response, error) {
	payload := c.routes[req.URL.Path]
	if payload == nil {
		payload = []byte(`{"error":"not found"}`)
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	statusCode := 200
	if strings.Contains(req.URL.Path, "error") {
		statusCode = 500
	}
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(payload)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func readFixture(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "wolt", filename)
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", filename, err)
	}
	return bytes
}

func TestItemsWithSuccessResponse(t *testing.T) {
	sectionsJSON := readFixture(t, "sections.json")
	client := woltgateway.NewClient(
		woltgateway.WithHTTPClient(&staticHTTPClient{routes: map[string][]byte{"/v1/pages/front": sectionsJSON}}),
		woltgateway.WithEndpoints(woltgateway.Endpoints{
			ConsumerFront: "https://example.test/v1/pages/front",
			SearchPage:    "https://example.test/unused/search",
			VenuePage:     "https://example.test/unused/venue/",
			VenueItem:     "https://example.test/unused/item/",
			Restaurant:    "https://example.test/unused/restaurants/",
		}),
	)

	items, err := client.Items(context.Background(), domain.Location{Lat: 10, Lon: 10})
	if err != nil {
		t.Fatalf("items returned error: %v", err)
	}
	if len(items) != 30 {
		t.Fatalf("expected 30 items, got %d", len(items))
	}
}

func TestRestaurantWithSuccessResponse(t *testing.T) {
	restaurantsJSON := readFixture(t, "restaurants.json")
	client := woltgateway.NewClient(
		woltgateway.WithHTTPClient(&staticHTTPClient{routes: map[string][]byte{"/v3/venues/test-venue": restaurantsJSON}}),
		woltgateway.WithEndpoints(woltgateway.Endpoints{
			ConsumerFront: "https://example.test/unused/front",
			SearchPage:    "https://example.test/unused/search",
			VenuePage:     "https://example.test/unused/venue/",
			VenueItem:     "https://example.test/unused/item/",
			Restaurant:    "https://example.test/v3/venues/",
		}),
	)

	restaurant, err := client.RestaurantByID(context.Background(), "test-venue")
	if err != nil {
		t.Fatalf("restaurant returned error: %v", err)
	}
	if restaurant.Name[0].Value != "N'Pizza " {
		t.Fatalf("expected restaurant name N'Pizza , got %q", restaurant.Name[0].Value)
	}
	if restaurant.City != "Kraków" {
		t.Fatalf("expected city Kraków, got %q", restaurant.City)
	}
	if restaurant.Country != "POL" {
		t.Fatalf("expected country POL, got %q", restaurant.Country)
	}
	if restaurant.Currency != "PLN" {
		t.Fatalf("expected currency PLN, got %q", restaurant.Currency)
	}
}
