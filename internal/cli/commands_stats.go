package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/mekedron/wolt-cli/internal/service/statsbundle"
	"github.com/mekedron/wolt-cli/internal/service/statsserve"
	"github.com/mekedron/wolt-cli/internal/service/statssync"
	"github.com/spf13/cobra"
)

// writeStep prints a "[i/N] Title" banner with a blank line before it so
// each phase visually separates from the previous one. No-op when w is nil
// (JSON / YAML mode passes nil to keep stdout clean for the envelope).
func writeStep(w io.Writer, step, total int, title string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "\n[%d/%d] %s\n", step, total, title)
}

const (
	statsCodePrereqMissing     = "WOLT_STATS_PREREQ_MISSING"
	statsCodeEnvError          = "WOLT_STATS_ENV_ERROR"
	statsCodeBundleUnavailable = "WOLT_STATS_BUNDLE_UNAVAILABLE"
	envStatsDir                = "WOLT_STATS_DIR"
)

// newStatsCommand returns the top-level `wolt stats` command: a single-step
// flow that fetches the dashboard bundle, syncs order history, starts a
// localhost HTTP server, and opens the browser.
func newStatsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	opts := statsFlags{
		Port: statsserve.DefaultPort,
	}

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Open the wolt-stats local dashboard.",
		Long: "Download the latest wolt-stats bundle, sync your order history into a local SQLite database, " +
			"serve the dashboard at http://127.0.0.1:5173, and open the browser.\n\n" +
			"The dashboard, database, and cache live under ~/.wolt/stats (override with --stats-dir or $WOLT_STATS_DIR).\n" +
			"Re-running is incremental — pass --resync to force a full history rescan.\n" +
			"Pass --no-sync to skip the sync (useful after data is already there).\n" +
			"Pass --no-open if you just want the server running and the URL printed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStats(cmd, deps, flags, opts)
		},
	}

	cmd.Flags().StringVar(&opts.StatsDir, "stats-dir", "", "Override the wolt-stats install dir (env WOLT_STATS_DIR; default ~/.wolt/stats).")
	cmd.Flags().IntVar(&opts.Port, "port", statsserve.DefaultPort, "Preferred HTTP server port (auto-picks +1 if busy).")
	cmd.Flags().BoolVar(&opts.NoOpen, "no-open", false, "Do not open the browser.")
	cmd.Flags().BoolVar(&opts.NoSync, "no-sync", false, "Skip the order history sync (use whatever DB is already on disk).")
	cmd.Flags().BoolVar(&opts.Resync, "resync", false, "Force a full re-sync of order history instead of incremental.")
	cmd.Flags().BoolVar(&opts.NoCheckUpdates, "no-check-updates", false, "Do not query GitHub for a newer wolt-stats bundle.")
	cmd.Flags().StringVar(&opts.BundleVersion, "bundle-version", "", "Pin to a specific wolt-stats release tag (e.g. v0.1.0).")
	addGlobalFlags(cmd, &flags)
	return cmd
}

type statsFlags struct {
	StatsDir       string
	Port           int
	NoOpen         bool
	NoSync         bool
	Resync         bool
	NoCheckUpdates bool
	BundleVersion  string
}

