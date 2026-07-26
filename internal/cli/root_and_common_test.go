package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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

	login, found := findCommand(root, "login")
	if !found {
		t.Fatal("login command not found")
	}
	hasWToken := false
	for _, option := range commandOptions(login) {
		if option.name == "wtoken" {
			hasWToken = true
			break
		}
	}
	if !hasWToken {
		t.Fatal("expected login command to document wtoken")
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
	if strings.Contains(out, "--wtoken") {
		t.Fatalf("did not expect hidden wtoken in root help output:\n%s", out)
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
	if got := buf.String(); !strings.Contains(got, "session expired") || !strings.Contains(got, "wolt login") {
		t.Fatalf("expected friendly auth hint, got %q", got)
	}

	buf.Reset()
	err = emitUpstreamError(
		cmd,
		output.FormatTable,
		"default",
		"en-FI",
		"",
		false,
		&woltgateway.UpstreamRequestError{StatusCode: 500},
	)
	if !errors.As(err, &exitErr) || exitErr.code != 1 {
		t.Fatalf("expected controlled exit error, got %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "temporarily unavailable") {
		t.Fatalf("expected friendly temporary-error hint, got %q", got)
	}
}

func TestEmitUpstreamErrorKeepsCodeStableInVerboseMode(t *testing.T) {
	upstreamErr := &woltgateway.UpstreamRequestError{
		Method:     "GET",
		URL:        "https://example.invalid/private",
		StatusCode: 503,
		Body:       "diagnostic body",
	}

	codes := make([]string, 0, 2)
	messages := make([]string, 0, 2)
	for _, verbose := range []bool{false, true} {
		cmd := &cobra.Command{}
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		err := emitUpstreamError(
			cmd,
			output.FormatJSON,
			"default",
			"en-FI",
			"",
			verbose,
			upstreamErr,
		)
		var exitErr *exitError
		if !errors.As(err, &exitErr) || exitErr.code != 1 {
			t.Fatalf("verbose=%v: expected controlled exit error, got %v", verbose, err)
		}
		var envelope struct {
			Error map[string]any `json:"error"`
		}
		if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
			t.Fatalf("verbose=%v: decode output: %v", verbose, err)
		}
		codes = append(codes, asString(envelope.Error["code"]))
		messages = append(messages, asString(envelope.Error["message"]))
	}

	if codes[0] != "WOLT_UPSTREAM_TEMPORARY" || codes[1] != codes[0] {
		t.Fatalf("codes = %v, want stable WOLT_UPSTREAM_TEMPORARY", codes)
	}
	if strings.Contains(messages[0], upstreamErr.URL) {
		t.Fatalf("non-verbose message leaked request URL: %q", messages[0])
	}
	if !strings.Contains(messages[1], upstreamErr.URL) ||
		!strings.Contains(messages[1], upstreamErr.Body) {
		t.Fatalf("verbose message omitted diagnostics: %q", messages[1])
	}
}

func TestClassifyCLIUpstreamInvalidSuccessResponse(t *testing.T) {
	err := &woltgateway.UpstreamRequestError{
		StatusCode: 200,
		Cause:      woltgateway.ErrInvalidResponse,
	}
	code, _, ok := classifyCLIUpstreamError(err, false)
	if !ok || code != "WOLT_UPSTREAM_INVALID_RESPONSE" {
		t.Fatalf("classification = (%q, %v), want WOLT_UPSTREAM_INVALID_RESPONSE", code, ok)
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

func TestOpenBrowserDispatchesPerPlatform(t *testing.T) {
	var capturedName string
	var capturedArgs []string
	prev := browserOpenCommand
	t.Cleanup(func() { browserOpenCommand = prev })

	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	browserOpenCommand = func(target string) (string, []string, error) {
		return testExecutable, []string{"-test.run=^$"}, nil
	}

	if err := openBrowser(context.Background(), "https://example.test"); err != nil {
		t.Fatalf("openBrowser returned error: %v", err)
	}

	// Re-route through a recording stub so we can assert the wire-format too.
	browserOpenCommand = func(target string) (string, []string, error) {
		capturedName = "stub"
		capturedArgs = []string{target}
		return testExecutable, []string{"-test.run=^$"}, nil
	}
	if err := openBrowser(context.Background(), "https://example.test"); err != nil {
		t.Fatalf("openBrowser stub returned error: %v", err)
	}
	if capturedName != "stub" || len(capturedArgs) != 1 || capturedArgs[0] != "https://example.test" {
		t.Fatalf("unexpected stub invocation: name=%q args=%v", capturedName, capturedArgs)
	}
}

func TestOpenBrowserRejectsEmptyURL(t *testing.T) {
	if err := openBrowser(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestDefaultBrowserOpenCommandShape(t *testing.T) {
	name, args, err := defaultBrowserOpenCommand("https://example.test")
	if err != nil {
		t.Fatalf("defaultBrowserOpenCommand returned error: %v", err)
	}
	if name == "" {
		t.Fatal("expected a non-empty command name")
	}
	if len(args) == 0 {
		t.Fatal("expected at least one arg containing the URL")
	}
	found := false
	for _, arg := range args {
		if strings.Contains(arg, "example.test") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("URL did not appear in args: %v", args)
	}
}
