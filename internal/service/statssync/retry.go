package statssync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// adjustablePacer enforces a minimum gap between upstream calls and grows
// that gap whenever a 429 is observed, settling on a rate Wolt will accept
// for the rest of the run. The pacer is run-scoped and shared between the
// catalog and detail phases so a 429 in the catalog phase also slows down
// detail fetching, which is where the bulk of the calls happen.
type adjustablePacer struct {
	mu        sync.Mutex
	base      time.Duration
	extra     time.Duration
	bump      time.Duration
	extraCap  time.Duration
	progress  io.Writer
	announced bool
}

func newAdjustablePacer(base, bump, cap time.Duration, progress io.Writer) *adjustablePacer {
	if base < 0 {
		base = 0
	}
	if bump <= 0 {
		bump = PacingBumpOn429
	}
	if cap < 0 {
		cap = 0
	}
	return &adjustablePacer{base: base, bump: bump, extraCap: cap, progress: progress}
}

// wait sleeps for the currently-tuned inter-call gap. sleep is the
// injectable sleeper so tests can record durations without blocking.
func (p *adjustablePacer) wait(ctx context.Context, sleep func(context.Context, time.Duration) error) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	d := p.base + p.extra
	p.mu.Unlock()
	if d <= 0 {
		return nil
	}
	return sleep(ctx, d)
}

// noteRateLimited bumps the extra delay applied to every subsequent wait.
// Returns the new total per-call delay so callers can log it. A no-op when
// already at the cap.
func (p *adjustablePacer) noteRateLimited() time.Duration {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.extraCap > 0 && p.extra >= p.extraCap {
		return p.base + p.extra
	}
	p.extra += p.bump
	if p.extraCap > 0 && p.extra > p.extraCap {
		p.extra = p.extraCap
	}
	if !p.announced && p.progress != nil {
		writeDetail(p.progress, "  pacing adjusted to %s per call after first 429", (p.base + p.extra).Round(time.Millisecond))
		p.announced = true
	}
	return p.base + p.extra
}

func (p *adjustablePacer) current() time.Duration {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.base + p.extra
}

// backoffPolicy controls how rate-limited upstream calls are retried.
// MaxAttempts counts the initial try, so MaxAttempts=1 disables retries.
type backoffPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// defaultBackoff balances "let a 1-minute throttle window pass" against
// "don't keep the user waiting forever on a permanently broken endpoint".
// 6 attempts with 2s exponential base caps total wait at ~62s before we
// surface a resumable error.
var defaultBackoff = backoffPolicy{
	MaxAttempts: 6,
	BaseDelay:   2 * time.Second,
	MaxDelay:    60 * time.Second,
}

// callWithBackoff runs fn with exponential backoff on rate-limited upstream
// errors (HTTP 429 / 503), honoring Retry-After when the server provides it.
// label is interpolated into progress lines so the user can tell which call
// is being throttled. pacer, when non-nil, is bumped on each 429 so the
// caller's steady-state inter-call delay grows to a sustainable rate.
func callWithBackoff(
	ctx context.Context,
	fn func(context.Context) (map[string]any, error),
	sleep func(context.Context, time.Duration) error,
	policy backoffPolicy,
	pacer *adjustablePacer,
	progress io.Writer,
	label string,
) (map[string]any, error) {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if sleep == nil {
		sleep = sleepCtx
	}
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		payload, err := fn(ctx)
		if err == nil {
			if attempt > 1 {
				writeDetail(progress, "  %s recovered on retry %d/%d", label, attempt-1, policy.MaxAttempts-1)
			}
			return payload, nil
		}
		if !isRateLimited(err) {
			return nil, err
		}
		lastErr = err
		pacer.noteRateLimited()
		if attempt == policy.MaxAttempts {
			break
		}
		delay := backoffDelay(err, attempt, policy)
		writeDetail(progress, "  %s rate-limited (HTTP %d); sleeping %s before retry %d/%d",
			label, statusOf(err), delay.Round(time.Second), attempt, policy.MaxAttempts-1)
		if err := sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s rate-limited after %d attempts: %w", label, policy.MaxAttempts, lastErr)
}

