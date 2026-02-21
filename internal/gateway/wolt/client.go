package wolt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
)

const (
	defaultConsumerAPIURL       = "https://consumer-api.wolt.com/v1/pages/front"
	defaultSearchAPIURL         = "https://restaurant-api.wolt.com/v1/pages/search"
	defaultVenuePageAPIURL      = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/slug/"
	defaultVenuePageDynamicURL  = "https://consumer-api.wolt.com/order-xp/web/v1/venue/slug/"
	defaultAssortmentAPIURL     = "https://consumer-api.wolt.com/consumer-api/consumer-assortment/v1/venues/slug/"
	defaultVenueContentAPIURL   = "https://consumer-api.wolt.com/consumer-api/venue-content-api/v3/web/venue-content/slug/"
	defaultVenueItemAPIURL      = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/"
	defaultRestaurantAPIURL     = "https://restaurant-api.wolt.com/v3/venues/"
	defaultUserMeAPIURL         = "https://restaurant-api.wolt.com/v1/user/me"
	defaultPaymentMethodsAPIURL = "https://restaurant-api.wolt.com/v3/user/me/payment_methods"
	defaultPaymentProfileAPIURL = "https://payment-service.wolt.com/v1/payment-methods/profile"
	defaultAddressFieldsAPIURL  = "https://restaurant-api.wolt.com/v1/consumer-api/address-fields"
	defaultDeliveryInfoAPIURL   = "https://restaurant-api.wolt.com/v2/delivery/info"
	defaultOrderHistoryAPIURL   = "https://consumer-api.wolt.com/order-tracking-api/v1/order_history/"
	defaultFavoritesPageAPIURL  = "https://consumer-api.wolt.com/v1/pages/venue-list/profile/favourites"
	defaultFavoriteVenueAPIURL  = "https://restaurant-api.wolt.com/v3/venues/favourites"
	defaultBasketCountAPIURL    = "https://consumer-api.wolt.com/order-xp/v1/baskets/count"
	defaultBasketsPageAPIURL    = "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets"
	defaultBasketAPIURL         = "https://consumer-api.wolt.com/order-xp/v1/baskets"
	defaultBasketBulkDeleteURL  = "https://consumer-api.wolt.com/order-xp/v1/baskets/bulk/delete"
	defaultCheckoutAPIURL       = "https://consumer-api.wolt.com/order-xp/web/v2/pages/checkout"
	defaultAccessTokenAPIURL    = "https://authentication.wolt.com/v1/wauth2/access_token"
	defaultPlatformHeader       = "Web"
	defaultClientVersionHeader  = "1.16.79"
	defaultSessionIDHeader      = "no-analytics-consent"
)

var defaultPaymentProfileAvailableMethods = []string{
	"applepay",
	"card",
	"cash",
	"cibus",
	"edenred",
	"epassi",
	"invoice",
	"klarna",
	"mobilepay",
	"pay_on_delivery",
	"paypal",
	"paypay",
	"paypay_raw",
	"pluxee",
	"rakutenpay",
	"smartum",
	"swish",
	"szep_kh",
	"szep_mkb",
	"szep_otp",
	"updejeuner",
	"vipps",
	"googlepay",
	"gift_card",
	"meal_benefit",
}

// ErrUpstream indicates Wolt API failure.
var ErrUpstream = errors.New("[Wolt] error when trying to get response from wolt api")

// HTTPClient is implemented by http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Endpoints stores upstream endpoint urls.
type Endpoints struct {
	ConsumerFront    string
	SearchPage       string
	VenuePage        string
	VenuePageDynamic string
	Assortment       string
	VenueContent     string
	VenueItem        string
	Restaurant       string
	UserMe           string
	PaymentMethods   string
	PaymentProfile   string
	AddressFields    string
	DeliveryInfo     string
	OrderHistory     string
	FavoritesPage    string
	FavoriteVenue    string
	BasketCount      string
	BasketsPage      string
	Basket           string
	BasketBulkDelete string
	Checkout         string
	AccessToken      string
}

