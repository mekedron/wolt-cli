package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
)

const defaultNominatimURL = "https://nominatim.openstreetmap.org/search"

// ErrLocationLookup is returned when geocoding fails.
var ErrLocationLookup = errors.New("error when trying to get location")

// Client resolves addresses to coordinates.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

type coordinate float64

func (c *coordinate) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return fmt.Errorf("parse coordinate %q: %w", text, err)
		}
		*c = coordinate(value)
		return nil
	}

	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*c = coordinate(value)
		return nil
	}

	return fmt.Errorf("coordinate must be a string or number")
}

type nominatimResult struct {
	Lat coordinate `json:"lat"`
	Lon coordinate `json:"lon"`
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
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.Location{}, ErrLocationLookup
	}

	var payload []nominatimResult
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return domain.Location{}, fmt.Errorf("%w: %v", ErrLocationLookup, err)
	}
	if len(payload) == 0 {
		return domain.Location{}, ErrLocationLookup
	}
	return domain.Location{
		Lat: float64(payload[0].Lat),
		Lon: float64(payload[0].Lon),
	}, nil
}
