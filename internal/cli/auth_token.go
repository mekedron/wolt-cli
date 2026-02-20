package cli

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const tokenExtractMaxDepth = 6

var (
	jwtExactPattern       = regexp.MustCompile(`(?i)^[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+$`)
	jwtFindPattern        = regexp.MustCompile(`(?i)[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+`)
	tokenKVPattern        = regexp.MustCompile(`(?i)(?:accessToken|access_token|__wtoken|wtoken|idToken|id_token|token)\s*[:=]\s*["']?([a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+)`)
	refreshTokenKVPattern = regexp.MustCompile(`(?i)(?:refreshToken|refresh_token|__wrtoken|wrtoken|wrefresh_token|refresh)\s*[:=]\s*["']?([^"'\s;,&}]+)`)
)

func normalizeWToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	token := extractWToken(raw, 0, map[string]struct{}{})
	if token == "" {
		return raw
	}
	return token
}

func normalizeRefreshToken(raw string) string {
	raw = trimTokenWrapper(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	for _, decoded := range decodeCandidates(raw) {
		candidate := trimTokenWrapper(strings.TrimSpace(decoded))
		if candidate == "" {
			continue
		}
		return candidate
	}
	return raw
}

func extractRefreshToken(raw string) string {
	return extractRefreshTokenInternal(raw, 0, map[string]struct{}{})
}

func extractRefreshTokenFromCookieInputs(cookies []string) string {
	for _, cookie := range cookies {
		token := extractRefreshTokenFromCookieHeader(cookie)
		if token == "" {
			continue
		}
		return token
	}
	return ""
}

func extractRefreshTokenFromCookieHeader(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, part := range strings.Split(raw, ";") {
		segment := strings.TrimSpace(part)
		key, value, ok := splitPair(segment, "=")
		if !ok {
			continue
		}
		if isRefreshTokenField(key) {
			if token := extractRefreshTokenInternal(value, 0, map[string]struct{}{}); token != "" {
				return token
			}
			if token := normalizeRefreshToken(value); token != "" {
				return token
			}
		}
		if token := extractRefreshTokenInternal(value, 0, map[string]struct{}{}); token != "" {
			return token
		}
	}
	key, value, ok := splitPair(raw, "=")
	if ok && isRefreshTokenField(key) {
		if token := extractRefreshTokenInternal(value, 0, map[string]struct{}{}); token != "" {
			return token
		}
		return normalizeRefreshToken(value)
	}
	return ""
}

func extractWTokenFromCookieInputs(cookies []string) string {
	for _, cookie := range cookies {
		token := extractWTokenFromCookieHeader(cookie)
		if token == "" {
			continue
		}
		return token
	}
	return ""
}

func extractWTokenFromCookieHeader(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, part := range strings.Split(raw, ";") {
		segment := strings.TrimSpace(part)
		key, value, ok := splitPair(segment, "=")
		if !ok || !isTokenField(key) {
			continue
		}
		if token := extractWToken(value, 0, map[string]struct{}{}); token != "" {
			return token
		}
	}
	key, value, ok := splitPair(raw, "=")
	if ok && isTokenField(key) {
		return extractWToken(value, 0, map[string]struct{}{})
	}
	return ""
}

func extractWToken(raw string, depth int, seen map[string]struct{}) string {
	if depth > tokenExtractMaxDepth {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, ok := seen[raw]; ok {
		return ""
	}
	seen[raw] = struct{}{}

	if jwtExactPattern.MatchString(raw) {
		return raw
	}

	unwrapped := trimTokenWrapper(raw)
	if unwrapped != raw {
		if token := extractWToken(unwrapped, depth+1, seen); token != "" {
			return token
		}
		raw = unwrapped
	}

	if value, ok := stripBearerPrefix(raw); ok {
		if token := extractWToken(value, depth+1, seen); token != "" {
			return token
		}
	}

	if key, value, ok := splitPair(raw, "="); ok && isTokenField(key) {
		if token := extractWToken(value, depth+1, seen); token != "" {
			return token
		}
	}
	if key, value, ok := splitPair(raw, ":"); ok && isTokenField(key) {
		if token := extractWToken(value, depth+1, seen); token != "" {
			return token
		}
	}

	if token := extractFromJSON(raw, depth, seen); token != "" {
		return token
	}
	if token := extractFromQuery(raw, depth, seen); token != "" {
		return token
	}
	if token := extractWTokenFromCookieHeader(raw); token != "" {
		return token
	}

	for _, decoded := range decodeCandidates(raw) {
		if token := extractWToken(decoded, depth+1, seen); token != "" {
			return token
		}
	}

	if match := tokenKVPattern.FindStringSubmatch(raw); len(match) == 2 && jwtExactPattern.MatchString(match[1]) {
		return strings.TrimSpace(match[1])
	}
	if match := jwtFindPattern.FindString(raw); match != "" {
		return strings.TrimSpace(match)
	}
	return ""
}

func extractRefreshTokenInternal(raw string, depth int, seen map[string]struct{}) string {
	if depth > tokenExtractMaxDepth {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, ok := seen[raw]; ok {
		return ""
	}
	seen[raw] = struct{}{}

	unwrapped := trimTokenWrapper(raw)
	if unwrapped != raw {
		if token := extractRefreshTokenInternal(unwrapped, depth+1, seen); token != "" {
			return token
		}
		raw = unwrapped
	}

	if key, value, ok := splitPair(raw, "="); ok && isRefreshTokenField(key) {
		if token := extractRefreshTokenInternal(value, depth+1, seen); token != "" {
			return token
		}
		if token := normalizeRefreshToken(value); token != "" {
			return token
		}
	}
	if key, value, ok := splitPair(raw, ":"); ok && isRefreshTokenField(key) {
		if token := extractRefreshTokenInternal(value, depth+1, seen); token != "" {
			return token
		}
		if token := normalizeRefreshToken(value); token != "" {
			return token
		}
	}

	if token := extractRefreshFromJSON(raw, depth, seen); token != "" {
		return token
	}
	if token := extractRefreshFromQuery(raw, depth, seen); token != "" {
		return token
	}
	if token := extractRefreshTokenFromCookieHeader(raw); token != "" {
		return token
	}

	for _, decoded := range decodeCandidates(raw) {
		if token := extractRefreshTokenInternal(decoded, depth+1, seen); token != "" {
			return token
		}
	}

	if match := refreshTokenKVPattern.FindStringSubmatch(raw); len(match) == 2 {
		return normalizeRefreshToken(match[1])
	}
	return ""
}

func extractRefreshFromJSON(raw string, depth int, seen map[string]struct{}) string {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	return extractRefreshFromAny(payload, depth+1, seen)
}

func extractRefreshFromAny(payload any, depth int, seen map[string]struct{}) string {
	if depth > tokenExtractMaxDepth {
		return ""
	}
	switch value := payload.(type) {
	case string:
		return extractRefreshTokenInternal(value, depth+1, seen)
	case []any:
		for _, item := range value {
			if token := extractRefreshFromAny(item, depth+1, seen); token != "" {
				return token
			}
		}
	case map[string]any:
		for _, key := range refreshTokenFieldPriority() {
			for actualKey, actualValue := range value {
				if !strings.EqualFold(strings.TrimSpace(actualKey), key) {
					continue
				}
				if token := extractRefreshFromAny(actualValue, depth+1, seen); token != "" {
					return token
				}
				if raw, ok := actualValue.(string); ok {
					if token := normalizeRefreshToken(raw); token != "" {
						return token
					}
				}
			}
		}
		for _, nested := range value {
			if token := extractRefreshFromAny(nested, depth+1, seen); token != "" {
				return token
			}
		}
	}
	return ""
}

func extractRefreshFromQuery(raw string, depth int, seen map[string]struct{}) string {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "?")
	if raw == "" || !strings.Contains(raw, "=") {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	for key, entries := range values {
		if !isRefreshTokenField(key) {
			continue
		}
		for _, value := range entries {
			if token := extractRefreshTokenInternal(value, depth+1, seen); token != "" {
				return token
			}
			if token := normalizeRefreshToken(value); token != "" {
				return token
			}
		}
	}
	return ""
}

func extractFromJSON(raw string, depth int, seen map[string]struct{}) string {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	return extractFromAny(payload, depth+1, seen)
}

func extractFromAny(payload any, depth int, seen map[string]struct{}) string {
	if depth > tokenExtractMaxDepth {
		return ""
	}
	switch value := payload.(type) {
	case string:
		return extractWToken(value, depth+1, seen)
	case []any:
		for _, item := range value {
			if token := extractFromAny(item, depth+1, seen); token != "" {
				return token
			}
		}
	case map[string]any:
		for _, key := range tokenFieldPriority() {
			for actualKey, actualValue := range value {
				if !strings.EqualFold(strings.TrimSpace(actualKey), key) {
					continue
				}
				if token := extractFromAny(actualValue, depth+1, seen); token != "" {
					return token
				}
			}
		}
		for _, nested := range value {
			if token := extractFromAny(nested, depth+1, seen); token != "" {
				return token
			}
		}
	}
	return ""
}

func extractFromQuery(raw string, depth int, seen map[string]struct{}) string {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "?")
	if raw == "" || !strings.Contains(raw, "=") {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	for key, entries := range values {
		if !isTokenField(key) {
			continue
		}
		for _, value := range entries {
			if token := extractWToken(value, depth+1, seen); token != "" {
				return token
			}
		}
	}
	return ""
}

func decodeCandidates(raw string) []string {
	candidates := make([]string, 0, 3)
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != raw {
		candidates = append(candidates, decoded)
	}
	if decoded, err := url.PathUnescape(raw); err == nil && decoded != raw {
		candidates = appendUniqueTokenCandidate(candidates, decoded)
	}
	if decoded, err := strconv.Unquote(raw); err == nil && decoded != raw {
		candidates = appendUniqueTokenCandidate(candidates, decoded)
	}
	return candidates
}

func appendUniqueTokenCandidate(candidates []string, value string) []string {
	for _, existing := range candidates {
		if existing == value {
			return candidates
		}
	}
	return append(candidates, value)
}

func trimTokenWrapper(raw string) string {
	for {
		trimmed := strings.TrimSpace(raw)
		if len(trimmed) < 2 {
			return trimmed
		}
		start := trimmed[0]
		end := trimmed[len(trimmed)-1]
		if (start == '"' && end == '"') || (start == '\'' && end == '\'') || (start == '`' && end == '`') {
			raw = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		return trimmed
	}
}

func stripBearerPrefix(raw string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(lower, "bearer ") {
		return "", false
	}
	return strings.TrimSpace(raw[len("bearer "):]), true
}

func splitPair(raw string, sep string) (string, string, bool) {
	parts := strings.SplitN(raw, sep, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" || value == "" {
		return "", "", false
	}
	return strings.Trim(key, `"'`), strings.TrimSpace(value), true
}

func isTokenField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "accesstoken", "access_token", "__wtoken", "wtoken", "token", "idtoken", "id_token":
		return true
	default:
		return false
	}
}

func tokenFieldPriority() []string {
	return []string{"accessToken", "access_token", "__wtoken", "wtoken", "idToken", "id_token", "token"}
}

func isRefreshTokenField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "refreshtoken", "refresh_token", "__wrtoken", "wrtoken", "wrefresh_token", "refresh":
		return true
	default:
		return false
	}
}

func refreshTokenFieldPriority() []string {
	return []string{"refreshToken", "refresh_token", "__wrtoken", "wrtoken", "wrefresh_token", "refresh"}
}

func tokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return time.Time{}, false
	}
	exp := asInt(claims["exp"])
	if exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0).UTC(), true
}

func tokenExpired(token string, now time.Time, leeway time.Duration) bool {
	expiry, ok := tokenExpiry(token)
	if !ok {
		return false
	}
	return !now.Before(expiry.Add(-leeway))
}
