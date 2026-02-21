package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return ""
}

type globalFlags struct {
	Format        string
	Profile       string
	Address       string
	Locale        string
	NoColor       bool
	Output        string
	WToken        string
	WRefreshToken string
	Cookies       []string
	Verbose       bool
}

const sharedGlobalFlagAnnotation = "wolt_cli_shared_global"

func addGlobalFlags(cmd *cobra.Command, flags *globalFlags) {
	addSharedGlobalFlag(cmd, "format", func() {
		cmd.Flags().StringVar(&flags.Format, "format", "table", "Output format: table, json, or yaml.")
	})
	addSharedGlobalFlag(cmd, "profile", func() {
		cmd.Flags().StringVar(&flags.Profile, "profile", "", "Profile name for saved local defaults.")
	})
	addSharedGlobalFlag(cmd, "address", func() {
		cmd.Flags().StringVar(&flags.Address, "address", "", "Temporary address override for this command. Geocoded to coordinates. Cannot be combined with --lat/--lon.")
	})
	addSharedGlobalFlag(cmd, "locale", func() {
		cmd.Flags().StringVar(&flags.Locale, "locale", "en-FI", "Response locale in BCP-47 format, for example en-FI.")
	})
	addSharedGlobalFlag(cmd, "no-color", func() {
		cmd.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable ANSI color codes in table output.")
	})
	addSharedGlobalFlag(cmd, "wtoken", func() {
		cmd.Flags().StringVar(&flags.WToken, "wtoken", "", "Wolt token for authenticated endpoints (JWT, Bearer value, or payload with accessToken).")
	})
	addSharedGlobalFlag(cmd, "wrtoken", func() {
		cmd.Flags().StringVar(&flags.WRefreshToken, "wrtoken", "", "Wolt refresh token for automatic access token rotation (or payload with refreshToken).")
	})
	addSharedGlobalFlag(cmd, "cookie", func() {
		cmd.Flags().StringArrayVar(&flags.Cookies, "cookie", nil, "HTTP cookie header value to forward (repeatable).")
	})
	addSharedGlobalFlag(cmd, "verbose", func() {
		cmd.Flags().BoolVar(&flags.Verbose, "verbose", false, "Enable verbose output (prints upstream request trace and detailed error diagnostics).")
	})
}

func addSharedGlobalFlag(cmd *cobra.Command, name string, register func()) {
	if cmd.Flags().Lookup(name) != nil {
		return
	}
	register()
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[sharedGlobalFlagAnnotation] = []string{"true"}
}

func resolveProfileLabel(profileName string) string {
	profile := strings.TrimSpace(profileName)
	if profile == "" {
		return "anonymous"
	}
	return profile
}

func resolveProfileLocation(
	ctx context.Context,
	deps Dependencies,
	address string,
	profileName string,
	format output.Format,
	locale string,
	outputPath string,
	auth *woltgateway.AuthContext,
	cmd *cobra.Command,
) (domain.Location, string, error) {
	return resolveLocation(ctx, deps, nil, nil, address, profileName, format, locale, outputPath, auth, cmd)
}

func parseOutputFormat(format string) (output.Format, error) {
	return output.ParseFormat(format)
}

func writeTable(cmd *cobra.Command, text string, outputPath string) error {
	if err := output.WriteOutput(cmd.OutOrStdout(), text, outputPath); err != nil {
		return err
	}
	return nil
}

func writeMachinePayload(cmd *cobra.Command, env output.Envelope, format output.Format, outputPath string) error {
	rendered, err := output.RenderPayload(env, format)
	if err != nil {
		return err
	}
	if err := output.WriteOutput(cmd.OutOrStdout(), rendered, outputPath); err != nil {
		return err
	}
	return nil
}

func emitError(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	code string,
	message string,
) error {
	if format == output.FormatTable {
		if err := output.WriteOutput(cmd.OutOrStdout(), message, outputPath); err != nil {
			return err
		}
		return &exitError{code: 1}
	}
	env := output.BuildEnvelope(profile, locale, nil, []string{}, map[string]any{
		"code":    code,
		"message": message,
	})
	if err := writeMachinePayload(cmd, env, format, outputPath); err != nil {
		return err
	}
	return &exitError{code: 1}
}

