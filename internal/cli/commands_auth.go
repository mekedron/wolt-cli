package cli

import (
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newAuthCommand(deps Dependencies) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Inspect authentication state for authenticated commands.",
	}
	auth.AddCommand(newAuthStatusCommand(deps))
	return auth
}

func newAuthStatusCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authenticated user status from upstream session.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profileName := defaultProfileName(flags.Profile)

			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if !auth.HasCredentials() {
				data := map[string]any{
					"authenticated":        false,
					"user_id":              "",
					"country":              "",
					"session_expires_at":   nil,
					"wolt_plus_subscriber": false,
				}
				warnings := []string{"no auth credentials provided"}
				if format == output.FormatTable {
					return writeTable(cmd, buildAuthStatusTable(data), flags.Output)
				}
				env := output.BuildEnvelope(profileName, flags.Locale, data, warnings, nil)
				return writeMachinePayload(cmd, env, format, flags.Output)
			}

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.UserMe(cmd.Context(), authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}

			user := asMap(payload["user"])
			userID := domain.NormalizeID(coalesceAny(user["_id"], user["id"]))
			country := asString(coalesceAny(user["country"], payload["country"]))
			expiresAt := tokenExpiryRFC3339(auth.WToken)
			woltPlusSubscriber, _ := extractWoltPlusSubscriber(payload)
			data := map[string]any{
				"authenticated":        true,
				"user_id":              userID,
				"country":              country,
				"session_expires_at":   emptyToNil(expiresAt),
				"wolt_plus_subscriber": woltPlusSubscriber,
			}
			if flags.Verbose {
				data["token_preview"] = tokenPreview(auth.WToken)
				data["cookie_count"] = len(auth.Cookies)
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildAuthStatusTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildAuthStatusTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Authenticated", boolToYesNo(asBool(data["authenticated"]))},
		{"Wolt+ subscriber", boolToYesNo(asBool(data["wolt_plus_subscriber"]))},
		{"User ID", fallbackString(asString(data["user_id"]), "-")},
		{"Country", fallbackString(asString(data["country"]), "-")},
		{"Session expires", fallbackString(asString(data["session_expires_at"]), "-")},
	}
	if preview := asString(data["token_preview"]); preview != "" {
		rows = append(rows, []string{"Token preview", preview})
	}
	if cookieCount := asInt(data["cookie_count"]); cookieCount > 0 {
		rows = append(rows, []string{"Cookie count", asString(cookieCount)})
	}
	return output.RenderTable("Auth status", headers, rows)
}

func tokenPreview(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 12 {
		return token
	}
	return token[:6] + "..." + token[len(token)-6:]
}

func tokenExpiryRFC3339(token string) string {
	expiry, ok := tokenExpiry(token)
	if !ok {
		return ""
	}
	return expiry.Format(time.RFC3339)
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func coalesceAny(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

func extractWoltPlusSubscriber(payload map[string]any) (bool, bool) {
	user := asMap(payload["user"])
	primaryCandidates := []map[string]any{user, payload}

	primaryKeys := []string{
		"is_wolt_plus_subscriber",
		"wolt_plus_subscriber",
		"is_wolt_plus_member",
		"wolt_plus_member",
		"wolt_plus_active",
		"wolt_plus",
		"has_wolt_plus",
		"is_wolt_plus",
		"is_plus_subscriber",
		"plus_subscriber",
	}
	for _, candidate := range primaryCandidates {
		if candidate == nil {
			continue
		}
		for _, key := range primaryKeys {
			parsed, ok := parseWoltPlusState(candidate[key])
			if ok {
				return parsed, true
			}
		}
	}

	nestedKeys := []string{
		"wolt_plus",
		"wolt_plus_subscription",
		"wolt_plus_membership",
		"plus_subscription",
		"subscription",
		"membership",
	}
	nestedCandidates := make([]map[string]any, 0, len(primaryCandidates)*len(nestedKeys))
	for _, candidate := range primaryCandidates {
		if candidate == nil {
			continue
		}
		for _, key := range nestedKeys {
			if nested := asMap(candidate[key]); nested != nil {
				nestedCandidates = append(nestedCandidates, nested)
			}
		}
	}

	nestedValueKeys := []string{
		"is_subscriber",
		"subscriber",
		"is_member",
		"member",
		"is_active",
		"active",
		"enabled",
		"is_enabled",
		"has_subscription",
		"status",
		"state",
		"membership_status",
		"subscription_status",
	}
	for _, candidate := range nestedCandidates {
		for _, key := range nestedValueKeys {
			parsed, ok := parseWoltPlusState(candidate[key])
			if ok {
				return parsed, true
			}
		}
	}

	return false, false
}

func parseWoltPlusState(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case float64:
		return typed != 0, true
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		switch normalized {
		case "true", "yes", "1", "active", "enabled", "subscribed", "member", "trial", "in_trial", "premium":
			return true, true
		case "false", "no", "0", "inactive", "disabled", "unsubscribed", "not_subscribed", "none", "cancelled", "canceled", "expired":
			return false, true
		}
	}
	return false, false
}
