package wolt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
)

const (
	defaultConsumerAPIURL   = "https://consumer-api.wolt.com/v1/pages/front"
	defaultSearchAPIURL     = "https://restaurant-api.wolt.com/v1/pages/search"
	defaultVenuePageAPIURL  = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/slug/"
	defaultVenueItemAPIURL  = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/"
	defaultRestaurantAPIURL = "https://restaurant-api.wolt.com/v3/venues/"
)

// ErrUpstream indicates Wolt API failure.
var ErrUpstream = errors.New("[Wolt] error when trying to get response from wolt api")

// HTTPClient is implemented by http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Endpoints stores upstream endpoint urls.
type Endpoints struct {
	ConsumerFront string
	SearchPage    string
	VenuePage     string
	VenueItem     string
	Restaurant    string
}

// Client queries Wolt public endpoints.
type Client struct {
	httpClient HTTPClient
	endpoints  Endpoints
	locale     string
}

// Option applies Client options.
type Option func(*Client)

// WithHTTPClient replaces default HTTP client.
func WithHTTPClient(httpClient HTTPClient) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithEndpoints replaces default endpoint set.
func WithEndpoints(endpoints Endpoints) Option {
	return func(c *Client) {
		c.endpoints = endpoints
	}
}

// WithLocale sets app-language header value.
func WithLocale(locale string) Option {
	return func(c *Client) {
		c.locale = locale
	}
}

// NewClient creates a production Wolt gateway client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		endpoints: Endpoints{
			ConsumerFront: defaultConsumerAPIURL,
			SearchPage:    defaultSearchAPIURL,
			VenuePage:     defaultVenuePageAPIURL,
			VenueItem:     defaultVenueItemAPIURL,
			Restaurant:    defaultRestaurantAPIURL,
		},
		locale: "en",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) headers(extra map[string]string) map[string]string {
	headers := map[string]string{"app-language": c.locale}
	for k, v := range extra {
		headers[k] = v
	}
	return headers
}

func (c *Client) doJSONRequest(ctx context.Context, method, rawURL string, params url.Values, body any, headers map[string]string) (map[string]any, error) {
	if len(params) > 0 {
		rawURL = rawURL + "?" + params.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, ErrUpstream
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	return payload, nil
}

func decodeAny[T any](value any) (T, error) {
	var out T
	payload, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) frontPage(ctx context.Context, location domain.Location) (map[string]any, error) {
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%f", location.Lat))
	params.Set("lon", fmt.Sprintf("%f", location.Lon))
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.ConsumerFront, params, nil, c.headers(nil))
}

// FrontPage returns the raw discovery page payload.
func (c *Client) FrontPage(ctx context.Context, location domain.Location) (map[string]any, error) {
	return c.frontPage(ctx, location)
}

// Sections returns discovery sections.
func (c *Client) Sections(ctx context.Context, location domain.Location) ([]domain.Section, error) {
	payload, err := c.frontPage(ctx, location)
	if err != nil {
		return nil, err
	}
	sectionsRaw, ok := payload["sections"]
	if !ok {
		return nil, fmt.Errorf("%w: missing sections", ErrUpstream)
	}
	sections, err := decodeAny[[]domain.Section](sectionsRaw)
	if err != nil {
		return nil, fmt.Errorf("decode sections: %w", err)
	}
	return sections, nil
}

// Items returns deduplicated discovery items containing venue metadata.
func (c *Client) Items(ctx context.Context, location domain.Location) ([]domain.Item, error) {
	sections, err := c.Sections(ctx, location)
	if err != nil {
		return nil, err
	}
	items := make([]domain.Item, 0)
	seen := map[string]struct{}{}
	for _, section := range sections {
		for _, item := range section.Items {
			if item.Venue == nil {
				continue
			}
			key := strings.Join([]string{item.TrackID, domain.NormalizeID(item.Venue.ID), item.Link.Target}, "|")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
	}
	return items, nil
}

func (c *Client) restaurant(ctx context.Context, venueID string) (*domain.Restaurant, error) {
	payload, err := c.doJSONRequest(ctx, http.MethodGet, c.endpoints.Restaurant+venueID, nil, nil, c.headers(nil))
	if err != nil {
		return nil, err
	}
	resultsRaw, ok := payload["results"]
	if !ok {
		return nil, fmt.Errorf("%w: missing results", ErrUpstream)
	}
	results, err := decodeAny[[]domain.Restaurant](resultsRaw)
	if err != nil {
		return nil, fmt.Errorf("decode restaurant: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%w: empty results", ErrUpstream)
	}
	return &results[0], nil
}

// RestaurantByID loads a detailed restaurant payload.
func (c *Client) RestaurantByID(ctx context.Context, venueID string) (*domain.Restaurant, error) {
	return c.restaurant(ctx, venueID)
}

// Search returns raw search endpoint payload.
func (c *Client) Search(ctx context.Context, location domain.Location, query string) (map[string]any, error) {
	body := map[string]any{
		"q":      query,
		"target": nil,
		"lat":    location.Lat,
		"lon":    location.Lon,
	}
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		c.endpoints.SearchPage,
		nil,
		body,
		c.headers(map[string]string{"Content-Type": "application/json"}),
	)
}

// VenuePageStatic returns venue static page payload.
func (c *Client) VenuePageStatic(ctx context.Context, slug string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenuePage+slug+"/static", nil, nil, c.headers(nil))
}

// VenuePageDynamic returns venue dynamic page payload.
func (c *Client) VenuePageDynamic(ctx context.Context, slug string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenuePage+slug+"/dynamic", nil, nil, c.headers(nil))
}

// VenueItemPage returns single item payload from a venue.
func (c *Client) VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenueItem+venueID+"/item/"+itemID, nil, nil, c.headers(nil))
}

// ItemBySlug resolves a discovery item by venue slug.
func (c *Client) ItemBySlug(ctx context.Context, location domain.Location, slug string) (*domain.Item, error) {
	items, err := c.Items(ctx, location)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Venue != nil && item.Venue.Slug == slug {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, nil
}
