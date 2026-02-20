package wolt

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
)

type captureHTTPClient struct {
	request      *http.Request
	requestBody  string
	statusCode   int
	responseBody string
}

func (c *captureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.request = req
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		c.requestBody = string(body)
	}
	statusCode := c.statusCode
	if statusCode == 0 {
		statusCode = 200
	}
	responseBody := c.responseBody
	if strings.TrimSpace(responseBody) == "" {
		responseBody = `{"results":{}}`
	}
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(responseBody)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestPaymentMethodsAddsPlatformHeaders(t *testing.T) {
	httpClient := &captureHTTPClient{}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			PaymentMethods: "https://example.test/v3/user/me/payment_methods",
		}),
	)

	_, err := client.PaymentMethods(context.Background(), AuthContext{WToken: "jwt-token"})
	if err != nil {
		t.Fatalf("payment methods returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected request to be captured")
	}

	headers := httpClient.request.Header
	if got := headers.Get("Authorization"); got != "Bearer jwt-token" {
		t.Fatalf("expected authorization bearer token, got %q", got)
	}
	if got := headers.Get("platform"); got != defaultPlatformHeader {
		t.Fatalf("expected platform header %q, got %q", defaultPlatformHeader, got)
	}
	if got := headers.Get("client-version"); got != defaultClientVersionHeader {
		t.Fatalf("expected client-version header %q, got %q", defaultClientVersionHeader, got)
	}
	if got := headers.Get("clientversionnumber"); got != defaultClientVersionHeader {
		t.Fatalf("expected clientversionnumber header %q, got %q", defaultClientVersionHeader, got)
	}
	if got := headers.Get("w-wolt-session-id"); got != defaultSessionIDHeader {
		t.Fatalf("expected w-wolt-session-id header %q, got %q", defaultSessionIDHeader, got)
	}
	if got := strings.TrimSpace(headers.Get("x-wolt-web-clientid")); got == "" {
		t.Fatal("expected non-empty x-wolt-web-clientid header")
	}
	if got := headers.Get("app-language"); got != "en" {
		t.Fatalf("expected app-language header %q, got %q", "en", got)
	}
}

func TestRefreshAccessTokenUsesFormBodyAndCookies(t *testing.T) {
	httpClient := &captureHTTPClient{
		responseBody: `{"access_token":"new-token","refresh_token":"new-refresh","expires_in":1800}`,
	}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			AccessToken: "https://example.test/v1/wauth2/access_token",
		}),
	)

	result, err := client.RefreshAccessToken(
		context.Background(),
		"refresh-token-1",
		AuthContext{Cookies: []string{"__wrtoken=refresh-token-1", "foo=bar"}},
	)
	if err != nil {
		t.Fatalf("refresh access token returned error: %v", err)
	}
	if result.AccessToken != "new-token" {
		t.Fatalf("expected access token new-token, got %q", result.AccessToken)
	}
	if result.RefreshToken != "new-refresh" {
		t.Fatalf("expected refresh token new-refresh, got %q", result.RefreshToken)
	}
	if result.ExpiresIn != 1800 {
		t.Fatalf("expected expires_in 1800, got %d", result.ExpiresIn)
	}
	if httpClient.request == nil {
		t.Fatal("expected request to be captured")
	}
	if got := httpClient.request.Method; got != http.MethodPost {
		t.Fatalf("expected POST request, got %s", got)
	}
	if got := httpClient.request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("expected form content-type, got %q", got)
	}
	if got := httpClient.request.Header.Get("Cookie"); got != "__wrtoken=refresh-token-1; foo=bar" {
		t.Fatalf("expected cookie header to be forwarded, got %q", got)
	}
	if strings.Contains(strings.ToLower(httpClient.requestBody), "access_token") {
		t.Fatalf("did not expect access token field in request body, got %q", httpClient.requestBody)
	}
	if !strings.Contains(httpClient.requestBody, "grant_type=refresh_token") {
		t.Fatalf("expected grant_type in request body, got %q", httpClient.requestBody)
	}
	if !strings.Contains(httpClient.requestBody, "refresh_token=refresh-token-1") {
		t.Fatalf("expected refresh_token in request body, got %q", httpClient.requestBody)
	}
}

