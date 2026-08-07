package searchload

import (
	"context"
	"net/http"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type searchProbe struct {
	authCalls []woltgateway.AuthContext
}

func (p *searchProbe) SearchItems(
	_ context.Context,
	_ domain.Location,
	_ string,
	_ int,
	auth woltgateway.AuthContext,
) (map[string]any, error) {
	p.authCalls = append(p.authCalls, auth)
	if auth.HasCredentials() {
		return nil, &woltgateway.UpstreamRequestError{Method: http.MethodPost, StatusCode: http.StatusUnauthorized}
	}
	return map[string]any{"sections": []any{}}, nil
}

func TestRequestItemsFallsBackToAnonymousOnlyForUnauthorizedOptionalAuth(t *testing.T) {
	probe := &searchProbe{}
	payload, err := RequestItems(
		context.Background(),
		probe,
		domain.Location{Lat: 10.25, Lon: 20.5},
		"query",
		20,
		woltgateway.AuthContext{WToken: "stale-token"},
	)
	if err != nil {
		t.Fatalf("RequestItems() error = %v", err)
	}
	if payload == nil || len(probe.authCalls) != 2 {
		t.Fatalf("payload = %#v, calls = %d", payload, len(probe.authCalls))
	}
	if !probe.authCalls[0].HasCredentials() || probe.authCalls[1].HasCredentials() {
		t.Fatalf("auth sequence = %#v", probe.authCalls)
	}
}
