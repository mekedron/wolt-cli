package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
)

const defaultNominatimURL = "https://nominatim.openstreetmap.org/search"

// ErrLocationLookup is returned when geocoding fails.
var ErrLocationLookup = errors.New("error when trying to get location")

// Client resolves addresses to coordinates.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a location client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    defaultNominatimURL,
	}
}

// Get resolves an address using OSM Nominatim.
func (c *Client) Get(ctx context.Context, address string) (domain.Location, error) {
	query := url.Values{}
	query.Set("q", address)
	query.Set("format", "json")
	uri := c.baseURL + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return domain.Location{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "wolt-cli-go/1.0")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Location{}, fmt.Errorf("%w: %v", ErrLocationLookup, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.Location{}, ErrLocationLookup
	}

	var payload []domain.Location
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return domain.Location{}, fmt.Errorf("%w: %v", ErrLocationLookup, err)
	}
	if len(payload) == 0 {
		return domain.Location{}, ErrLocationLookup
	}
	return payload[0], nil
}
