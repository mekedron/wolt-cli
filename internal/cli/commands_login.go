package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

const (
	defaultLoginURL             = "https://wolt.com/login"
	defaultChromeDebugPort      = 9222
	managedChromeStartupTimeout = 10 * time.Second
	managedChromePollInterval   = 250 * time.Millisecond
	loginChromePollInterval     = 1500 * time.Millisecond
	loginChromeReadTimeout      = 3 * time.Second
)

func newLoginCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var wtoken string
	var wrtoken string
	var cookies []string
	var timeout time.Duration
	var browserURL string
	var loginURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Wolt and save this account locally.",
		Long: "Log in to Wolt and save this account locally.\n\n" +
			"Without token flags, this opens a managed Chrome window and extracts Wolt auth cookies through Chrome DevTools. " +
			"Use --wtoken/--wrtoken/--cookie for manual token login.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			auth := buildAuthContext(globalFlags{WToken: wtoken, WRefreshToken: wrtoken, Cookies: cookies})
			if !auth.CanAuthenticate() {
				auth, err = loginViaManagedChrome(cmd.Context(), browserURL, loginURL, timeout)
				if err != nil {
					return err
				}
			}
			if err := saveAccountCredentials(cmd.Context(), deps, auth); err != nil {
				return err
			}

			data := map[string]any{
				"logged_in":          auth.CanAuthenticate(),
				"saved":              true,
				"session_expires_at": emptyToNil(tokenExpiryRFC3339(auth.WToken)),
			}
			warnings := []string{}
			if deps.Wolt != nil && auth.CanAuthenticate() {
				payload, authWarnings, userErr := invokeWithAuthAutoRefresh(
					cmd.Context(),
					deps,
					flags,
					&auth,
					func(authCtx woltgateway.AuthContext) (map[string]any, error) {
						return deps.Wolt.UserMe(cmd.Context(), authCtx)
					},
				)
				warnings = append(warnings, authWarnings...)
				if userErr != nil {
					warnings = append(warnings, "credentials saved but account validation failed")
				} else {
					user := asMap(payload["user"])
					data["user_id"] = domain.NormalizeID(coalesceAny(user["_id"], user["id"]))
					data["country"] = asString(coalesceAny(user["country"], payload["country"]))
				}
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildLoginTable(data), flags.Output)
			}
			env := output.BuildEnvelope("default", flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&wtoken, "wtoken", "", "Wolt access token, Bearer value, or copied auth payload.")
	cmd.Flags().StringVar(&wrtoken, "wrtoken", "", "Wolt refresh token or copied refresh payload.")
	cmd.Flags().StringArrayVar(&cookies, "cookie", nil, "Wolt cookie value to save (repeatable).")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "How long to wait for browser login.")
	cmd.Flags().StringVar(&browserURL, "browser-url", fmt.Sprintf("http://127.0.0.1:%d", defaultChromeDebugPort), "Chrome DevTools browser URL.")
	cmd.Flags().StringVar(&loginURL, "login-url", defaultLoginURL, "Wolt login URL to open.")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newLogoutCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out locally by removing saved Wolt credentials.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			if deps.Config != nil {
				if err := configstore.ApplyUpdate(cmd.Context(), deps.Config, func(cfg *domain.Config) (bool, error) {
					hasAddress := strings.TrimSpace(cfg.Account.WoltAddressID) != ""
					if len(cfg.Profiles) > 0 {
						hasAddress = hasAddress ||
							strings.TrimSpace(cfg.Profiles[0].WoltAddressID) != ""
					}
					hasCredentials := !configstore.CredentialsFromConfig(*cfg).Equal(configstore.Credentials{})
					if !hasCredentials && !hasAddress {
						return false, nil
					}
					configstore.SetCredentials(cfg, configstore.Credentials{})
					cfg.Account.WoltAddressID = ""
					cfg.Profiles[0].WoltAddressID = ""
					return true, nil
				}); err != nil {
					return fmt.Errorf("update config during logout: %w", err)
				}
			}
			// Drop the slug→venue-id cache too: a different account may have
			// different locale / coverage and stale entries could mislead the
			// next session.
			clearVenueSlugCache()
			data := map[string]any{"logged_out": true}
			if format == output.FormatTable {
				return writeTable(cmd, output.RenderTable("Logout", []string{"Field", "Value"}, [][]string{{"Saved credentials", "removed"}}), flags.Output)
			}
			env := output.BuildEnvelope("default", flags.Locale, data, nil, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func saveAccountCredentials(ctx context.Context, deps Dependencies, auth woltgateway.AuthContext) error {
	if deps.Config == nil {
		return nil
	}
	wToken := normalizeWToken(auth.WToken)
	wRefresh := normalizeRefreshToken(auth.RefreshToken)
	cookies := normalizeCookieInputs(auth.Cookies)
	if wToken == "" {
		wToken = extractWTokenFromCookieInputs(cookies)
	}
	if wRefresh == "" {
		wRefresh = extractRefreshTokenFromCookieInputs(cookies)
	}
	return configstore.ApplyUpdate(ctx, deps.Config, func(cfg *domain.Config) (bool, error) {
		configstore.SetCredentials(cfg, configstore.Credentials{
			AccessToken:  wToken,
			RefreshToken: wRefresh,
			Cookies:      cookies,
		})
		return true, nil
	})
}

func buildLoginTable(data map[string]any) string {
	return output.RenderTable("Login", []string{"Field", "Value"}, [][]string{
		{"Logged in", boolToYesNo(asBool(data["logged_in"]))},
		{"Saved", boolToYesNo(asBool(data["saved"]))},
		{"User ID", fallbackString(asString(data["user_id"]), "-")},
		{"Country", fallbackString(asString(data["country"]), "-")},
		{"Session expires", fallbackString(asString(data["session_expires_at"]), "-")},
	})
}

func loginViaManagedChrome(ctx context.Context, browserURL string, loginURL string, timeout time.Duration) (woltgateway.AuthContext, error) {
	if timeout <= 0 {
		return woltgateway.AuthContext{}, fmt.Errorf("browser login timeout must be greater than zero")
	}
	loginCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	browserURL = strings.TrimRight(strings.TrimSpace(browserURL), "/")
	if browserURL == "" {
		browserURL = fmt.Sprintf("http://127.0.0.1:%d", defaultChromeDebugPort)
	}
	if err := ensureManagedChrome(loginCtx, browserURL); err != nil {
		return woltgateway.AuthContext{}, managedChromeLoginError(ctx, loginCtx, err)
	}
	if err := openChromeTarget(loginCtx, browserURL, loginURL); err != nil {
		return woltgateway.AuthContext{}, managedChromeLoginError(ctx, loginCtx, err)
	}

	for {
		readCtx, readCancel := context.WithTimeout(loginCtx, loginChromeReadTimeout)
		auth, err := readAuthFromChrome(readCtx, browserURL)
		readCancel()
		if err == nil && chromeAuthHasRealSession(auth) {
			return auth, nil
		}
		if err := waitForContext(loginCtx, loginChromePollInterval); err != nil {
			return woltgateway.AuthContext{}, managedChromeLoginError(ctx, loginCtx, err)
		}
	}
}

func managedChromeLoginError(parent context.Context, loginCtx context.Context, err error) error {
	if parentErr := parent.Err(); parentErr != nil {
		return parentErr
	}
	if loginCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out waiting for Wolt login in Chrome")
	}
	return err
}