func resolveLocation(
	ctx context.Context,
	deps Dependencies,
	lat *float64,
	lon *float64,
	address string,
	profileName string,
	format output.Format,
	locale string,
	outputPath string,
	auth *woltgateway.AuthContext,
	cmd *cobra.Command,
) (domain.Location, string, error) {
	resolvedAddress := strings.TrimSpace(address)
	if resolvedAddress != "" {
		if lat != nil || lon != nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_INVALID_ARGUMENT",
				"Do not combine --address with --lat/--lon. Use either --address or both --lat and --lon.",
			)
		}
		if deps.Location == nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_LOCATION_RESOLVE_ERROR",
				"Location resolver is not available.",
			)
		}
		location, err := deps.Location.Get(ctx, resolvedAddress)
		if err != nil {
			return domain.Location{}, "", emitError(
				cmd,
				format,
				resolveProfileLabel(profileName),
				locale,
				outputPath,
				"WOLT_LOCATION_RESOLVE_ERROR",
				err.Error(),
			)
		}
		return location, resolveProfileLabel(profileName), nil
	}

	if lat == nil && lon == nil {
		profile, err := deps.Profiles.Find(ctx, profileName)
		if err != nil {
			return domain.Location{}, "", profileError(err, format, profileName, locale, outputPath, cmd)
		}
		location, locationErr := resolveAccountLocation(ctx, deps, profile, auth)
		if locationErr == nil {
			return location, profile.Name, nil
		}
		return domain.Location{}, "", emitError(
			cmd,
			format,
			profile.Name,
			locale,
			outputPath,
			"WOLT_LOCATION_RESOLVE_ERROR",
			"unable to resolve location from Wolt account; use --address or sign in and set an address in Wolt",
		)
	}

	if lat == nil || lon == nil {
		return domain.Location{}, "", emitError(
			cmd,
			format,
			resolveProfileLabel(profileName),
			locale,
			outputPath,
			"WOLT_INVALID_ARGUMENT",
			"Both --lat and --lon must be provided together, or omit both to use Wolt account address.",
		)
	}

	return domain.Location{Lat: *lat, Lon: *lon}, resolveProfileLabel(profileName), nil
}

func profileError(err error, format output.Format, profileName string, locale string, outputPath string, cmd *cobra.Command) error {
	message := err.Error()
	if strings.TrimSpace(profileName) == "" {
		profileName = "default"
	}
	return emitError(cmd, format, profileName, locale, outputPath, "WOLT_PROFILE_ERROR", message)
}

func emitUpstreamError(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	verbose bool,
	err error,
) error {
	if err == nil {
		err = woltgateway.ErrUpstream
	}
	if verbose {
		return emitError(cmd, format, profile, locale, outputPath, "WOLT_UPSTREAM_ERROR", err.Error())
	}

	message := woltgateway.ErrUpstream.Error() + " (use --verbose for details)"
	var upstreamErr *woltgateway.UpstreamRequestError
	if errors.As(err, &upstreamErr) && upstreamErr.StatusCode > 0 {
		message = fmt.Sprintf("%s (status %d, use --verbose for details)", woltgateway.ErrUpstream.Error(), upstreamErr.StatusCode)
	}
	return emitError(cmd, format, profile, locale, outputPath, "WOLT_UPSTREAM_ERROR", message)
}

func splitCSV(value string) map[string]struct{} {
	result := map[string]struct{}{}
	if strings.TrimSpace(value) == "" {
		return result
	}
	for _, part := range strings.Split(value, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}
		result[token] = struct{}{}
	}
	return result
}

func requiredArg(name string) string {
	return fmt.Sprintf("%s is required", name)
}

func normalizeCookieInputs(raw []string) []string {
	cookies := make([]string, 0, len(raw))
	for _, cookie := range raw {
		trimmed := strings.TrimSpace(cookie)
		if trimmed == "" {
			continue
		}
		cookies = append(cookies, trimmed)
	}
	return cookies
}

func buildAuthContext(flags globalFlags) woltgateway.AuthContext {
	auth := woltgateway.AuthContext{
		WToken: normalizeWToken(flags.WToken),
	}
	auth.RefreshToken = extractRefreshToken(flags.WRefreshToken)
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = normalizeRefreshToken(flags.WRefreshToken)
	}
	auth.Cookies = normalizeCookieInputs(flags.Cookies)
	if auth.WToken == "" {
		auth.WToken = extractWTokenFromCookieInputs(auth.Cookies)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshToken(flags.WToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshTokenFromCookieInputs(auth.Cookies)
	}
	return auth
}

func buildAuthContextWithProfile(ctx context.Context, deps Dependencies, flags globalFlags) woltgateway.AuthContext {
	auth := buildAuthContext(flags)
	if deps.Profiles == nil {
		return auth
	}
	profile, err := deps.Profiles.Find(ctx, flags.Profile)
	if err != nil {
		return auth
	}
	if len(auth.Cookies) == 0 {
		auth.Cookies = normalizeCookieInputs(profile.Cookies)
	}
	if strings.TrimSpace(auth.WToken) == "" {
		auth.WToken = normalizeWToken(profile.WToken)
	}
	if strings.TrimSpace(auth.WToken) == "" {
		auth.WToken = extractWTokenFromCookieInputs(auth.Cookies)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = normalizeRefreshToken(profile.WRefreshToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshToken(profile.WToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshTokenFromCookieInputs(auth.Cookies)
	}
	return auth
}

func requireAuth(
	cmd *cobra.Command,
	format output.Format,
	profile string,
	locale string,
	outputPath string,
	auth woltgateway.AuthContext,
) error {
	if auth.HasCredentials() {
		return nil
	}
	return emitError(
		cmd,
		format,
		profile,
		locale,
		outputPath,
		"WOLT_AUTH_REQUIRED",
		"Authentication is required. Provide --wtoken or at least one --cookie.",
	)
}

func defaultProfileName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "default"
	}
	return trimmed
}