// Client queries Wolt public endpoints.
type Client struct {
	httpClient     HTTPClient
	endpoints      Endpoints
	locale         string
	webClientID    string
	minRequestGap  time.Duration
	requestWindowM sync.Mutex
	nextRequestAt  time.Time
	verboseOutput  io.Writer
	verboseOutputM sync.RWMutex
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

// WithRequestMinInterval limits request burst by enforcing minimum delay between upstream calls.
func WithRequestMinInterval(interval time.Duration) Option {
	return func(c *Client) {
		if interval < 0 {
			interval = 0
		}
		c.minRequestGap = interval
	}
}

// WithVerboseOutput enables per-request trace output for upstream HTTP calls.
func WithVerboseOutput(out io.Writer) Option {
	return func(c *Client) {
		c.SetVerboseOutput(out)
	}
}

// NewClient creates a production Wolt gateway client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		endpoints: Endpoints{
			ConsumerFront:    defaultConsumerAPIURL,
			SearchPage:       defaultSearchAPIURL,
			VenuePage:        defaultVenuePageAPIURL,
			VenuePageDynamic: defaultVenuePageDynamicURL,
			Assortment:       defaultAssortmentAPIURL,
			VenueContent:     defaultVenueContentAPIURL,
			VenueItem:        defaultVenueItemAPIURL,
			Restaurant:       defaultRestaurantAPIURL,
			UserMe:           defaultUserMeAPIURL,
			PaymentMethods:   defaultPaymentMethodsAPIURL,
			PaymentProfile:   defaultPaymentProfileAPIURL,
			AddressFields:    defaultAddressFieldsAPIURL,
			DeliveryInfo:     defaultDeliveryInfoAPIURL,
			OrderHistory:     defaultOrderHistoryAPIURL,
			FavoritesPage:    defaultFavoritesPageAPIURL,
			FavoriteVenue:    defaultFavoriteVenueAPIURL,
			BasketCount:      defaultBasketCountAPIURL,
			BasketsPage:      defaultBasketsPageAPIURL,
			Basket:           defaultBasketAPIURL,
			BasketBulkDelete: defaultBasketBulkDeleteURL,
			Checkout:         defaultCheckoutAPIURL,
			AccessToken:      defaultAccessTokenAPIURL,
		},
		locale:      "en",
		webClientID: generateWebClientID(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetVerboseOutput sets destination for verbose HTTP request trace lines.
func (c *Client) SetVerboseOutput(out io.Writer) {
	c.verboseOutputM.Lock()
	c.verboseOutput = out
	c.verboseOutputM.Unlock()
}

func (c *Client) headers(extra map[string]string, auth *AuthContext) map[string]string {
	headers := map[string]string{
		"app-language":        c.locale,
		"platform":            defaultPlatformHeader,
		"client-version":      defaultClientVersionHeader,
		"clientversionnumber": defaultClientVersionHeader,
		"w-wolt-session-id":   defaultSessionIDHeader,
	}
	if strings.TrimSpace(c.webClientID) != "" {
		headers["x-wolt-web-clientid"] = c.webClientID
	}
	if auth != nil {
		token := strings.TrimSpace(auth.WToken)
		if token != "" {
			headers["Authorization"] = "Bearer " + token
		}
		if len(auth.Cookies) > 0 {
			headers["Cookie"] = strings.Join(auth.Cookies, "; ")
		}
	}
	for k, v := range extra {
		headers[k] = v
	}
	return headers
}

func generateWebClientID() string {
	var payload [16]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	payload[6] = (payload[6] & 0x0f) | 0x40
	payload[8] = (payload[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", payload[0:4], payload[4:6], payload[6:8], payload[8:10], payload[10:16])
}

func (c *Client) doJSONRequest(ctx context.Context, method, rawURL string, params url.Values, body any, headers map[string]string) (map[string]any, error) {
	if len(params) > 0 {
		rawURL = rawURL + "?" + params.Encode()
	}

	var bodyReader io.Reader
	bodyBytes := 0
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyBytes = len(payload)
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if err := c.waitForRequestSlot(ctx); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	c.traceRequestStart(method, rawURL, bodyBytes)

	res, err := c.httpClient.Do(req)
	if err != nil {
		upstreamErr := &UpstreamRequestError{
			Method: method,
			URL:    rawURL,
			Cause:  err,
		}
		c.traceRequestDone(method, rawURL, 0, 0, startedAt, upstreamErr)
		return nil, upstreamErr
	}
	defer func() {
		_ = res.Body.Close()
	}()

	rawResponse, err := io.ReadAll(res.Body)
	if err != nil {
		upstreamErr := &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: res.StatusCode,
			Cause:      fmt.Errorf("read response body: %w", err),
		}
		c.traceRequestDone(method, rawURL, res.StatusCode, 0, startedAt, upstreamErr)
		return nil, upstreamErr
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		upstreamErr := &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: res.StatusCode,
			Body:       string(rawResponse),
		}
		c.traceRequestDone(method, rawURL, res.StatusCode, len(rawResponse), startedAt, upstreamErr)
		return nil, upstreamErr
	}
	if len(rawResponse) == 0 {
		c.traceRequestDone(method, rawURL, res.StatusCode, 0, startedAt, nil)
		return map[string]any{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(rawResponse, &payload); err != nil {
		upstreamErr := &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: res.StatusCode,
			Body:       string(rawResponse),
			Cause:      fmt.Errorf("decode response body: %w", err),
		}
		c.traceRequestDone(method, rawURL, res.StatusCode, len(rawResponse), startedAt, upstreamErr)
		return nil, upstreamErr
	}

	c.traceRequestDone(method, rawURL, res.StatusCode, len(rawResponse), startedAt, nil)
	return payload, nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method string,
	rawURL string,
	body io.Reader,
	headers map[string]string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if err := c.waitForRequestSlot(ctx); err != nil {
		return nil, err
	}

	bodyBytes := 0
	if sized, ok := body.(interface{ Len() int }); ok {
		bodyBytes = sized.Len()
	}
	startedAt := time.Now()
	c.traceRequestStart(method, rawURL, bodyBytes)

	res, err := c.httpClient.Do(req)
	if err != nil {
		upstreamErr := &UpstreamRequestError{
			Method: method,
			URL:    rawURL,
			Cause:  err,
		}
		c.traceRequestDone(method, rawURL, 0, 0, startedAt, upstreamErr)
		return nil, upstreamErr
	}
	c.traceRequestDone(method, rawURL, res.StatusCode, 0, startedAt, nil)
	return res, nil
}

func (c *Client) traceRequestStart(method, rawURL string, bodyBytes int) {
	if bodyBytes > 0 {
		c.tracef("[http] -> %s %s body_bytes=%d", method, rawURL, bodyBytes)
		return
	}
	c.tracef("[http] -> %s %s", method, rawURL)
}

func (c *Client) traceRequestDone(method, rawURL string, statusCode int, responseBytes int, startedAt time.Time, reqErr error) {
	duration := time.Since(startedAt).Round(time.Millisecond)
	if reqErr != nil {
		c.tracef("[http] <- %s %s error=%v duration=%s", method, rawURL, reqErr, duration)
		return
	}
	c.tracef(
		"[http] <- %s %s status=%d duration=%s bytes=%d",
		method,
		rawURL,
		statusCode,
		duration,
		responseBytes,
	)
}

func (c *Client) waitForRequestSlot(ctx context.Context) error {
	interval := c.minRequestGap
	if interval <= 0 {
		return nil
	}
	for {
		c.requestWindowM.Lock()
		wait := time.Until(c.nextRequestAt)
		if wait <= 0 {
			c.nextRequestAt = time.Now().Add(interval)
			c.requestWindowM.Unlock()
			return nil
		}
		c.requestWindowM.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *Client) tracef(format string, args ...any) {
	c.verboseOutputM.RLock()
	out := c.verboseOutput
	c.verboseOutputM.RUnlock()
	if out == nil {
		return
	}
	_, _ = fmt.Fprintf(out, format+"\n", args...)
}

func decodeResponsePayload(method string, rawURL string, statusCode int, rawResponse []byte) (map[string]any, error) {
	if len(rawResponse) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(rawResponse, &payload); err != nil {
		return nil, &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: statusCode,
			Body:       string(rawResponse),
			Cause:      fmt.Errorf("decode response body: %w", err),
		}
	}
	return payload, nil
}

func readResponseBody(res *http.Response, method string, rawURL string) ([]byte, error) {
	rawResponse, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: res.StatusCode,
			Cause:      fmt.Errorf("read response body: %w", err),
		}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &UpstreamRequestError{
			Method:     method,
			URL:        rawURL,
			StatusCode: res.StatusCode,
			Body:       string(rawResponse),
		}
	}
	return rawResponse, nil
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		for actualKey, rawValue := range payload {
			if !strings.EqualFold(strings.TrimSpace(actualKey), strings.TrimSpace(key)) {
				continue
			}
			if value, ok := rawValue.(string); ok {
				if token := strings.TrimSpace(value); token != "" {
					return token
				}
			}
		}
	}
	return ""
}

