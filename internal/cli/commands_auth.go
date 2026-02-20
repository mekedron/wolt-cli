package cli

import (
	"strings"
	"time"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
	woltgateway "github.com/Valaraucoo/wolt-cli/internal/gateway/wolt"
	"github.com/Valaraucoo/wolt-cli/internal/service/output"
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
					"authenticated":      false,
					"user_id":            "",
					"country":            "",
					"session_expires_at": nil,
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
			data := map[string]any{
				"authenticated":      true,
				"user_id":            userID,
				"country":            country,
				"session_expires_at": emptyToNil(expiresAt),
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