func isUnauthorizedUpstream(err error) bool {
	var upstreamErr *woltgateway.UpstreamRequestError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	return upstreamErr.StatusCode == 401
}

func upsertProfileTokens(
	ctx context.Context,
	deps Dependencies,
	selectedProfile string,
	accessToken string,
	refreshToken string,
) error {
	if deps.Config == nil {
		return nil
	}
	cfg, err := deps.Config.Load(ctx)
	if err != nil {
		return err
	}
	index := -1
	profileName := strings.TrimSpace(selectedProfile)
	if profileName == "" {
		for i, profile := range cfg.Profiles {
			if profile.IsDefault {
				index = i
				break
			}
		}
	} else {
		for i, profile := range cfg.Profiles {
			if strings.EqualFold(strings.TrimSpace(profile.Name), profileName) {
				index = i
				break
			}
		}
	}
	if index < 0 {
		if profileName == "" {
			return fmt.Errorf("default profile not found in config")
		}
		return fmt.Errorf("profile %q not found in config", profileName)
	}
	if strings.TrimSpace(accessToken) != "" {
		cfg.Profiles[index].WToken = normalizeWToken(accessToken)
	}
	if strings.TrimSpace(refreshToken) != "" {
		cfg.Profiles[index].WRefreshToken = normalizeRefreshToken(refreshToken)
	}
	return deps.Config.Save(ctx, cfg)
}

func refreshAuthContext(
	ctx context.Context,
	deps Dependencies,
	selectedProfile string,
	auth *woltgateway.AuthContext,
) (bool, []string, error) {
	warnings := []string{}
	if auth == nil {
		return false, warnings, fmt.Errorf("auth context is nil")
	}
	refreshToken := strings.TrimSpace(auth.RefreshToken)
	if refreshToken == "" {
		return false, warnings, nil
	}
	result, err := deps.Wolt.RefreshAccessToken(ctx, refreshToken, *auth)
	if err != nil {
		return false, warnings, err
	}
	accessToken := normalizeWToken(result.AccessToken)
	if accessToken == "" {
		return false, warnings, fmt.Errorf("refresh response did not include access token")
	}
	auth.WToken = accessToken
	if candidate := normalizeRefreshToken(result.RefreshToken); candidate != "" {
		auth.RefreshToken = candidate
	}
	warnings = append(warnings, "access token refreshed automatically")
	if err := upsertProfileTokens(ctx, deps, selectedProfile, auth.WToken, auth.RefreshToken); err != nil {
		warnings = append(warnings, "failed to persist rotated tokens in profile config")
	}
	return true, warnings, nil
}

func invokeWithAuthAutoRefresh[T any](
	ctx context.Context,
	deps Dependencies,
	flags globalFlags,
	auth *woltgateway.AuthContext,
	invoke func(woltgateway.AuthContext) (T, error),
) (T, []string, error) {
	var zero T
	warnings := []string{}
	if auth == nil {
		return zero, warnings, fmt.Errorf("auth context is nil")
	}
	selectedProfile := strings.TrimSpace(flags.Profile)
	if tokenExpired(auth.WToken, time.Now().UTC(), 30*time.Second) {
		_, refreshWarnings, refreshErr := refreshAuthContext(ctx, deps, selectedProfile, auth)
		warnings = append(warnings, refreshWarnings...)
		if refreshErr != nil {
			warnings = append(warnings, "automatic token refresh failed before request")
		}
	}

	result, err := invoke(*auth)
	if err == nil {
		return result, warnings, nil
	}
	if !isUnauthorizedUpstream(err) {
		return result, warnings, err
	}

	refreshed, refreshWarnings, refreshErr := refreshAuthContext(ctx, deps, selectedProfile, auth)
	warnings = append(warnings, refreshWarnings...)
	if refreshErr != nil {
		return result, warnings, fmt.Errorf("%w: automatic token refresh failed: %v", err, refreshErr)
	}
	if !refreshed {
		return result, warnings, err
	}

	retryResult, retryErr := invoke(*auth)
	return retryResult, warnings, retryErr
}