func payloadInt(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		for actualKey, rawValue := range payload {
			if !strings.EqualFold(strings.TrimSpace(actualKey), strings.TrimSpace(key)) {
				continue
			}
			switch value := rawValue.(type) {
			case float64:
				return int(value)
			case int:
				return value
			case int64:
				return int(value)
			case json.Number:
				if parsed, err := value.Int64(); err == nil {
					return int(parsed)
				}
			}
		}
	}
	return 0
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
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.ConsumerFront, params, nil, c.headers(nil, nil))
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
	payload, err := c.doJSONRequest(ctx, http.MethodGet, c.endpoints.Restaurant+venueID, nil, nil, c.headers(nil, nil))
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
		c.headers(map[string]string{"Content-Type": "application/json"}, nil),
	)
}

// VenuePageStatic returns venue static page payload.
func (c *Client) VenuePageStatic(ctx context.Context, slug string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenuePage+slug+"/static", nil, nil, c.headers(nil, nil))
}

// VenuePageDynamic returns venue dynamic page payload.
func (c *Client) VenuePageDynamic(ctx context.Context, slug string, options VenuePageDynamicOptions) (map[string]any, error) {
	endpoint := strings.TrimSpace(c.endpoints.VenuePageDynamic)
	if endpoint == "" {
		endpoint = defaultVenuePageDynamicURL
	}
	params := url.Values{}
	if options.Location != nil {
		params.Set("lat", strconv.FormatFloat(options.Location.Lat, 'f', 6, 64))
		params.Set("lon", strconv.FormatFloat(options.Location.Lon, 'f', 6, 64))
		selectedDeliveryMethod := strings.TrimSpace(options.SelectedDeliveryMethod)
		if selectedDeliveryMethod == "" {
			selectedDeliveryMethod = "homedelivery"
		}
		params.Set("selected_delivery_method", selectedDeliveryMethod)
	}
	return c.doJSONRequest(
		ctx,
		http.MethodGet,
		endpoint+slug+"/dynamic/",
		params,
		nil,
		c.headers(nil, &options.Auth),
	)
}

