package wolt

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxErrorBodyPreview = 800

// UpstreamRequestError carries HTTP context for failed upstream calls.
type UpstreamRequestError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
	Cause      error
	// RetryAfter, when > 0, is the server-suggested delay parsed from the
	// Retry-After response header. Callers performing batch work (e.g. the
	// stats sync) honor this hint before retrying on 429/503.
	RetryAfter time.Duration
}

func (e *UpstreamRequestError) Error() string {
	parts := []string{ErrUpstream.Error()}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	method := strings.TrimSpace(e.Method)
	url := strings.TrimSpace(e.URL)
	if method != "" || url != "" {
		parts = append(parts, strings.TrimSpace(method+" "+url))
	}
	if trimmed := compactBodyPreview(e.Body); trimmed != "" {
		parts = append(parts, fmt.Sprintf("body=%q", trimmed))
	}
	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause=%v", e.Cause))
	}
	return strings.Join(parts, "; ")
}

func (e *UpstreamRequestError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrUpstream}
	}
	return []error{ErrUpstream, e.Cause}
}

// HasStatus reports whether err wraps an upstream response with statusCode.
func HasStatus(err error, statusCode int) bool {
	var upstream *UpstreamRequestError
	return errors.As(err, &upstream) && upstream.StatusCode == statusCode
}

// parseRetryAfter reads the Retry-After header per RFC 7231 §7.1.3.
// Returns 0 when absent/unparseable. Accepts both delay-seconds and HTTP-date.
func parseRetryAfter(h http.Header, now time.Time) time.Duration {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		delta := when.Sub(now)
		if delta <= 0 {
			return 0
		}
		return delta
	}
	return 0
}

func compactBodyPreview(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")
	body = strings.Join(strings.Fields(body), " ")
	if len(body) > maxErrorBodyPreview {
		return body[:maxErrorBodyPreview] + "..."
	}
	return body
}

// LooksLikeOutdatedClient reports whether a Wolt error body explicitly asks
// the caller to update its client/application version.
func LooksLikeOutdatedClient(body string) bool {
	body = strings.ToLower(body)
	for _, marker := range []string{
		"client version",
		"client-version",
		"outdated client",
		"update your app",
		"update the app",
		"app version",
		"version of the app",
	} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}
