package catalogload

import (
	"context"
	"errors"
	"net/http"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// RetryPolicy controls retries for transient catalog requests. A zero value
// performs one request without delay.
type RetryPolicy struct {
	Attempts int
	Delay    time.Duration
	// MaxDelay bounds a single retry wait. When Wolt asks for a longer
	// Retry-After, the request returns the original error instead of retrying
	// before the server-provided window. Zero leaves the wait unbounded.
	MaxDelay time.Duration
	wait     func(context.Context, time.Duration) error
}

// RequestPayload runs one catalog request with bounded transient retries.
// Credentialed requests fall back to anonymous access only after an
// unauthorized response; rate limits and temporary failures never change the
// authentication context.
func RequestPayload(
	ctx context.Context,
	auth woltgateway.AuthContext,
	retry RetryPolicy,
	request func(context.Context, woltgateway.AuthContext) (map[string]any, error),
) (map[string]any, error) {
	payload, err := requestPayloadWithRetry(ctx, auth, retry, request)
	if err == nil || !auth.HasCredentials() || !woltgateway.HasStatus(err, http.StatusUnauthorized) {
		return payload, err
	}
	return requestPayloadWithRetry(ctx, woltgateway.AuthContext{}, retry, request)
}

func requestPayloadWithRetry(
	ctx context.Context,
	auth woltgateway.AuthContext,
	retry RetryPolicy,
	request func(context.Context, woltgateway.AuthContext) (map[string]any, error),
) (map[string]any, error) {
	attempts := retry.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		payload, err := request(ctx, auth)
		if err == nil {
			return payload, nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		lastErr = err
		if !shouldRetry(err) || attempt+1 == attempts {
			break
		}
		delay, withinBudget := retryDelay(err, retry)
		if !withinBudget {
			break
		}
		if err := waitForRetry(ctx, retry, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func retryDelay(err error, retry RetryPolicy) (time.Duration, bool) {
	delay := retry.Delay
	var upstream *woltgateway.UpstreamRequestError
	if errors.As(err, &upstream) && upstream.RetryAfter > delay {
		delay = upstream.RetryAfter
	}
	if retry.MaxDelay > 0 && delay > retry.MaxDelay {
		return 0, false
	}
	return delay, true
}

func waitForRetry(ctx context.Context, retry RetryPolicy, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if retry.wait != nil {
		return retry.wait(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	var upstream *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstream) {
		return true
	}
	return upstream.StatusCode == 0 ||
		upstream.StatusCode == 429 ||
		upstream.StatusCode >= 500
}
