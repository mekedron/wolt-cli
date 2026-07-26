package wolt

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestInvalidSuccessResponsesPreserveGatewayErrorChain(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		wantSyntax   bool
	}{
		{name: "malformed JSON", responseBody: `{not-json`, wantSyntax: true},
		{name: "missing sections", responseBody: `{"items":[]}`},
		{name: "invalid sections", responseBody: `{"sections":"not-an-array"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient(
				WithHTTPClient(&captureHTTPClient{responseBody: test.responseBody}),
				WithEndpoints(Endpoints{ConsumerFront: "https://example.test/front"}),
			)
			_, err := client.Sections(context.Background(), domain.Location{Lat: 10, Lon: 20})
			if err == nil {
				t.Fatal("Sections succeeded with an invalid response")
			}
			if !errors.Is(err, ErrUpstream) {
				t.Fatalf("error %v does not preserve ErrUpstream", err)
			}
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error %v does not preserve ErrInvalidResponse", err)
			}
			if test.wantSyntax {
				var upstream *UpstreamRequestError
				if !errors.As(err, &upstream) || upstream.StatusCode != 200 {
					t.Fatalf("upstream error = %#v, want HTTP 200 context", upstream)
				}
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) {
					t.Fatalf("error %v does not preserve the JSON decode cause", err)
				}
			}
		})
	}
}

func TestRefreshResponseMissingAccessTokenPreservesGatewayErrorChain(t *testing.T) {
	client := NewClient(
		WithHTTPClient(&captureHTTPClient{
			responseBody: `{"refresh_token":"rotated-refresh"}`,
		}),
		WithEndpoints(Endpoints{AccessToken: "https://example.test/access-token"}),
	)
	_, err := client.RefreshAccessToken(context.Background(), "bootstrap-refresh", AuthContext{})
	if err == nil {
		t.Fatal("RefreshAccessToken succeeded without an access token")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error %v does not preserve ErrUpstream", err)
	}
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error %v does not preserve ErrInvalidResponse", err)
	}
}