// AssortmentByVenueSlug returns full assortment payload for one venue slug.
func (c *Client) AssortmentByVenueSlug(ctx context.Context, slug string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.Assortment+slug+"/assortment", nil, nil, c.headers(nil, nil))
}

// AssortmentCategoryByVenueSlug returns one category assortment payload by category slug.
func (c *Client) AssortmentCategoryByVenueSlug(
	ctx context.Context,
	slug string,
	categorySlug string,
	language string,
	auth AuthContext,
) (map[string]any, error) {
	params := url.Values{}
	if lang := strings.TrimSpace(language); lang != "" {
		params.Set("language", lang)
	} else if lang = strings.TrimSpace(c.locale); lang != "" {
		params.Set("language", lang)
	}
	endpoint := c.endpoints.Assortment + slug + "/assortment/categories/slug/" + url.PathEscape(strings.TrimSpace(categorySlug))
	return c.doJSONRequest(ctx, http.MethodGet, endpoint, params, nil, c.headers(nil, &auth))
}

// AssortmentItemsByVenueSlug returns detailed item payload for selected assortment item ids.
func (c *Client) AssortmentItemsByVenueSlug(
	ctx context.Context,
	slug string,
	itemIDs []string,
	auth AuthContext,
) (map[string]any, error) {
	ids := make([]string, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		trimmedID := strings.TrimSpace(itemID)
		if trimmedID == "" {
			continue
		}
		ids = append(ids, trimmedID)
	}
	body := map[string]any{
		"item_ids": ids,
	}
	endpoint := c.endpoints.Assortment + slug + "/assortment/items"
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		endpoint,
		nil,
		body,
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// AssortmentItemsSearchByVenueSlug searches items inside one venue assortment.
func (c *Client) AssortmentItemsSearchByVenueSlug(
	ctx context.Context,
	slug string,
	query string,
	language string,
	auth AuthContext,
) (map[string]any, error) {
	params := url.Values{}
	if lang := strings.TrimSpace(language); lang != "" {
		params.Set("language", lang)
	} else if lang = strings.TrimSpace(c.locale); lang != "" {
		params.Set("language", lang)
	}
	body := map[string]any{
		"q": strings.TrimSpace(query),
	}
	endpoint := c.endpoints.Assortment + slug + "/assortment/items/search"
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		endpoint,
		params,
		body,
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// VenueContentByVenueSlug returns venue-content payload by venue slug.
func (c *Client) VenueContentByVenueSlug(ctx context.Context, slug string, nextPageToken string, auth AuthContext) (map[string]any, error) {
	params := url.Values{}
	if token := strings.TrimSpace(nextPageToken); token != "" {
		params.Set("next_page_token", token)
	}
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenueContent+slug, params, nil, c.headers(nil, &auth))
}

// VenueItemPage returns single item payload from a venue.
func (c *Client) VenueItemPage(ctx context.Context, venueID, itemID string) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.VenueItem+venueID+"/item/"+itemID, nil, nil, c.headers(nil, nil))
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

