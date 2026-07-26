package venueresolve

import (
	"context"
	"errors"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type pageStub struct {
	static  func(context.Context, string) (map[string]any, error)
	dynamic func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error)
}

func (stub pageStub) VenuePageStatic(ctx context.Context, reference string) (map[string]any, error) {
	return stub.static(ctx, reference)
}

func (stub pageStub) VenuePageDynamic(
	ctx context.Context,
	reference string,
	options woltgateway.VenuePageDynamicOptions,
) (map[string]any, error) {
	return stub.dynamic(ctx, reference, options)
}

func TestResolveNeverCopiesSlugIntoID(t *testing.T) {
	stub := pageStub{
		static: func(context.Context, string) (map[string]any, error) {
			return nil, errors.New("static unavailable")
		},
		dynamic: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			return nil, errors.New("dynamic unavailable")
		},
	}
	result := Resolve(context.Background(), stub, "example-market", Options{})
	if result.Slug != "example-market" || result.ID != "" {
		t.Fatalf("unexpected unresolved identity: %+v", result)
	}
}

func TestResolveRejectsThirdPartyURLWithoutCallingWolt(t *testing.T) {
	calls := 0
	stub := pageStub{
		static: func(context.Context, string) (map[string]any, error) {
			calls++
			return nil, nil
		},
		dynamic: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			calls++
			return nil, nil
		},
	}

	result := Resolve(
		context.Background(),
		stub,
		"https://example.com/en/example/venue/example-market",
		Options{},
	)
	if result.ID != "" || result.Slug != "" || calls != 0 {
		t.Fatalf("third-party URL resolved or reached Wolt: result=%+v calls=%d", result, calls)
	}
}

func TestResolveCombinesCanonicalIdentity(t *testing.T) {
	const venueID = "0123456789abcdef01234567"
	stub := pageStub{
		static: func(_ context.Context, reference string) (map[string]any, error) {
			return map[string]any{"venue": map[string]any{"id": venueID, "slug": reference}}, nil
		},
		dynamic: func(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error) {
			t.Fatal("dynamic endpoint should not be called after complete static resolution")
			return nil, nil
		},
	}
	result := Resolve(context.Background(), stub, "example-market", Options{})
	if result.ID != venueID || result.Slug != "example-market" || result.StaticPayload == nil {
		t.Fatalf("unexpected resolved identity: %+v", result)
	}
}

func TestRequestDynamicRetriesUnauthorizedCredentialsAnonymously(t *testing.T) {
	calls := 0
	stub := pageStub{
		static: func(context.Context, string) (map[string]any, error) { return nil, nil },
		dynamic: func(
			_ context.Context,
			_ string,
			options woltgateway.VenuePageDynamicOptions,
		) (map[string]any, error) {
			calls++
			if calls == 1 {
				if !options.Auth.HasCredentials() {
					t.Fatal("first request should retain credentials")
				}
				return nil, &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			if options.Auth.HasCredentials() {
				t.Fatal("anonymous retry should clear credentials")
			}
			return map[string]any{"venue": map[string]any{"id": "0123456789abcdef01234567"}}, nil
		},
	}
	payload, err := RequestDynamic(
		context.Background(),
		stub,
		"example-market",
		woltgateway.VenuePageDynamicOptions{
			Auth: woltgateway.AuthContext{WToken: "test-token"},
		},
	)
	if err != nil || payload == nil || calls != 2 {
		t.Fatalf("RequestDynamic() payload=%v err=%v calls=%d", payload, err, calls)
	}
}
