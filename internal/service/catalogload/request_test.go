package catalogload

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestRequestPayloadRetriesWithoutAnonymousFallbackOrFinalWait(t *testing.T) {
	upstreamErr := &woltgateway.UpstreamRequestError{
		Method:     http.MethodPost,
		StatusCode: http.StatusTooManyRequests,
	}
	authCalls := []woltgateway.AuthContext{}
	waitCalls := 0
	retry := RetryPolicy{
		Attempts: 2,
		Delay:    time.Hour,
		wait: func(_ context.Context, _ time.Duration) error {
			waitCalls++
			return nil
		},
	}

	_, err := RequestPayload(
		context.Background(),
		woltgateway.AuthContext{WToken: "valid-token"},
		retry,
		func(_ context.Context, auth woltgateway.AuthContext) (map[string]any, error) {
			authCalls = append(authCalls, auth)
			return nil, upstreamErr
		},
	)
	if err != upstreamErr {
		t.Fatalf("error = %v, want original rate-limit error", err)
	}
	if len(authCalls) != 2 {
		t.Fatalf("calls = %d, want 2", len(authCalls))
	}
	for _, auth := range authCalls {
		if !auth.HasCredentials() {
			t.Fatal("rate limit incorrectly triggered an anonymous request")
		}
	}
	if waitCalls != 1 {
		t.Fatalf("wait calls = %d, want one wait only between attempts", waitCalls)
	}
}

func TestRequestPayloadHonorsRetryAfterWithinWaitBudget(t *testing.T) {
	upstreamErr := &woltgateway.UpstreamRequestError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 750 * time.Millisecond,
	}
	calls := 0
	var waited time.Duration
	payload, err := RequestPayload(
		context.Background(),
		woltgateway.AuthContext{},
		RetryPolicy{
			Attempts: 2,
			Delay:    120 * time.Millisecond,
			MaxDelay: time.Second,
			wait: func(_ context.Context, delay time.Duration) error {
				waited = delay
				return nil
			},
		},
		func(_ context.Context, _ woltgateway.AuthContext) (map[string]any, error) {
			calls++
			if calls == 1 {
				return nil, upstreamErr
			}
			return map[string]any{"ok": true}, nil
		},
	)
	if err != nil {
		t.Fatalf("RequestPayload() error = %v", err)
	}
	if payload["ok"] != true || calls != 2 {
		t.Fatalf("payload = %#v, calls = %d", payload, calls)
	}
	if waited != upstreamErr.RetryAfter {
		t.Fatalf("wait = %v, want Retry-After %v", waited, upstreamErr.RetryAfter)
	}
}

func TestRequestPayloadDoesNotRetryBeforeRetryAfterBeyondWaitBudget(t *testing.T) {
	upstreamErr := &woltgateway.UpstreamRequestError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 2 * time.Second,
	}
	calls := 0
	waited := false
	_, err := RequestPayload(
		context.Background(),
		woltgateway.AuthContext{},
		RetryPolicy{
			Attempts: 2,
			Delay:    120 * time.Millisecond,
			MaxDelay: time.Second,
			wait: func(context.Context, time.Duration) error {
				waited = true
				return nil
			},
		},
		func(_ context.Context, _ woltgateway.AuthContext) (map[string]any, error) {
			calls++
			return nil, upstreamErr
		},
	)
	if err != upstreamErr {
		t.Fatalf("error = %v, want original rate-limit error", err)
	}
	if calls != 1 || waited {
		t.Fatalf("calls = %d, waited = %t; want no premature retry", calls, waited)
	}
}

func TestRequestPayloadReturnsContextErrorWhenRequestWrapsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := RequestPayload(
		ctx,
		woltgateway.AuthContext{},
		RetryPolicy{},
		func(_ context.Context, _ woltgateway.AuthContext) (map[string]any, error) {
			cancel()
			return nil, &woltgateway.UpstreamRequestError{
				Method: http.MethodPost,
				Cause:  context.Canceled,
			}
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}