func TestPaymentMethodsProfileSetsQueryAndHeaders(t *testing.T) {
	httpClient := &captureHTTPClient{}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			PaymentProfile: "https://example.test/v1/payment-methods/profile",
		}),
	)

	_, err := client.PaymentMethodsProfile(
		context.Background(),
		AuthContext{WToken: "jwt-token"},
		PaymentMethodsProfileOptions{
			Country: "fin",
		},
	)
	if err != nil {
		t.Fatalf("payment methods profile returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected request to be captured")
	}
	if got := httpClient.request.Header.Get("Authorization"); got != "Bearer jwt-token" {
		t.Fatalf("expected authorization bearer token, got %q", got)
	}
	values := httpClient.request.URL.Query()
	if values.Get("country") != "FIN" {
		t.Fatalf("expected country FIN, got %q", values.Get("country"))
	}
	if values.Get("is_ftu") != "false" {
		t.Fatalf("expected is_ftu=false, got %q", values.Get("is_ftu"))
	}
	availableRaw := values.Get("available_methods")
	if strings.TrimSpace(availableRaw) == "" {
		t.Fatal("expected non-empty available_methods query param")
	}
	decoded, err := url.QueryUnescape(availableRaw)
	if err != nil {
		t.Fatalf("decode available_methods query: %v", err)
	}
	if !strings.Contains(decoded, "card") {
		t.Fatalf("expected card in available_methods, got %q", decoded)
	}
}

func TestDeleteBasketsPostsBulkDeletePayload(t *testing.T) {
	httpClient := &captureHTTPClient{responseBody: `null`}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			BasketBulkDelete: "https://example.test/order-xp/v1/baskets/bulk/delete",
		}),
	)

	_, err := client.DeleteBaskets(context.Background(), []string{"basket-1", " basket-2 "}, AuthContext{WToken: "jwt-token"})
	if err != nil {
		t.Fatalf("delete baskets returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected request to be captured")
	}
	if got := httpClient.request.Method; got != http.MethodPost {
		t.Fatalf("expected POST request, got %s", got)
	}
	if got := httpClient.request.URL.String(); got != "https://example.test/order-xp/v1/baskets/bulk/delete" {
		t.Fatalf("unexpected url: %s", got)
	}
	if got := httpClient.request.Header.Get("Authorization"); got != "Bearer jwt-token" {
		t.Fatalf("expected authorization header, got %q", got)
	}
	if got := httpClient.request.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
	if strings.TrimSpace(httpClient.requestBody) != `{"ids":["basket-1","basket-2"]}` {
		t.Fatalf("unexpected request body: %s", httpClient.requestBody)
	}
}

func TestFavoriteVenuesUsesLocationQuery(t *testing.T) {
	httpClient := &captureHTTPClient{}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			FavoritesPage: "https://example.test/v1/pages/venue-list/profile/favourites",
		}),
	)

	_, err := client.FavoriteVenues(context.Background(), domain.Location{Lat: 60.14889, Lon: 24.6911577}, AuthContext{WToken: "jwt-token"})
	if err != nil {
		t.Fatalf("favorite venues returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected request to be captured")
	}
	if got := httpClient.request.Method; got != http.MethodGet {
		t.Fatalf("expected GET request, got %s", got)
	}
	if got := httpClient.request.Header.Get("Authorization"); got != "Bearer jwt-token" {
		t.Fatalf("expected authorization header, got %q", got)
	}
	values := httpClient.request.URL.Query()
	if values.Get("lat") == "" || values.Get("lon") == "" {
		t.Fatalf("expected lat/lon query params, got %q", httpClient.request.URL.String())
	}
}

func TestFavoriteVenueMutationsUseExpectedMethods(t *testing.T) {
	httpClient := &captureHTTPClient{}
	client := NewClient(
		WithHTTPClient(httpClient),
		WithEndpoints(Endpoints{
			FavoriteVenue: "https://example.test/v3/venues/favourites",
		}),
	)

	_, err := client.FavoriteVenueAdd(context.Background(), "venue-1", AuthContext{WToken: "jwt-token"})
	if err != nil {
		t.Fatalf("favorite venue add returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected add request to be captured")
	}
	if got := httpClient.request.Method; got != http.MethodPut {
		t.Fatalf("expected PUT request, got %s", got)
	}
	if got := httpClient.request.URL.String(); got != "https://example.test/v3/venues/favourites/venue-1" {
		t.Fatalf("unexpected add URL: %s", got)
	}

	_, err = client.FavoriteVenueRemove(context.Background(), "venue-1", AuthContext{WToken: "jwt-token"})
	if err != nil {
		t.Fatalf("favorite venue remove returned error: %v", err)
	}
	if httpClient.request == nil {
		t.Fatal("expected remove request to be captured")
	}
	if got := httpClient.request.Method; got != http.MethodDelete {
		t.Fatalf("expected DELETE request, got %s", got)
	}
	if got := httpClient.request.URL.String(); got != "https://example.test/v3/venues/favourites/venue-1" {
		t.Fatalf("unexpected remove URL: %s", got)
	}
}