// UserMe returns authenticated user details.
func (c *Client) UserMe(ctx context.Context, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.UserMe, nil, nil, c.headers(nil, &auth))
}

// PaymentMethods returns payment methods available for the authenticated user.
func (c *Client) PaymentMethods(ctx context.Context, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.PaymentMethods, nil, nil, c.headers(nil, &auth))
}

// PaymentMethodsProfile returns checkout payment options shown in web profile.
func (c *Client) PaymentMethodsProfile(ctx context.Context, auth AuthContext, options PaymentMethodsProfileOptions) (map[string]any, error) {
	params := url.Values{}
	country := strings.ToUpper(strings.TrimSpace(options.Country))
	if country != "" {
		params.Set("country", country)
	}
	availableMethods := options.AvailableMethods
	if len(availableMethods) == 0 {
		availableMethods = defaultPaymentProfileAvailableMethods
	}
	params.Set("available_methods", strings.Join(availableMethods, ","))
	if options.IsFTU {
		params.Set("is_ftu", "true")
	} else {
		params.Set("is_ftu", "false")
	}
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.PaymentProfile, params, nil, c.headers(nil, &auth))
}

// AddressFields returns address form field metadata for a location.
func (c *Client) AddressFields(ctx context.Context, location domain.Location, language string, auth AuthContext) (map[string]any, error) {
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%f", location.Lat))
	params.Set("lon", fmt.Sprintf("%f", location.Lon))
	lang := strings.TrimSpace(language)
	if lang == "" {
		lang = c.locale
	}
	params.Set("language", lang)
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.AddressFields, params, nil, c.headers(nil, &auth))
}

