package cli

import (
	"context"
	"testing"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func TestPullAuthFromRunningChromeReturnsWoltCookies(t *testing.T) {
	t.Setenv(envDisableChromeSync, "")
	fake := newFakeChromeDevTools(t, []map[string]any{
		{"name": "__wtoken", "value": "header.payload.sig", "domain": "wolt.com"},
		{"name": "__wrtoken", "value": "refresh-from-chrome", "domain": "wolt.com"},
		{"name": "telemetryDeviceId", "value": "abc-123", "domain": "wolt.com"},
	}, "https://wolt.com/account")
	defer fake.Close()

	auth, found, err := pullAuthFromRunningChrome(context.Background(), fake.BrowserURL())
	if err != nil {
		t.Fatalf("pullAuthFromRunningChrome returned error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true with a Wolt session in fake Chrome")
	}
	if auth.WToken != "header.payload.sig" {
		t.Fatalf("unexpected wtoken: %q", auth.WToken)
	}
	if auth.RefreshToken != "refresh-from-chrome" {
		t.Fatalf("unexpected refresh token: %q", auth.RefreshToken)
	}
}

func TestPullAuthFromRunningChromeRejectsTelemetryOnlyCookies(t *testing.T) {
	t.Setenv(envDisableChromeSync, "")
	fake := newFakeChromeDevTools(t, []map[string]any{
		{"name": "telemetryDeviceId", "value": "synthetic-device", "domain": "wolt.com"},
	}, "https://wolt.com/discovery")
	defer fake.Close()

	auth, found, err := pullAuthFromRunningChrome(context.Background(), fake.BrowserURL())
	if err != nil {
		t.Fatalf("pullAuthFromRunningChrome returned error: %v", err)
	}
	if found || auth.HasCredentials() {
		t.Fatalf("telemetry-only snapshot was accepted: found=%v auth=%#v", found, auth)
	}
}

func TestPullAuthFromRunningChromeRejectsLookalikeCookieDomains(t *testing.T) {
	t.Setenv(envDisableChromeSync, "")
	fake := newFakeChromeDevTools(t, []map[string]any{
		{"name": "__wtoken", "value": "untrusted-access", "domain": "evilwolt.example"},
		{"name": "__wrtoken", "value": "untrusted-refresh", "domain": "notwolt.com"},
	}, "https://example.test/")
	defer fake.Close()

	auth, found, err := pullAuthFromRunningChrome(context.Background(), fake.BrowserURL())
	if err != nil {
		t.Fatalf("pullAuthFromRunningChrome returned error: %v", err)
	}
	if found || auth.CanAuthenticate() {
		t.Fatalf("lookalike-domain cookies were accepted: found=%v auth=%#v", found, auth)
	}
}

func TestPullAuthFromRunningChromeBoundsStalledCDP(t *testing.T) {
	t.Setenv(envDisableChromeSync, "")
	fake := newFakeChromeDevTools(t, nil, "https://wolt.com/account")
	fake.stallCDP = true
	defer fake.Close()

	start := time.Now()
	_, found, err := pullAuthFromRunningChrome(context.Background(), fake.BrowserURL())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("stalled CDP returned error: %v", err)
	}
	if found {
		t.Fatal("stalled CDP returned found=true")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("stalled CDP exceeded opportunistic timeout: %s", elapsed)
	}
}

func TestPullAuthFromRunningChromeRespectsDisableEnv(t *testing.T) {
	t.Setenv(envDisableChromeSync, "1")
	fake := newFakeChromeDevTools(t, []map[string]any{
		{"name": "__wtoken", "value": "header.payload.sig", "domain": "wolt.com"},
	}, "https://wolt.com/account")
	defer fake.Close()

	_, found, err := pullAuthFromRunningChrome(context.Background(), fake.BrowserURL())
	if err != nil {
		t.Fatalf("expected no error when disabled, got %v", err)
	}
	if found {
		t.Fatal("expected found=false when WOLT_DISABLE_CHROME_SYNC is set")
	}
}

func TestPullAuthFromRunningChromeIsSilentWhenChromeDown(t *testing.T) {
	t.Setenv(envDisableChromeSync, "")
	// Point at a guaranteed-unused port. The probe must return found=false,
	// not error, so opportunistic syncs in production never escalate when
	// Chrome is simply closed.
	_, found, err := pullAuthFromRunningChrome(context.Background(), "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("expected no error when Chrome is unreachable, got %v", err)
	}
	if found {
		t.Fatal("expected found=false when Chrome is unreachable")
	}
}

func TestChromeAuthIsFresherThanComparesJWTExpiry(t *testing.T) {
	now := time.Now()
	older := buildExpiringJWT(now.Add(-time.Hour).Unix())
	newer := buildExpiringJWT(now.Add(time.Hour).Unix())

	if !chromeAuthIsFresherThan(authContextWithToken(newer), older) {
		t.Fatal("expected newer JWT to be considered fresher")
	}
	if chromeAuthIsFresherThan(authContextWithToken(older), newer) {
		t.Fatal("expected older JWT to NOT be considered fresher")
	}
	// Garbage current token → prefer Chrome's parseable one.
	if !chromeAuthIsFresherThan(authContextWithToken(newer), "not-a-jwt") {
		t.Fatal("expected Chrome's parseable token to win over unparseable current token")
	}
	// Garbage Chrome token → never adopt.
	if chromeAuthIsFresherThan(authContextWithToken("not-a-jwt"), newer) {
		t.Fatal("expected unparseable Chrome token to never be considered fresher")
	}
}

func authContextWithToken(token string) woltgateway.AuthContext {
	return woltgateway.AuthContext{WToken: token}
}