func runStats(cmd *cobra.Command, deps Dependencies, flags globalFlags, opts statsFlags) error {
	ctx := cmd.Context()
	format, err := parseOutputFormat(flags.Format)
	if err != nil {
		return err
	}
	profileName := defaultProfileName(flags.Profile)
	statsDir, err := resolveStatsDir(opts.StatsDir)
	if err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, err.Error())
	}

	// Progress: stream phase + detail lines to stderr in table mode so the
	// user sees what's happening during the long bundle download and sync.
	// JSON / YAML mode stays clean — those consumers want a deterministic
	// envelope, not a chatty log stream.
	var progress io.Writer
	totalSteps := 2 // bundle + serve
	if !opts.NoSync {
		totalSteps = 3 // bundle + sync + serve
	}
	if format == output.FormatTable {
		progress = cmd.ErrOrStderr()
	}

	writeStep(progress, 1, totalSteps, "Resolving dashboard bundle")
	warnings := []string{}

	manager, err := statsbundle.New(statsDir)
	if err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, err.Error())
	}

	bundle, err := manager.EnsureBundle(ctx, statsbundle.EnsureOptions{
		PinnedVersion:   strings.TrimSpace(opts.BundleVersion),
		SkipUpdateCheck: opts.NoCheckUpdates,
		Progress:        progress,
	})
	if err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeBundleUnavailable, err.Error())
	}

	dbPath := filepath.Join(statsDir, "db", "wolt-history.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, fmt.Sprintf("create db dir: %v", err))
	}

	var (
		email     string
		syncStats *statssync.Result
	)
	// Always run schema + migration setup, even when the caller passes
	// --no-sync. The legacy-data repair migration lives in openStore;
	// skipping it would leave post-upgrade users staring at the same
	// stale dashboard they came here to fix.
	if err := statssync.EnsureSchema(ctx, dbPath); err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, fmt.Sprintf("prepare stats database: %v", err))
	}
	if !opts.NoSync {
		writeStep(progress, 2, totalSteps, "Syncing your Wolt order history")
		auth, err := loadRequiredAuth(ctx, deps, flags, format, cmd)
		if err != nil {
			return err
		}
		tokenPersistence := newCredentialPersistence(
			ctx,
			deps,
			auth,
			allowAutomaticCredentialPersistence(flags),
		)
		userPayload, authWarnings, userErr := invokeWithAuthAutoRefreshUsingPersistence(
			ctx,
			deps,
			flags,
			&auth,
			func(authCtx woltgateway.AuthContext) (map[string]any, error) {
				return deps.Wolt.UserMe(ctx, authCtx)
			},
			tokenPersistence,
		)
		warnings = append(warnings, authWarnings...)
		if userErr != nil {
			return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, userErr, authWarnings...)
		}
		user := asMap(userPayload["user"])
		email = strings.TrimSpace(asString(user["email"]))
		if email == "" {
			return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, "could not resolve your Wolt email from the saved session; run \"wolt login\" again")
		}

		res, syncErr := statssync.Sync(ctx, deps.Wolt, statssync.Options{
			DBPath:      dbPath,
			UserEmail:   email,
			ProfileName: profileName,
			Auth:        auth,
			ForceFull:   opts.Resync,
			Progress:    progress,
			Verbose:     flags.Verbose,
			Refresher:   deps.Wolt.RefreshAccessToken,
			OnAccessTokenRefreshed: func(updated woltgateway.AuthContext) error {
				attempted, persisted, persistErr := tokenPersistence.persistAccess(
					ctx,
					updated.WToken,
				)
				if persistErr != nil {
					return persistErr
				}
				if attempted && !persisted {
					return fmt.Errorf("saved credentials changed concurrently")
				}
				return nil
			},
		})
		if syncErr != nil {
			return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, syncErr.Error())
		}
		syncStats = &res
	}

	serveStep := totalSteps
	writeStep(progress, serveStep, totalSteps, "Starting dashboard server")
	server, err := statsserve.Start(statsserve.Options{
		BundleDir: bundle.Path,
		DBPath:    dbPath,
		Port:      opts.Port,
	})
	if err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, statsCodeEnvError, err.Error())
	}
	if progress != nil {
		_, _ = fmt.Fprintf(progress, "    Listening on %s\n", server.URL())
	}

	data := map[string]any{
		"stats_dir": statsDir,
		"bundle": map[string]any{
			"version":     bundle.Version,
			"source":      bundle.Source,
			"downloaded":  bundle.Downloaded,
			"active_path": bundle.Path,
		},
		"db_path": dbPath,
		"server": map[string]any{
			"url":  server.URL(),
			"port": server.Port(),
			"pid":  os.Getpid(),
		},
	}
	if email != "" {
		data["email"] = email
	}
	if syncStats != nil {
		data["sync"] = syncSummary(*syncStats)
	}

	if format == output.FormatTable {
		if err := writeTable(cmd, buildStatsTable(data), flags.Output); err != nil {
			_ = server.Shutdown(context.Background())
			return err
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nDashboard ready. Press Ctrl-C to stop.\n")
	} else {
		env := output.BuildEnvelope(profileName, flags.Locale, data, warnings, nil)
		if err := writeMachinePayload(cmd, env, format, flags.Output); err != nil {
			_ = server.Shutdown(context.Background())
			return err
		}
	}

	if !opts.NoOpen {
		go func() {
			// Tiny grace period so the URL appears in the terminal before
			// the browser tab pops up.
			time.Sleep(150 * time.Millisecond)
			if err := openBrowser(ctx, server.URL()); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not auto-open browser: %v\n", err)
			}
		}()
	}

	return waitForShutdown(ctx, cmd, server)
}

