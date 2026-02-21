package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCommandOptionsHideSharedGlobals(t *testing.T) {
	root := NewRootCommand(Dependencies{Version: "test"})

	cartAdd, found := findCommand(root, "cart", "add")
	if !found {
		t.Fatal("cart add command not found")
	}
	for _, option := range commandOptions(cartAdd) {
		if option.name == "wtoken" || option.name == "wrtoken" || option.name == "cookie" {
			t.Fatalf("shared auth option leaked into command-specific options: %s", option.name)
		}
	}

	configure, found := findCommand(root, "configure")
	if !found {
		t.Fatal("configure command not found")
	}
	hasWToken := false
	for _, option := range commandOptions(configure) {
		if option.name == "wtoken" {
			hasWToken = true
			break
		}
	}
	if hasWToken {
		t.Fatal("expected configure command to avoid duplicate global wtoken option docs")
	}
}

func TestRenderRootHelpIncludesGlobalSection(t *testing.T) {
	root := NewRootCommand(Dependencies{Version: "test"})
	buf := &bytes.Buffer{}
	renderRootHelp(buf, root)
	out := buf.String()
	if !strings.Contains(out, "global options") {
		t.Fatalf("expected global options in help output:\n%s", out)
	}
	if !strings.Contains(out, "--wtoken") {
		t.Fatalf("expected wtoken in help output:\n%s", out)
	}
}

type testVerboseTraceSetter struct {
	output io.Writer
}

func (s *testVerboseTraceSetter) SetVerboseOutput(out io.Writer) {
	s.output = out
}

func TestAttachVerboseHTTPTrace(t *testing.T) {
	cmd := &cobra.Command{}
	stderr := &bytes.Buffer{}
	cmd.SetErr(stderr)
	cmd.Flags().Bool("verbose", false, "test verbose")

	setter := &testVerboseTraceSetter{}
	attachVerboseHTTPTrace(cmd, setter)
	if setter.output != nil {
		t.Fatal("expected verbose trace sink to stay disabled when --verbose is false")
	}

	if err := cmd.Flags().Set("verbose", "true"); err != nil {
		t.Fatalf("set verbose flag: %v", err)
	}
	attachVerboseHTTPTrace(cmd, setter)
	if setter.output == nil {
		t.Fatal("expected verbose trace sink to be enabled")
	}
	if !strings.Contains(stderr.String(), "http trace enabled") {
		t.Fatalf("expected trace activation message, got %q", stderr.String())
	}
}

