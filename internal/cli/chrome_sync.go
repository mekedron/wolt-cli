package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

const (
	// chromeProbeTimeout bounds the complete opportunistic Chrome scrape. Kept
	// short so syncs don't add noticeable latency when Chrome is closed.
	chromeProbeTimeout = 500 * time.Millisecond
	// envDisableChromeSync, when non-empty, suppresses the opportunistic
	// re-sync from a running Chrome. Tests set it to keep results
	// independent of the developer's local browser state; users with
	// hardened setups can set it too.
	envDisableChromeSync = "WOLT_DISABLE_CHROME_SYNC"
)

var pullOpportunisticChromeAuth = pullAuthFromRunningChrome

// pullAuthFromRunningChrome reads Wolt auth cookies from a Chrome that is
// already running with --remote-debugging-port. Unlike loginViaManagedChrome it
// never launches Chrome and never waits for user interaction — if Chrome isn't
// reachable within a short probe window, it returns found=false with no error.
//
// This mirrors what the browser itself does: every wolt.com tab refreshes the
// in-memory access token off the bootstrap __wrtoken cookie. If a user runs
// the CLI while their Chrome is open, the CLI can sip the freshest cookies
// straight from that session — same chain, no divergence.
func pullAuthFromRunningChrome(ctx context.Context, browserURL string) (woltgateway.AuthContext, bool, error) {
	if strings.TrimSpace(os.Getenv(envDisableChromeSync)) != "" {
		return woltgateway.AuthContext{}, false, nil
	}
	browserURL = strings.TrimRight(strings.TrimSpace(browserURL), "/")
	if browserURL == "" {
		browserURL = fmt.Sprintf("http://127.0.0.1:%d", defaultChromeDebugPort)
	}

	scrapeCtx, cancel := context.WithTimeout(ctx, chromeProbeTimeout)
	defer cancel()

	auth, err := readAuthFromChrome(scrapeCtx, browserURL)
	if err != nil {
		// Chrome is up but has no wolt.com session, or CDP refused — treat as
		// "no fresh auth available", not as a hard failure.
		return woltgateway.AuthContext{}, false, nil
	}
	if !chromeAuthHasRealSession(auth) {
		return woltgateway.AuthContext{}, false, nil
	}
	return auth, true, nil
}

// chromeAuthIsFresherThan reports whether Chrome's snapshot has a strictly
// newer __wtoken JWT expiry than the auth we're about to use. This is the
// signal to adopt Chrome's auth before making a request — it means the
// browser has refreshed more recently than we have.
func chromeAuthIsFresherThan(chromeAuth woltgateway.AuthContext, currentToken string) bool {
	chromeExp, chromeOK := tokenExpiry(chromeAuth.WToken)
	if !chromeOK {
		return false
	}
	currentExp, currentOK := tokenExpiry(currentToken)
	if !currentOK {
		// We don't have a usable expiry — Chrome's is at least parseable, so
		// prefer it.
		return true
	}
	return chromeExp.After(currentExp)
}

// adoptChromeAuth replaces only the current process's auth context with
// Chrome's snapshot. Opportunistic sync must never rewrite shared config:
// explicit `wolt login` and `wolt logout` are the only credential writers.
func adoptChromeAuth(
	auth *woltgateway.AuthContext,
	chromeAuth woltgateway.AuthContext,
) error {
	if auth == nil {
		return fmt.Errorf("auth context is nil")
	}
	adopted := chromeAuth
	if strings.TrimSpace(adopted.RefreshToken) == "" {
		adopted.RefreshToken = auth.RefreshToken
	}
	adopted.Cookies = append([]string(nil), chromeAuth.Cookies...)
	*auth = adopted
	return nil
}
