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