// DeliveryInfoList returns saved delivery addresses from Wolt account.
func (c *Client) DeliveryInfoList(ctx context.Context, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.DeliveryInfo, nil, nil, c.headers(nil, &auth))
}

// DeliveryInfoCreate creates a new saved delivery address in Wolt account.
func (c *Client) DeliveryInfoCreate(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		c.endpoints.DeliveryInfo,
		nil,
		payload,
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// DeliveryInfoDelete removes a saved delivery address by id.
func (c *Client) DeliveryInfoDelete(ctx context.Context, addressID string, auth AuthContext) (map[string]any, error) {
	endpoint := strings.TrimRight(c.endpoints.DeliveryInfo, "/") + "/" + strings.TrimSpace(addressID)
	return c.doJSONRequest(ctx, http.MethodDelete, endpoint, nil, nil, c.headers(nil, &auth))
}

// OrderHistory returns paginated account order history.
func (c *Client) OrderHistory(ctx context.Context, auth AuthContext, options OrderHistoryOptions) (map[string]any, error) {
	params := url.Values{}
	limit := options.Limit
	if limit <= 0 {
		limit = 50
	}
	params.Set("limit", strconv.Itoa(limit))
	if pageToken := strings.TrimSpace(options.PageToken); pageToken != "" {
		params.Set("page_token", pageToken)
	}
	endpoint := strings.TrimRight(c.endpoints.OrderHistory, "/") + "/"
	return c.doJSONRequest(ctx, http.MethodGet, endpoint, params, nil, c.headers(nil, &auth))
}

// OrderHistoryPurchase returns detailed payload for one purchase id.
func (c *Client) OrderHistoryPurchase(ctx context.Context, purchaseID string, auth AuthContext) (map[string]any, error) {
	trimmedID := strings.TrimSpace(purchaseID)
	if trimmedID == "" {
		return nil, fmt.Errorf("purchase id is required")
	}
	params := url.Values{}
	params.Set("tips_use_percentage", "true")
	endpoint := strings.TrimRight(c.endpoints.OrderHistory, "/") + "/purchase/" + url.PathEscape(trimmedID)
	return c.doJSONRequest(ctx, http.MethodGet, endpoint, params, nil, c.headers(nil, &auth))
}

// FavoriteVenues returns account favourite venues list page payload.
func (c *Client) FavoriteVenues(ctx context.Context, location domain.Location, auth AuthContext) (map[string]any, error) {
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%f", location.Lat))
	params.Set("lon", fmt.Sprintf("%f", location.Lon))
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.FavoritesPage, params, nil, c.headers(nil, &auth))
}

// FavoriteVenueAdd marks one venue as favourite for the authenticated account.
func (c *Client) FavoriteVenueAdd(ctx context.Context, venueID string, auth AuthContext) (map[string]any, error) {
	trimmedID := strings.TrimSpace(venueID)
	if trimmedID == "" {
		return nil, fmt.Errorf("venue id is required")
	}
	endpoint := strings.TrimRight(c.endpoints.FavoriteVenue, "/") + "/" + trimmedID
	return c.doJSONRequest(ctx, http.MethodPut, endpoint, nil, nil, c.headers(nil, &auth))
}

// FavoriteVenueRemove removes one venue from favourites for the authenticated account.
func (c *Client) FavoriteVenueRemove(ctx context.Context, venueID string, auth AuthContext) (map[string]any, error) {
	trimmedID := strings.TrimSpace(venueID)
	if trimmedID == "" {
		return nil, fmt.Errorf("venue id is required")
	}
	endpoint := strings.TrimRight(c.endpoints.FavoriteVenue, "/") + "/" + trimmedID
	return c.doJSONRequest(ctx, http.MethodDelete, endpoint, nil, nil, c.headers(nil, &auth))
}