// chromeAuthHasRealSession reports whether the CDP cookie scrape produced a
// genuine signed-in session, not just the telemetry/consent cookies Wolt sets
// on every page load. The polling loop uses this to wait for the user to
// actually sign in instead of returning immediately on cookie noise.
func chromeAuthHasRealSession(auth woltgateway.AuthContext) bool {
	return strings.TrimSpace(auth.WToken) != "" || strings.TrimSpace(auth.RefreshToken) != ""
}

func ensureManagedChrome(ctx context.Context, browserURL string) error {
	if chromeDevToolsReady(ctx, browserURL) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	profileDir := filepath.Join(defaultConfigDir(), "chrome-profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return err
	}
	chromeBin, err := managedChromeBinary()
	if err != nil {
		return err
	}
	port := defaultChromeDebugPort
	if parsed, err := url.Parse(browserURL); err == nil && parsed.Port() != "" {
		_, _ = fmt.Sscanf(parsed.Port(), "%d", &port)
	}
	cmd := exec.CommandContext(
		ctx,
		chromeBin,
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	startupCtx, startupCancel := context.WithTimeout(ctx, managedChromeStartupTimeout)
	defer startupCancel()
	if waitForChromeDevTools(startupCtx, browserURL) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("chrome started but DevTools did not become available at %s", browserURL)
}

func managedChromeBinary() (string, error) {
	return discoverChromeBinary(
		runtime.GOOS,
		os.Getenv,
		exec.LookPath,
		func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
	)
}

func discoverChromeBinary(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
	fileExists func(string) bool,
) (string, error) {
	if getenv == nil || lookPath == nil || fileExists == nil {
		return "", fmt.Errorf("chrome discovery is unavailable")
	}
	if configured := trimExecutablePath(getenv("CHROME_BIN")); configured != "" {
		if resolved, ok := resolveChromeCandidate(configured, lookPath, fileExists); ok {
			return resolved, nil
		}
		return "", fmt.Errorf(
			"chrome executable from CHROME_BIN was not found: %s",
			configured,
		)
	}
	for _, command := range chromeCommandCandidates(goos) {
		if resolved, err := lookPath(command); err == nil && strings.TrimSpace(resolved) != "" {
			return resolved, nil
		}
	}
	for _, path := range chromeInstallCandidates(goos, getenv) {
		if fileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf(
		"chrome or Chromium was not found; set CHROME_BIN or start a browser with remote debugging",
	)
}

func resolveChromeCandidate(
	candidate string,
	lookPath func(string) (string, error),
	fileExists func(string) bool,
) (string, bool) {
	if resolved, err := lookPath(candidate); err == nil && strings.TrimSpace(resolved) != "" {
		return resolved, true
	}
	if fileExists(candidate) {
		return filepath.Clean(candidate), true
	}
	return "", false
}

func trimExecutablePath(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func chromeCommandCandidates(goos string) []string {
	switch goos {
	case "windows":
		return []string{"chrome.exe", "msedge.exe", "chromium.exe"}
	case "darwin":
		return []string{"google-chrome", "chromium", "chrome"}
	default:
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
		}
	}
}

func chromeInstallCandidates(goos string, getenv func(string) string) []string {
	var candidates []string
	appendUnder := func(root string, suffixes ...string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		for _, suffix := range suffixes {
			candidates = append(candidates, filepath.Join(root, filepath.FromSlash(suffix)))
		}
	}

	switch goos {
	case "windows":
		appendUnder(
			getenv("LOCALAPPDATA"),
			"Google/Chrome/Application/chrome.exe",
			"Chromium/Application/chrome.exe",
			"Microsoft/Edge/Application/msedge.exe",
		)
		for _, variable := range []string{"PROGRAMFILES", "PROGRAMFILES(X86)"} {
			appendUnder(
				getenv(variable),
				"Google/Chrome/Application/chrome.exe",
				"Chromium/Application/chrome.exe",
				"Microsoft/Edge/Application/msedge.exe",
			)
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		)
		appendUnder(
			getenv("HOME"),
			"Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"Applications/Chromium.app/Contents/MacOS/Chromium",
			"Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		)
	default:
		candidates = append(candidates,
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/microsoft-edge",
			"/opt/google/chrome/google-chrome",
		)
	}
	return candidates
}

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".wolt")
}

func chromeDevToolsReady(ctx context.Context, browserURL string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, chromeProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, browserURL+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitForChromeDevTools(ctx context.Context, browserURL string) bool {
	for {
		if chromeDevToolsReady(ctx, browserURL) {
			return true
		}
		if waitForContext(ctx, managedChromePollInterval) != nil {
			return false
		}
	}
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func openChromeTarget(ctx context.Context, browserURL string, loginURL string) error {
	if strings.TrimSpace(loginURL) == "" {
		loginURL = defaultLoginURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, browserURL+"/json/new?"+url.QueryEscape(loginURL), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}
	return openBrowser(ctx, loginURL)
}

type chromePage struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func readAuthFromChrome(ctx context.Context, browserURL string) (woltgateway.AuthContext, error) {
	// Network.getAllCookies returns *all* browser cookies regardless of which
	// CDP target we attach to — so we don't need a wolt.com tab to be open.
	// Wolt cookies sitting in Chrome's cookie jar are accessible from any
	// page-level CDP target, or from the browser-level target as a fallback.
	pages, err := listChromePages(ctx, browserURL)
	if err != nil {
		return woltgateway.AuthContext{}, err
	}

	tryTarget := func(wsURL string) (woltgateway.AuthContext, bool) {
		client, err := newCDPClient(ctx, wsURL)
		if err != nil {
			return woltgateway.AuthContext{}, false
		}
		auth, err := client.readWoltAuth(ctx)
		_ = client.close()
		if err != nil || !chromeAuthHasRealSession(auth) {
			return woltgateway.AuthContext{}, false
		}
		return auth, true
	}

	// Prefer a wolt.com page if one is open — that's the strongest signal
	// the user actually has an active Wolt session in this browser, which
	// the login wait loop depends on to avoid returning stale cookies from
	// a previous profile.
	for _, page := range pages {
		if page.Type != "page" || page.WebSocketDebuggerURL == "" {
			continue
		}
		if !strings.Contains(page.URL, "wolt.") && !strings.Contains(page.URL, "wolt.com") {
			continue
		}
		if auth, ok := tryTarget(page.WebSocketDebuggerURL); ok {
			return auth, nil
		}
	}

	// No wolt tab open but the cookies may still be in Chrome's jar.
	// Try any page target.
	for _, page := range pages {
		if page.Type != "page" || page.WebSocketDebuggerURL == "" {
			continue
		}
		if auth, ok := tryTarget(page.WebSocketDebuggerURL); ok {
			return auth, nil
		}
	}

	// Last resort: connect to the browser-level target from /json/version.
	// Works even when no normal page is open.
	if wsURL, err := browserDebuggerURL(ctx, browserURL); err == nil && wsURL != "" {
		if auth, ok := tryTarget(wsURL); ok {
			return auth, nil
		}
	}

	return woltgateway.AuthContext{}, fmt.Errorf("no Wolt auth cookies found in Chrome")
}

func listChromePages(ctx context.Context, browserURL string) ([]chromePage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, browserURL+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var pages []chromePage
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return nil, err
	}
	return pages, nil
}

func browserDebuggerURL(ctx context.Context, browserURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, browserURL+"/json/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.WebSocketDebuggerURL), nil
}

type cdpClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
	next int
}

func newCDPClient(ctx context.Context, wsURL string) (*cdpClient, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	return &cdpClient{conn: conn}, nil
}

func (c *cdpClient) close() error {
	return c.conn.Close()
}

func (c *cdpClient) call(
	ctx context.Context,
	method string,
	params map[string]any,
) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return nil, err
		}
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stopCancel := context.AfterFunc(ctx, func() {
		// Gorilla permits Close concurrently with reads and writes, while
		// SetReadDeadline and SetWriteDeadline count as read/write methods and
		// would race with an in-flight ReadJSON or WriteJSON. A canceled CDP
		// request cannot safely reuse its message stream, so close it to
		// unblock either operation.
		_ = c.conn.Close()
	})
	defer stopCancel()

	c.next++
	id := c.next
	if params == nil {
		params = map[string]any{}
	}
	if err := c.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	for {
		var msg map[string]any
		if err := c.conn.ReadJSON(&msg); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if asInt(msg["id"]) != id {
			continue
		}
		if errPayload := asMap(msg["error"]); errPayload != nil {
			return nil, fmt.Errorf("%s", asString(errPayload["message"]))
		}
		return asMap(msg["result"]), nil
	}
}

func (c *cdpClient) readWoltAuth(ctx context.Context) (woltgateway.AuthContext, error) {
	result, err := c.call(ctx, "Network.getAllCookies", nil)
	if err != nil {
		return woltgateway.AuthContext{}, err
	}
	cookies := []string{}
	for _, rawCookie := range asSlice(result["cookies"]) {
		cookie := asMap(rawCookie)
		domainValue := strings.ToLower(asString(cookie["domain"]))
		name := strings.TrimSpace(asString(cookie["name"]))
		value := strings.TrimSpace(asString(cookie["value"]))
		if name == "" || value == "" || !isWoltCookieDomain(domainValue) {
			continue
		}
		cookies = append(cookies, name+"="+value)
	}
	auth := woltgateway.AuthContext{Cookies: cookies}
	auth.WToken = extractWTokenFromCookieInputs(cookies)
	auth.RefreshToken = extractRefreshTokenFromCookieInputs(cookies)
	return auth, nil
}

func isWoltCookieDomain(value string) bool {
	domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
	return domain == "wolt.com" || strings.HasSuffix(domain, ".wolt.com")
}
