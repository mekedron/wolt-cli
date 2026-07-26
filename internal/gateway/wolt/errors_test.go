package wolt

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestHasStatus(t *testing.T) {
	unauthorized := fmt.Errorf("request failed: %w", &UpstreamRequestError{
		StatusCode: http.StatusUnauthorized,
	})
	if !HasStatus(unauthorized, http.StatusUnauthorized) {
		t.Fatal("expected wrapped upstream status to match")
	}
	if HasStatus(unauthorized, http.StatusTooManyRequests) {
		t.Fatal("unexpected status match")
	}
	if HasStatus(errors.New("not an upstream response"), http.StatusUnauthorized) {
		t.Fatal("non-upstream errors must not match")
	}

	invalid := &UpstreamRequestError{StatusCode: http.StatusOK, Cause: ErrInvalidResponse}
	if !errors.Is(invalid, ErrUpstream) || !errors.Is(invalid, ErrInvalidResponse) {
		t.Fatal("UpstreamRequestError must preserve both its upstream sentinel and cause")
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 5, 21, 20, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"absent header", "", 0},
		{"seconds form", "12", 12 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-5", 0},
		{"http-date future", now.Add(30 * time.Second).UTC().Format(http.TimeFormat), 30 * time.Second},
		{"http-date past", now.Add(-time.Minute).UTC().Format(http.TimeFormat), 0},
		{"garbage", "soon-ish", 0},
		{"whitespace", "  15  ", 15 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			if c.header != "" {
				h.Set("Retry-After", c.header)
			}
			got := parseRetryAfter(h, now)
			if c.name == "http-date future" {
				// HTTP-date has 1-second resolution; accept ±1s drift.
				delta := got - c.want
				if delta < -time.Second || delta > time.Second {
					t.Fatalf("want ~%s, got %s", c.want, got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("want %s, got %s", c.want, got)
			}
		})
	}
}