// BasketCount returns total basket count.
func (c *Client) BasketCount(ctx context.Context, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.BasketCount, nil, nil, c.headers(nil, &auth))
}

// BasketsPage returns full basket page payload and totals.
func (c *Client) BasketsPage(ctx context.Context, location domain.Location, auth AuthContext) (map[string]any, error) {
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%f", location.Lat))
	params.Set("lon", fmt.Sprintf("%f", location.Lon))
	return c.doJSONRequest(ctx, http.MethodGet, c.endpoints.BasketsPage, params, nil, c.headers(nil, &auth))
}

// AddToBasket adds a menu item payload to basket.
func (c *Client) AddToBasket(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		c.endpoints.Basket,
		nil,
		payload,
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// DeleteBaskets deletes one or many baskets by id.
func (c *Client) DeleteBaskets(ctx context.Context, basketIDs []string, auth AuthContext) (map[string]any, error) {
	ids := make([]any, 0, len(basketIDs))
	for _, basketID := range basketIDs {
		trimmed := strings.TrimSpace(basketID)
		if trimmed == "" {
			continue
		}
		ids = append(ids, trimmed)
	}
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		c.endpoints.BasketBulkDelete,
		nil,
		map[string]any{"ids": ids},
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// CheckoutPreview returns checkout projection payload.
func (c *Client) CheckoutPreview(ctx context.Context, payload map[string]any, auth AuthContext) (map[string]any, error) {
	return c.doJSONRequest(
		ctx,
		http.MethodPost,
		c.endpoints.Checkout,
		nil,
		payload,
		c.headers(map[string]string{"Content-Type": "application/json"}, &auth),
	)
}

// RefreshAccessToken exchanges refresh token for a new access token pair.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string, auth AuthContext) (TokenRefreshResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return TokenRefreshResult{}, fmt.Errorf("refresh token is required")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	headers := c.headers(map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
	}, nil)
	if len(auth.Cookies) > 0 {
		headers["Cookie"] = strings.Join(auth.Cookies, "; ")
	}

	res, err := c.doRequest(
		ctx,
		http.MethodPost,
		c.endpoints.AccessToken,
		strings.NewReader(form.Encode()),
		headers,
	)
	if err != nil {
		return TokenRefreshResult{}, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	rawResponse, err := readResponseBody(res, http.MethodPost, c.endpoints.AccessToken)
	if err != nil {
		return TokenRefreshResult{}, err
	}

	payload, err := decodeResponsePayload(http.MethodPost, c.endpoints.AccessToken, res.StatusCode, rawResponse)
	if err != nil {
		return TokenRefreshResult{}, err
	}
	accessToken := payloadString(payload, "access_token", "accessToken", "__wtoken")
	resolvedRefreshToken := payloadString(payload, "refresh_token", "refreshToken", "__wrtoken")
	if data, ok := payload["data"].(map[string]any); ok {
		if accessToken == "" {
			accessToken = payloadString(data, "access_token", "accessToken", "__wtoken")
		}
		if resolvedRefreshToken == "" {
			resolvedRefreshToken = payloadString(data, "refresh_token", "refreshToken", "__wrtoken")
		}
	}

	if strings.TrimSpace(accessToken) == "" {
		return TokenRefreshResult{}, fmt.Errorf("%w: refresh response missing access_token", ErrUpstream)
	}
	if strings.TrimSpace(resolvedRefreshToken) == "" {
		resolvedRefreshToken = refreshToken
	}
	expiresIn := payloadInt(payload, "expires_in", "expiresIn")
	if data, ok := payload["data"].(map[string]any); ok && expiresIn <= 0 {
		expiresIn = payloadInt(data, "expires_in", "expiresIn")
	}

	return TokenRefreshResult{
		AccessToken:  accessToken,
		RefreshToken: resolvedRefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}
