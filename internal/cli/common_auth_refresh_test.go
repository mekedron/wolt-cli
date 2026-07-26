package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

type failingLoadConfigManager struct {
	loadErr   error
	loadCalls int
	saveCalls int
	saved     domain.Config
}

type failingProfileResolver struct {
	err error
}

func (r *failingProfileResolver) Find(context.Context, string) (domain.Profile, error) {
	return domain.Profile{}, r.err
}

func (*failingLoadConfigManager) Path() string {
	return "test-config.json"
}

func (m *failingLoadConfigManager) Load(context.Context) (domain.Config, error) {
	m.loadCalls++
	if m.loadErr == nil {
		return domain.Config{}, errors.New("temporary read failure")
	}
	return domain.Config{}, m.loadErr
}

func (m *failingLoadConfigManager) Save(_ context.Context, cfg domain.Config) error {
	m.saveCalls++
	m.saved = cfg
	return nil
}

func TestInvokeWithAuthAutoRefreshBootstrapsRefreshOnlyCredentials(t *testing.T) {
	refreshCalls := 0
	requestCalls := 0
	deps := Dependencies{
		Wolt: &testWoltAPI{
			refreshAccessTokenFn: func(
				_ context.Context,
				refreshToken string,
				auth woltgateway.AuthContext,
			) (woltgateway.TokenRefreshResult, error) {
				refreshCalls++
				if refreshToken != "bootstrap-refresh" || auth.WToken != "" {
					t.Fatalf("refresh input token=%q auth=%#v", refreshToken, auth)
				}
				return woltgateway.TokenRefreshResult{
					AccessToken:  "fresh-access",
					RefreshToken: "rotated-refresh",
				}, nil
			},
		},
	}
	auth := woltgateway.AuthContext{RefreshToken: "bootstrap-refresh"}
	if err := requireAuth(
		&cobra.Command{},
		output.FormatJSON,
		"default",
		"en",
		"",
		auth,
	); err != nil {
		t.Fatalf("refresh-only credentials rejected: %v", err)
	}

	result, warnings, err := invokeWithAuthAutoRefresh(
		context.Background(),
		deps,
		globalFlags{},
		&auth,
		func(current woltgateway.AuthContext) (string, error) {
			requestCalls++
			if current.WToken == "" {
				return "", &woltgateway.UpstreamRequestError{StatusCode: 401}
			}
			if current.WToken != "fresh-access" {
				t.Fatalf("request access token = %q", current.WToken)
			}
			return "ok", nil
		},
	)
	if err != nil || result != "ok" {
		t.Fatalf("invokeWithAuthAutoRefresh = %q, %v", result, err)
	}
	if refreshCalls != 1 || requestCalls != 2 {
		t.Fatalf("refresh calls = %d, request calls = %d", refreshCalls, requestCalls)
	}
	if auth.RefreshToken != "rotated-refresh" ||
		len(warnings) != 1 || warnings[0] != "access token refreshed automatically" {
		t.Fatalf("final auth = %#v, warnings = %#v", auth, warnings)
	}
}

func TestLoadRequiredAuthReportsProfileFailures(t *testing.T) {
	cmd := &cobra.Command{}
	outputBuffer := &bytes.Buffer{}
	cmd.SetOut(outputBuffer)
	cmd.SetErr(outputBuffer)
	_, err := loadRequiredAuth(
		context.Background(),
		Dependencies{Profiles: &failingProfileResolver{err: errors.New("config read denied")}},
		globalFlags{Format: "json"},
		output.FormatJSON,
		cmd,
	)
	if err == nil {
		t.Fatal("profile load failure was ignored")
	}
	if !strings.Contains(outputBuffer.String(), `"code": "WOLT_PROFILE_ERROR"`) ||
		strings.Contains(outputBuffer.String(), `"code": "WOLT_AUTH_REQUIRED"`) {
		t.Fatalf("profile load output = %s", outputBuffer.String())
	}
}

func TestExplicitAuthOverridesUnavailableProfile(t *testing.T) {
	auth, err := loadAuthContextWithProfile(
		context.Background(),
		Dependencies{Profiles: &failingProfileResolver{err: errors.New("config read denied")}},
		globalFlags{WRefreshToken: "explicit-refresh"},
	)
	if err != nil || auth.RefreshToken != "explicit-refresh" {
		t.Fatalf("explicit auth = %#v, %v", auth, err)
	}
}

func TestSaveAccountCredentialsCreatesOnlyWhenConfigIsMissing(t *testing.T) {
	config := &failingLoadConfigManager{loadErr: configstore.ErrConfigNotFound}
	err := saveAccountCredentials(
		context.Background(),
		Dependencies{Config: config},
		woltgateway.AuthContext{
			WToken:       "fresh-access",
			RefreshToken: "bootstrap-refresh",
			Cookies:      []string{"cookie=value"},
		},
	)
	if err != nil {
		t.Fatalf("saveAccountCredentials: %v", err)
	}
	if config.saveCalls != 1 || len(config.saved.Profiles) != 1 ||
		config.saved.Profiles[0].WToken != "fresh-access" ||
		config.saved.Profiles[0].WRefreshToken != "bootstrap-refresh" {
		t.Fatalf("saved config = %#v, Save calls = %d", config.saved, config.saveCalls)
	}

	config = &failingLoadConfigManager{}
	err = saveAccountCredentials(
		context.Background(),
		Dependencies{Config: config},
		woltgateway.AuthContext{WToken: "must-not-save"},
	)
	if err == nil || config.saveCalls != 0 {
		t.Fatalf("non-missing Load failure = %v, Save calls = %d", err, config.saveCalls)
	}
}

func TestLogoutHandlesConfigLoadFailuresHonestly(t *testing.T) {
	t.Run("missing config is already logged out", func(t *testing.T) {
		config := &failingLoadConfigManager{loadErr: configstore.ErrConfigNotFound}
		cmd := newLogoutCommand(Dependencies{Config: config})
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{"--format", "json"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("logout with missing config: %v", err)
		}
		if config.saveCalls != 0 || !strings.Contains(output.String(), `"logged_out": true`) {
			t.Fatalf("output = %q, Save calls = %d", output.String(), config.saveCalls)
		}
	})

	t.Run("read failure is reported", func(t *testing.T) {
		config := &failingLoadConfigManager{}
		cmd := newLogoutCommand(Dependencies{Config: config})
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.SetArgs([]string{"--format", "json"})

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "update config during logout") {
			t.Fatalf("logout error = %v, want Load failure", err)
		}
		if config.saveCalls != 0 || strings.Contains(output.String(), `"logged_out": true`) {
			t.Fatalf("output = %q, Save calls = %d", output.String(), config.saveCalls)
		}
	})
}
