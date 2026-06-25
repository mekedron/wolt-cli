package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
	"github.com/spf13/cobra"
)

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

			result, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (authStatusResult, error) {
					user, userErr := deps.Wolt.UserMe(cmd.Context(), authCtx)
					if userErr != nil {
						return authStatusResult{}, userErr
					}
					res := authStatusResult{user: user}
					// Wolt+ membership lives on a dedicated subscriptions endpoint,
					// not on /v1/user/me. Treat its failure as non-fatal: the auth
					// summary is still useful, and an expired token is already
					// refreshed via the UserMe call above before this runs on retry.
					subscriptions, subsErr := deps.Wolt.Subscriptions(cmd.Context(), authCtx)
					if subsErr != nil {
						res.subscriptionsErr = subsErr
						return res, nil
					}
					res.subscriptions = subscriptions
					return res, nil
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err, authWarnings...)
			}

			user := asMap(result.user["user"])
			userID := domain.NormalizeID(coalesceAny(user["_id"], user["id"]))
			country := asString(coalesceAny(user["country"], result.user["country"]))
			expiresAt := tokenExpiryRFC3339(auth.WToken)
			woltPlusSubscriber, woltPlusKnown := woltPlusActive(result.subscriptions, time.Now().UTC())
			if !woltPlusKnown {
				if result.subscriptionsErr != nil {
					authWarnings = append(authWarnings, fmt.Sprintf("wolt plus status unavailable: %v", result.subscriptionsErr))
				} else {
					authWarnings = append(authWarnings, "wolt plus status unavailable")
				}
			}
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
	return payloadutil.CoalesceAny(values...)
}

// authStatusResult bundles the upstream payloads the status command depends on so
// they can be fetched under a single auth-refresh attempt.
type authStatusResult struct {
	user             map[string]any
	subscriptions    map[string]any
	subscriptionsErr error
}

// woltPlusActive reports whether the subscriptions payload from
// consumer-api.wolt.com/subscriptions-api/v1/subscriptions contains a currently
// active Wolt+ subscription. The second return value is false when the payload
// carries no subscriptions list at all, meaning membership could not be
// determined (e.g. the lookup failed) rather than being definitively absent.
func woltPlusActive(payload map[string]any, now time.Time) (active bool, known bool) {
	raw, ok := payload["subscriptions"]
	if !ok {
		return false, false
	}
	subscriptions := asSlice(raw)
	if subscriptions == nil {
		return false, false
	}
	for _, value := range subscriptions {
		if subscriptionActive(asMap(value), now) {
			return true, true
		}
	}
	return false, true
}

// subscriptionActive decides whether a single subscription entry currently grants
// Wolt+ access. paid_until_date is the moment the paid period lapses; while it is
// in the future the member still has access, which also covers cancelled-but-not-
// yet-expired subscriptions. end_date is null/absent for open-ended auto-renewing
// plans, so it is only consulted when paid_until_date is missing.
func subscriptionActive(sub map[string]any, now time.Time) bool {
	if sub == nil {
		return false
	}
	nowUnix := float64(now.Unix())
	if paidUntil, ok := asFloat(sub["paid_until_date"]); ok {
		return paidUntil > nowUnix
	}
	if endDate, ok := asFloat(sub["end_date"]); ok {
		return endDate > nowUnix
	}
	if start, ok := asFloat(sub["start_date"]); ok {
		return start <= nowUnix
	}
	return true
}
