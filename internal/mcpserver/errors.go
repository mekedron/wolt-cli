package mcpserver

import (
	"errors"
	"fmt"
	"strings"
	"time"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// ToolErrorInfo is the stable, machine-readable error shape returned in
// CallToolResult.Meta["wolt_error"]. The human-facing Content remains the short
// Message only.
type ToolErrorInfo struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	RetryAfterMS int64  `json:"retry_after_ms"`
}

type classifiedToolError struct {
	info ToolErrorInfo
}

func (e *classifiedToolError) Error() string {
	return e.info.Message
}

func newToolError(code, message string, retryable bool, retryAfter time.Duration) error {
	retryAfterMS := int64(0)
	if retryAfter > 0 {
		retryAfterMS = int64((retryAfter + time.Millisecond - 1) / time.Millisecond)
	}
	return &classifiedToolError{info: ToolErrorInfo{
		Code:         code,
		Message:      compactText(message),
		Retryable:    retryable,
		RetryAfterMS: retryAfterMS,
	}}
}

// toolErr returns an error suitable for handing back from a tool handler.
// The SDK wraps it into a CallToolResult with IsError=true and the message in
// a TextContent block — and crucially, skips output-schema validation on
// errors, which means zero-valued Out structs are safe to return.
//
// Upstream failures are rewritten to short, LLM-friendly directives. Do not
// include UpstreamRequestError.Error() here: it contains request URLs and a
// response-body preview that is useful in debug logs but too noisy (and
// potentially sensitive) for an MCP response.
func toolErr(err error) error {
	if err == nil {
		return nil
	}
	var alreadyClassified *classifiedToolError
	if errors.As(err, &alreadyClassified) {
		return alreadyClassified
	}
	if errors.Is(err, ErrNotLoggedIn) {
		return newToolError(
			"AUTH_REQUIRED",
			"not logged in — run 'wolt login' in a terminal to sign in, then retry",
			false,
			0,
		)
	}
	var configFailure *profileConfigError
	if errors.As(err, &configFailure) {
		if errors.Is(configFailure.cause, configstore.ErrInvalidConfig) {
			return newToolError(
				"CONFIG_INVALID",
				"profile configuration is invalid; repair it or move it aside, then retry",
				false,
				0,
			)
		}
		return newToolError(
			"CONFIG_UNAVAILABLE",
			"profile configuration is unavailable; check the local config and retry",
			true,
			0,
		)
	}
	var refreshFailure *tokenRefreshError
	if errors.As(err, &refreshFailure) {
		var refreshUpstream *woltgateway.UpstreamRequestError
		if errors.As(refreshFailure.cause, &refreshUpstream) {
			if invalidUpstreamResponse(refreshFailure.cause, refreshUpstream) {
				return newToolError(
					"UPSTREAM_INVALID_RESPONSE",
					"Wolt returned an invalid session refresh response; retry later",
					true,
					refreshUpstream.RetryAfter,
				)
			}
			switch refreshUpstream.StatusCode {
			case 429:
				return newToolError(
					"SESSION_REFRESH_FAILED",
					"wolt session refresh was rate-limited; retry later",
					true,
					refreshUpstream.RetryAfter,
				)
			case 0, 408, 425, 500, 502, 503, 504:
				return newToolError(
					"SESSION_REFRESH_FAILED",
					"wolt session refresh failed because Wolt is temporarily unavailable; retry later",
					true,
					refreshUpstream.RetryAfter,
				)
			case 410:
				return classifyGoneUpstream(refreshUpstream)
			case 404, 405, 501:
				return newToolError(
					"UNSUPPORTED_ENDPOINT",
					"Wolt's session refresh endpoint is unavailable or unsupported; update wolt-cli and retry",
					false,
					0,
				)
			}
			if refreshUpstream.StatusCode > 0 {
				return newToolError(
					"SESSION_REFRESH_FAILED",
					"Wolt rejected the session refresh; run 'wolt login' in a terminal, then retry",
					false,
					0,
				)
			}
		}
		if errors.Is(refreshFailure.cause, woltgateway.ErrInvalidResponse) {
			return newToolError(
				"UPSTREAM_INVALID_RESPONSE",
				"Wolt returned an invalid session refresh response; retry later",
				true,
				0,
			)
		}
		return newToolError(
			"SESSION_REFRESH_FAILED",
			"wolt session refresh failed — run 'wolt login' in a terminal, then retry",
			false,
			0,
		)
	}
	var upstream *woltgateway.UpstreamRequestError
	if errors.As(err, &upstream) {
		if invalidUpstreamResponse(err, upstream) {
			return newToolError(
				"UPSTREAM_INVALID_RESPONSE",
				"Wolt returned an invalid response; retry later",
				true,
				upstream.RetryAfter,
			)
		}
		switch upstream.StatusCode {
		case 0:
			return newToolError(
				"UPSTREAM_NETWORK_ERROR",
				"could not reach Wolt; check the network connection and retry",
				true,
				0,
			)
		case 401:
			return newToolError(
				"AUTH_EXPIRED",
				"wolt session expired or missing — run 'wolt login' in a terminal to refresh, then retry",
				false,
				0,
			)
		case 403:
			return newToolError(
				"FORBIDDEN",
				"wolt rejected this request (HTTP 403); the current account or request is not allowed",
				false,
				0,
			)
		case 404:
			return newToolError(
				"NOT_FOUND",
				"the requested Wolt resource was not found (HTTP 404)",
				false,
				0,
			)
		case 405, 501:
			return newToolError(
				"UNSUPPORTED_ENDPOINT",
				"this Wolt API operation is unavailable or unsupported; update wolt-cli and retry",
				false,
				0,
			)
		case 410:
			return classifyGoneUpstream(upstream)
		case 429:
			message := "wolt is rate-limiting requests; try again in a few seconds"
			if upstream.RetryAfter > 0 {
				delay := upstream.RetryAfter.Round(time.Second)
				if delay < time.Second {
					delay = time.Second
				}
				message = fmt.Sprintf("wolt is rate-limiting requests; retry after %s", delay)
			}
			return newToolError("RATE_LIMITED", message, true, upstream.RetryAfter)
		case 408, 425, 500, 502, 503, 504:
			return newToolError(
				"UPSTREAM_TEMPORARY",
				fmt.Sprintf("wolt is temporarily unavailable (HTTP %d); retry later", upstream.StatusCode),
				true,
				upstream.RetryAfter,
			)
		}
		if upstream.StatusCode >= 400 && upstream.StatusCode < 500 {
			return newToolError(
				"UPSTREAM_REJECTED",
				fmt.Sprintf("wolt rejected the request (HTTP %d)", upstream.StatusCode),
				false,
				0,
			)
		}
		if upstream.StatusCode > 0 {
			return newToolError(
				"UPSTREAM_ERROR",
				fmt.Sprintf("wolt API request failed (HTTP %d)", upstream.StatusCode),
				false,
				0,
			)
		}
	}
	if errors.Is(err, woltgateway.ErrInvalidResponse) {
		return newToolError(
			"UPSTREAM_INVALID_RESPONSE",
			"Wolt returned an invalid response; retry later",
			true,
			0,
		)
	}
	return newToolError("TOOL_ERROR", err.Error(), false, 0)
}

func classifyGoneUpstream(upstream *woltgateway.UpstreamRequestError) error {
	if woltgateway.LooksLikeOutdatedClient(upstream.Body) {
		return newToolError(
			"CLIENT_OUTDATED",
			"Wolt rejected the configured client version; update wolt-cli or its upstream client headers",
			false,
			0,
		)
	}
	return newToolError(
		"UNSUPPORTED_ENDPOINT",
		"this Wolt resource or API operation is no longer supported (HTTP 410)",
		false,
		0,
	)
}

func invalidUpstreamResponse(err error, upstream *woltgateway.UpstreamRequestError) bool {
	return upstream != nil &&
		upstream.StatusCode >= 200 &&
		upstream.StatusCode < 300 &&
		errors.Is(err, woltgateway.ErrInvalidResponse)
}

func toolErrf(format string, args ...any) error {
	return toolErr(fmt.Errorf(format, args...))
}

func compactText(message string) string {
	const maxRunes = 400
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	return string(runes[:maxRunes-1]) + "…"
}