func resolveStatsDir(override string) (string, error) {
	dir := strings.TrimSpace(override)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv(envStatsDir))
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".wolt", "stats")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create stats dir: %w", err)
	}
	return abs, nil
}

func syncSummary(r statssync.Result) map[string]any {
	out := map[string]any{
		"performed":           true,
		"mode":                r.Mode,
		"pages_fetched":       r.PagesFetched,
		"orders_scanned":      r.OrdersScanned,
		"details_fetched":     r.DetailsFetched,
		"inserted_orders":     r.InsertedOrders,
		"updated_orders":      r.UpdatedOrders,
		"catalog_count":       r.CatalogCount,
		"detail_count":        r.DetailCount,
		"reached_history_end": r.ReachedHistoryEnd,
		"duration_ms":         r.DurationMs,
	}
	if strings.TrimSpace(r.StopReason) != "" {
		out["stop_reason"] = r.StopReason
	}
	return out
}

func buildStatsTable(data map[string]any) string {
	bundle := asMap(data["bundle"])
	server := asMap(data["server"])
	sync := asMap(data["sync"])

	rows := [][]string{
		{"Stats dir", fallbackString(asString(data["stats_dir"]), "-")},
		{"Bundle version", fallbackString(asString(bundle["version"]), "-")},
		{"Bundle source", fallbackString(asString(bundle["source"]), "-")},
		{"Bundle downloaded", boolToYesNo(asBool(bundle["downloaded"]))},
		{"Database", fallbackString(asString(data["db_path"]), "-")},
	}
	if email := asString(data["email"]); email != "" {
		rows = append(rows, []string{"Account", email})
	}
	if sync != nil {
		rows = append(rows, []string{"Sync mode", fallbackString(asString(sync["mode"]), "-")})
		rows = append(rows, []string{"Orders scanned", fmt.Sprintf("%d", asInt(sync["orders_scanned"]))})
		rows = append(rows, []string{"Details fetched", fmt.Sprintf("%d", asInt(sync["details_fetched"]))})
		rows = append(rows, []string{"New orders", fmt.Sprintf("%d", asInt(sync["inserted_orders"]))})
		rows = append(rows, []string{"Updated orders", fmt.Sprintf("%d", asInt(sync["updated_orders"]))})
		if reason := asString(sync["stop_reason"]); reason != "" {
			rows = append(rows, []string{"Stop reason", reason})
		}
	}
	rows = append(rows,
		[]string{"Server PID", fmt.Sprintf("%d", asInt(server["pid"]))},
		[]string{"Dashboard", fallbackString(asString(server["url"]), "-")},
	)
	return output.RenderTable("Wolt stats", []string{"Field", "Value"}, rows)
}

func waitForShutdown(ctx context.Context, cmd *cobra.Command, server *statsserve.Server) error {
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Wait() }()

	select {
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Shutdown error: %v\n", err)
		}
		// Drain background Wait so the goroutine exits.
		<-serverDone
		return &exitError{code: 130}
	case err := <-serverDone:
		if err != nil {
			return emitError(cmd, output.FormatTable, "default", "", "", statsCodeEnvError, err.Error())
		}
		return nil
	}
}