func isRateLimited(err error) bool {
	switch statusOf(err) {
	case 429, 503:
		return true
	default:
		return false
	}
}

// callWithAuthAndBackoff wraps callWithBackoff with one mid-call auth
// refresh: if the first cycle ends in 401, refreshAuth is invoked, the
// underlying closure picks up the rotated tokens (the caller dereferences
// a *AuthContext inside fn), and the full backoff cycle is re-run once
// with the fresh credentials. refreshAuth may be nil — in that case 401
// surfaces unchanged.
func callWithAuthAndBackoff(
	ctx context.Context,
	fn func(context.Context) (map[string]any, error),
	refreshAuth func(context.Context) error,
	sleep func(context.Context, time.Duration) error,
	policy backoffPolicy,
	pacer *adjustablePacer,
	progress io.Writer,
	label string,
) (map[string]any, error) {
	payload, err := callWithBackoff(ctx, fn, sleep, policy, pacer, progress, label)
	if err == nil {
		return payload, nil
	}
	if !woltgateway.HasStatus(err, http.StatusUnauthorized) || refreshAuth == nil {
		return nil, err
	}
	writeDetail(progress, "  %s received HTTP 401; refreshing access token", label)
	if rerr := refreshAuth(ctx); rerr != nil {
		return nil, fmt.Errorf("%s: upstream 401 and auth refresh failed: %w (original: %v)", label, rerr, err)
	}
	return callWithBackoff(ctx, fn, sleep, policy, pacer, progress, label)
}

// buildRefreshHook returns a closure suitable for catalogParams.RefreshAuth
// / detailParams.RefreshAuth. Each invocation: swaps auth.RefreshToken via
// refresher, mutates *auth in place with the new access (+ rotated refresh)
// token, then notifies onRefreshed. Returns nil — meaning "no refresh path
// available" — when either refresher is nil or the supplied auth has no
// refresh token to use.
func buildRefreshHook(
	auth *woltgateway.AuthContext,
	refresher Refresher,
	onRefreshed func(woltgateway.AuthContext) error,
	progress io.Writer,
) func(context.Context) error {
	if refresher == nil || auth == nil || strings.TrimSpace(auth.RefreshToken) == "" {
		return nil
	}
	return func(ctx context.Context) error {
		rt := strings.TrimSpace(auth.RefreshToken)
		if rt == "" {
			return fmt.Errorf("no refresh token available")
		}
		result, err := refresher(ctx, rt, *auth)
		if err != nil {
			return err
		}
		newAccess := strings.TrimSpace(result.AccessToken)
		if newAccess == "" {
			return fmt.Errorf("%w: refresh response missing access token", woltgateway.ErrInvalidResponse)
		}
		auth.WToken = newAccess
		if newRefresh := strings.TrimSpace(result.RefreshToken); newRefresh != "" {
			auth.RefreshToken = newRefresh
		}
		if onRefreshed != nil {
			if persistErr := onRefreshed(*auth); persistErr != nil {
				writeDetail(
					progress,
					"  warning: persisting refreshed access token failed: %v",
					persistErr,
				)
			}
		}
		writeDetail(progress, "  access token refreshed mid-sync")
		return nil
	}
}

func statusOf(err error) int {
	var upstream *woltgateway.UpstreamRequestError
	if errors.As(err, &upstream) {
		return upstream.StatusCode
	}
	return 0
}

func backoffDelay(err error, attempt int, policy backoffPolicy) time.Duration {
	var upstream *woltgateway.UpstreamRequestError
	if errors.As(err, &upstream) && upstream.RetryAfter > 0 {
		d := upstream.RetryAfter
		if d > policy.MaxDelay {
			return policy.MaxDelay
		}
		return d
	}
	if attempt < 1 {
		attempt = 1
	}
	shift := uint(attempt - 1)
	if shift > 16 {
		shift = 16
	}
	d := policy.BaseDelay << shift
	if d <= 0 || d > policy.MaxDelay {
		d = policy.MaxDelay
	}
	return d
}