func TestEmitUpstreamErrorFormatting(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := emitUpstreamError(
		cmd,
		output.FormatTable,
		"default",
		"en-FI",
		"",
		false,
		&woltgateway.UpstreamRequestError{StatusCode: 401},
	)
	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.code != 1 {
		t.Fatalf("expected controlled exit error, got %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "status 401") {
		t.Fatalf("expected non-verbose status hint, got %q", got)
	}
}

func TestResolveLocationValidation(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	deps := Dependencies{
		Profiles: &testProfiles{
			profile: domain.Profile{Name: "default", Location: domain.Location{Lat: 60.1, Lon: 24.9}},
		},
		Location: &testLocation{location: domain.Location{Lat: 61.0, Lon: 25.0}},
	}

	lon := 24.9
	_, _, err := resolveLocation(context.Background(), deps, nil, &lon, "", "", output.FormatTable, "en-FI", "", nil, cmd)
	if err == nil {
		t.Fatal("expected resolveLocation to fail when only lon is provided")
	}

	lat := 60.1
	_, _, err = resolveLocation(context.Background(), deps, &lat, &lon, "Kamppi, Helsinki", "", output.FormatTable, "en-FI", "", nil, cmd)
	if err == nil {
		t.Fatal("expected resolveLocation to fail when --address and --lat/--lon are combined")
	}

	location, profile, err := resolveLocation(context.Background(), deps, nil, nil, "Kamppi, Helsinki", "", output.FormatTable, "en-FI", "", nil, cmd)
	if err != nil {
		t.Fatalf("expected resolveLocation to resolve --address, got %v", err)
	}
	if location.Lat != 61.0 || location.Lon != 25.0 {
		t.Fatalf("unexpected resolved location: %+v", location)
	}
	if profile != "anonymous" {
		t.Fatalf("expected anonymous profile for address override, got %q", profile)
	}
}

func TestResolveLocationUsesWoltAccountAddress(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	deps := Dependencies{
		Profiles: &testProfiles{
			profile: domain.Profile{
				Name:          "default",
				WToken:        "token-1",
				WoltAddressID: "addr-2",
			},
		},
		Wolt: &testWoltAPI{
			deliveryInfoListFn: func(context.Context, woltgateway.AuthContext) (map[string]any, error) {
				return map[string]any{
					"results": []any{
						map[string]any{
							"id": "addr-1",
							"location": map[string]any{
								"user_coordinates": map[string]any{
									"type":        "Point",
									"coordinates": []any{24.9000, 60.1000},
								},
							},
						},
						map[string]any{
							"id": "addr-2",
							"location": map[string]any{
								"user_coordinates": map[string]any{
									"type":        "Point",
									"coordinates": []any{25.1000, 61.2000},
								},
							},
						},
					},
				}, nil
			},
		},
	}

	location, profile, err := resolveLocation(context.Background(), deps, nil, nil, "", "", output.FormatTable, "en-FI", "", nil, cmd)
	if err != nil {
		t.Fatalf("expected location from account, got error: %v", err)
	}
	if profile != "default" {
		t.Fatalf("expected profile default, got %q", profile)
	}
	if location.Lat != 61.2 || location.Lon != 25.1 {
		t.Fatalf("expected preferred account address coordinates, got %+v", location)
	}
}

func TestResolveLocationErrorsWithoutAccountOrOverrides(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	deps := Dependencies{
		Profiles: &testProfiles{
			profile: domain.Profile{Name: "default"},
		},
		Wolt: &testWoltAPI{},
	}

	_, _, err := resolveLocation(context.Background(), deps, nil, nil, "", "", output.FormatTable, "en-FI", "", nil, cmd)
	if err == nil {
		t.Fatal("expected resolveLocation to fail without account location or overrides")
	}
}

func TestInvokeWithAuthAutoRefreshRetriesOnUnauthorized(t *testing.T) {
	deps := Dependencies{
		Wolt: &testWoltAPI{
			refreshAccessTokenFn: func(_ context.Context, refreshToken string, _ woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
				if refreshToken != "refresh-1" {
					t.Fatalf("unexpected refresh token: %q", refreshToken)
				}
				return woltgateway.TokenRefreshResult{
					AccessToken:  "new-access-token",
					RefreshToken: "refresh-2",
				}, nil
			},
		},
		Config: &testConfigManager{
			cfg: domain.Config{
				Profiles: []domain.Profile{
					{Name: "default", IsDefault: true, WToken: "expired", WRefreshToken: "refresh-1"},
				},
			},
		},
	}

	auth := &woltgateway.AuthContext{WToken: "expired", RefreshToken: "refresh-1"}
	calls := 0
	result, warnings, err := invokeWithAuthAutoRefresh(
		context.Background(),
		deps,
		globalFlags{Profile: "default"},
		auth,
		func(inAuth woltgateway.AuthContext) (string, error) {
			calls++
			if calls == 1 {
				return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			if inAuth.WToken != "new-access-token" {
				t.Fatalf("expected refreshed access token, got %q", inAuth.WToken)
			}
			return "ok", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected ok result, got %q", result)
	}
	if calls != 2 {
		t.Fatalf("expected two invocations, got %d", calls)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected refresh warning, got none")
	}
}

func TestFlagHelpers(t *testing.T) {
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.StringP("profile", "p", "", "Profile.")
	flag := flagSet.Lookup("profile")
	if flag == nil {
		t.Fatal("profile flag not found")
	}
	flag.Annotations = map[string][]string{cobra.BashCompOneRequiredFlag: {"true"}}

	token := flagToken(flag)
	if token != "--profile/-p" {
		t.Fatalf("unexpected flag token: %q", token)
	}
	if !isFlagRequired(flag) {
		t.Fatal("expected required flag")
	}
	label := optionLabels(optionDoc{required: true, inherited: true})
	if label != " [required, global]" {
		t.Fatalf("unexpected option labels: %q", label)
	}
}

func findCommand(root *cobra.Command, path ...string) (*cobra.Command, bool) {
	current := root
	for _, segment := range path {
		next := current.Commands()
		found := false
		for _, cmd := range next {
			if cmd.Name() == segment {
				current = cmd
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return current, true
}

func TestDefaultProfileName(t *testing.T) {
	if got := defaultProfileName(""); got != "default" {
		t.Fatalf("expected default profile name, got %q", got)
	}
	if got := defaultProfileName(" work "); got != "work" {
		t.Fatalf("expected trimmed profile name, got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	result := splitCSV("hours, tags, HOURS")
	if len(result) != 2 {
		t.Fatalf("expected two unique keys, got %v", result)
	}
}

func TestEmptyToNil(t *testing.T) {
	if got := emptyToNil("   "); got != nil {
		t.Fatalf("expected nil for blank input, got %v", got)
	}
	if got := emptyToNil("x"); got == nil {
		t.Fatal("expected non-nil for non-blank input")
	}
}

func TestInvokeWithExpiredTokenPreRefreshNoRefreshToken(t *testing.T) {
	deps := Dependencies{Wolt: &testWoltAPI{}}
	auth := &woltgateway.AuthContext{WToken: buildExpiringJWT(time.Now().Add(-time.Hour).Unix()), RefreshToken: ""}
	calls := 0
	_, warnings, err := invokeWithAuthAutoRefresh(
		context.Background(),
		deps,
		globalFlags{},
		auth,
		func(_ woltgateway.AuthContext) (string, error) {
			calls++
			return "ok", nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one invoke call, got %d", calls)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings without refresh token, got %v", warnings)
	}
}

type testLocation struct {
	location domain.Location
}

func (m *testLocation) Get(context.Context, string) (domain.Location, error) {
	return m.location, nil
}

func buildExpiringJWT(exp int64) string {
	header := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	payloadJSON := fmt.Sprintf(`{"exp":%d}`, exp)
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}
