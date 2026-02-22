package wolt

import (
	"fmt"
	"strings"
)

const maxErrorBodyPreview = 800

// UpstreamRequestError carries HTTP context for failed upstream calls.
type UpstreamRequestError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
	Cause      error
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

func (e *UpstreamRequestError) Unwrap() error {
	return ErrUpstream
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
