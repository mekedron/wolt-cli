package location

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestClient(t *testing.T, responseBody string, statusCode int) *Client {
	t.Helper()
	client := &Client{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("format") != "json" {
					t.Fatalf("expected format=json, got %q", req.URL.Query().Get("format"))
				}
				return &http.Response{
					StatusCode: statusCode,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(responseBody)),
				}, nil
			}),
		},
		baseURL: "https://nominatim.test/search",
	}
	return client
}

func TestGetParsesStringCoordinates(t *testing.T) {
	client := newTestClient(t, `[{"lat":"60.1699","lon":"24.9384"}]`, http.StatusOK)
	location, err := client.Get(context.Background(), "Helsinki")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if math.Abs(location.Lat-60.1699) > 1e-9 {
		t.Fatalf("expected lat 60.1699, got %f", location.Lat)
	}
	if math.Abs(location.Lon-24.9384) > 1e-9 {
		t.Fatalf("expected lon 24.9384, got %f", location.Lon)
	}
}

func TestGetReturnsLookupErrorOnInvalidCoordinates(t *testing.T) {
	client := newTestClient(t, `[{"lat":"not-a-number","lon":"24.9384"}]`, http.StatusOK)
	_, err := client.Get(context.Background(), "Helsinki")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrLocationLookup) {
		t.Fatalf("expected ErrLocationLookup, got %v", err)
	}
}
