package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
	woltgateway "github.com/Valaraucoo/wolt-cli/internal/gateway/wolt"
	"github.com/Valaraucoo/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return ""
}

type globalFlags struct {
	Format  string
	Profile string
	Locale  string
	NoColor bool
	Output  string
}

func addGlobalFlags(cmd *cobra.Command, flags *globalFlags) {
	cmd.Flags().StringVar(&flags.Format, "format", "table", "Output format: table, json, or yaml.")
	cmd.Flags().StringVar(&flags.Profile, "profile", "", "Profile name for saved location. Used when --lat and --lon are not provided.")
	cmd.Flags().StringVar(&flags.Locale, "locale", "en-FI", "Response locale in BCP-47 format, for example en-FI.")
	cmd.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable ANSI color codes in table output.")
	cmd.Flags().StringVar(&flags.Output, "output", "", "Write the command output to a file.")
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
	profileName string,
	format output.Format,
	locale string,
	outputPath string,
	cmd *cobra.Command,
) (domain.Location, string, error) {
	if lat == nil && lon == nil {
		profile, err := deps.Profiles.Find(ctx, profileName)
		if err != nil {
			return domain.Location{}, "", profileError(err, format, profileName, locale, outputPath, cmd)
		}
		return profile.Location, profile.Name, nil
	}

	if lat == nil || lon == nil {
		profile := profileName
		if strings.TrimSpace(profile) == "" {
			profile = "anonymous"
		}
		return domain.Location{}, "", emitError(
			cmd,
			format,
			profile,
			locale,
			outputPath,
			"WOLT_INVALID_ARGUMENT",
			"Both --lat and --lon must be provided together, or omit both to use profile location.",
		)
	}

	profile := profileName
	if strings.TrimSpace(profile) == "" {
		profile = "anonymous"
	}
	return domain.Location{Lat: *lat, Lon: *lon}, profile, nil
}

func profileError(err error, format output.Format, profileName string, locale string, outputPath string, cmd *cobra.Command) error {
	message := err.Error()
	if strings.TrimSpace(profileName) == "" {
		profileName = "default"
	}
	return emitError(cmd, format, profileName, locale, outputPath, "WOLT_PROFILE_ERROR", message)
}

func emitUpstreamError(cmd *cobra.Command, format output.Format, profile string, locale string, outputPath string, err error) error {
	if err == nil {
		err = woltgateway.ErrUpstream
	}
	return emitError(cmd, format, profile, locale, outputPath, "WOLT_UPSTREAM_ERROR", err.Error())
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
