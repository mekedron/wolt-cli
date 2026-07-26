// Package statssync synchronises a Wolt account's order history into the
// SQLite database the wolt-stats dashboard reads. It is a pure-Go port of
// wolt-stats/scripts/sync-wolt-history.mjs + wolt-sync-db.mjs, sharing the
// same schema and stop-decision semantics so the resulting file is
// byte-compatible with the Node implementation.
package statssync

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// DefaultPageSize matches sync-wolt-history.mjs:40 and the Wolt API's max.
const DefaultPageSize = 50

// DefaultRateLimit is the inter-call gap. The Node implementation used 650ms
// but observation against real Wolt shows ~1 in 5 detail fetches gets a 429
// at that rate. 1.1s sustains ~0.9 req/s, which empirically clears the
// per-token throttle while still finishing 1k orders in ~20min.
const DefaultRateLimit = 1100 * time.Millisecond

// MaxPacingExtra caps how much the adaptive pacer can add on top of
// DefaultRateLimit when 429s happen. 5s + base 1.1s ≈ 6.1s per call;
// beyond that we likely have a deeper problem than throttling.
const MaxPacingExtra = 5 * time.Second

// PacingBumpOn429 is how much each rate-limited response adds to the
// adaptive extra delay for the remainder of the run.
const PacingBumpOn429 = 500 * time.Millisecond

// WoltClient is the slice of the wolt gateway statssync needs. The CLI
// passes deps.Wolt directly; tests pass a fake.
type WoltClient interface {
	OrderHistory(ctx context.Context, auth woltgateway.AuthContext, opts woltgateway.OrderHistoryOptions) (map[string]any, error)
	OrderHistoryPurchase(ctx context.Context, purchaseID string, auth woltgateway.AuthContext) (map[string]any, error)
}

// Refresher swaps a refresh token for a new access/refresh pair. The
// CLI wires this to deps.Wolt.RefreshAccessToken; tests pass a fake.
// Optional — when nil, statssync surfaces 401 responses directly
// instead of refreshing.
type Refresher func(ctx context.Context, refreshToken string, auth woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error)

// Options controls a single Sync run.
type Options struct {
	DBPath      string
	UserEmail   string
	UserID      string // optional override; defaults to slug of UserEmail
	UserLabel   string // optional display label; defaults to email's local part
	ProfileName string // for sync_runs.profile_name; defaults to "default"
	Auth        woltgateway.AuthContext
	PageSize    int
	ForceFull   bool
	RateLimit   time.Duration // sleep between Wolt calls; defaults to DefaultRateLimit
	Now         func() time.Time
	// Progress, if non-nil, receives plain-text status lines so users can
	// watch the sync work through pages and orders. Lines are prefixed
	// with "==> " for phases and "    " for in-phase detail.
	Progress io.Writer
	// Verbose enables per-page / per-order detail lines. Without it, only
	// phase banners and a final summary are emitted.
	Verbose bool
	// Refresher, when non-nil, lets the sync swap an expired access token
	// for a fresh one mid-run using the refresh token in Auth. The sync
	// is long enough (hours, for large histories) that the original
	// access token will expire before completion.
	Refresher Refresher
	// OnAccessTokenRefreshed, when non-nil, is called after a successful
	// refresh so the caller can safely persist the new access token.
	OnAccessTokenRefreshed func(woltgateway.AuthContext) error
	// sleep is the inter-call pacer and backoff sleeper. Tests override it
	// to record requested durations without waiting. Production leaves it
	// nil, which defaults to sleepCtx.
	sleep func(context.Context, time.Duration) error
	// backoff overrides the default 429/503 retry policy. Tests use this to
	// keep MaxAttempts low; production uses the package default.
	backoff *backoffPolicy
}

// Result summarises a Sync run. Field names match the JSON envelope the
// orchestrator emits.
type Result struct {
	Mode              string `json:"mode"`
	PagesFetched      int    `json:"pages_fetched"`
	OrdersScanned     int    `json:"orders_scanned"`
	DetailsFetched    int    `json:"details_fetched"`
	InsertedOrders    int    `json:"inserted_orders"`
	UpdatedOrders     int    `json:"updated_orders"`
	CatalogCount      int    `json:"catalog_count"`
	DetailCount       int    `json:"detail_count"`
	StopReason        string `json:"stop_reason,omitempty"`
	ReachedHistoryEnd bool   `json:"reached_history_end"`
	DurationMs        int64  `json:"duration_ms"`
}

// EnsureSchema opens the SQLite at dbPath solely to apply the schema
// and the idempotent legacy-data repair migration, then closes it.
// Useful when the caller wants the dashboard to read a fully-migrated
// DB without running the (potentially long) catalog + detail sync —
// for example, "wolt stats --no-sync" needs this so post-upgrade
// users see backfilled amounts even before they re-sync.
func EnsureSchema(ctx context.Context, dbPath string) error {
	db, err := openStore(ctx, dbPath)
	if err != nil {
		return err
	}
	return db.Close()
}

// Sync runs the catalog + detail phases, returning a Result describing
// what changed. Sync writes incrementally; if ctx is cancelled mid-run the
// already-committed transactions remain on disk and a subsequent call
// resumes from there.
func Sync(ctx context.Context, client WoltClient, opts Options) (Result, error) {
	if client == nil {
		return Result{}, errors.New("statssync: WoltClient is required")
	}
	email := strings.ToLower(strings.TrimSpace(opts.UserEmail))
	if email == "" {
		return Result{}, errors.New("statssync: UserEmail is required")
	}
	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = slugify(email)
	}
	if userID == "" {
		return Result{}, errors.New("statssync: could not derive user id from email")
	}
	label := strings.TrimSpace(opts.UserLabel)
	if label == "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			label = email[:at]
		} else {
			label = email
		}
	}
	profileName := strings.TrimSpace(opts.ProfileName)
	if profileName == "" {
		profileName = "default"
	}
	pageSize := opts.PageSize
	if pageSize <= 0 || pageSize > DefaultPageSize {
		pageSize = DefaultPageSize
	}
	rateLimit := opts.RateLimit
	if rateLimit <= 0 {
		rateLimit = DefaultRateLimit
	}
	clock := opts.Now
	if clock == nil {
		clock = time.Now
	}
	sleep := opts.sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	backoff := defaultBackoff
	if opts.backoff != nil {
		backoff = *opts.backoff
	}
	pacer := newAdjustablePacer(rateLimit, PacingBumpOn429, MaxPacingExtra, opts.Progress)

	// authCtx is mutated in place when a refresh swaps the access token.
	// runCatalogPhase / runDetailPhase pass a pointer down so the closures
	// they hand to callWithAuthAndBackoff always see the latest value.
	authCtx := opts.Auth
	refreshAuth := buildRefreshHook(
		&authCtx,
		opts.Refresher,
		opts.OnAccessTokenRefreshed,
		opts.Progress,
	)

	db, err := openStore(ctx, opts.DBPath)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = db.Close() }()

	startedAt := clock().UTC()
	startedISO := startedAt.Format(time.RFC3339)

	if err := withTx(ctx, db, func(tx *sql.Tx) error {
		return upsertUser(ctx, tx, userID, email, label, startedAt)
	}); err != nil {
		return Result{}, fmt.Errorf("statssync: upsert user: %w", err)
	}

	knownCount, err := getCatalogCount(ctx, db, userID)
	if err != nil {
		return Result{}, fmt.Errorf("statssync: catalog count: %w", err)
	}
	newestKnown, err := getNewestCatalogPaymentTimeTs(ctx, db, userID)
	if err != nil {
		return Result{}, fmt.Errorf("statssync: newest payment ts: %w", err)
	}
	mode := resolveCatalogScanMode(opts.ForceFull, knownCount, newestKnown)
	knownIDs := map[string]struct{}{}
	if mode == "incremental" {
		knownIDs, err = loadKnownPurchaseIDs(ctx, db, userID)
		if err != nil {
			return Result{}, fmt.Errorf("statssync: load known ids: %w", err)
		}
	}

	writePhase(opts.Progress, "Syncing order history for %s (%s mode)", email, mode)
	if mode == "incremental" {
		writeDetail(opts.Progress, "%d orders already known; scanning until we hit one of them", knownCount)
	}
	catalog, err := runCatalogPhase(ctx, client, db, &authCtx, catalogParams{
		UserID:               userID,
		PageSize:             pageSize,
		Mode:                 mode,
		KnownPurchaseIDs:     knownIDs,
		NewestKnownPaymentTs: newestKnown,
		Clock:                clock,
		Progress:             opts.Progress,
		Verbose:              opts.Verbose,
		Sleep:                sleep,
		Backoff:              backoff,
		Pacer:                pacer,
		RefreshAuth:          refreshAuth,
	})
	if err != nil {
		return Result{}, err
	}
	writeDetail(opts.Progress, "Catalog phase: %d pages, %d summaries scanned (%s)", catalog.PagesFetched, catalog.OrdersScanned, formatStopReason(catalog.StopReason))

	detail, err := runDetailPhase(ctx, client, db, &authCtx, detailParams{
		UserID:      userID,
		ForceFull:   opts.ForceFull,
		Clock:       clock,
		Progress:    opts.Progress,
		Verbose:     opts.Verbose,
		Sleep:       sleep,
		Backoff:     backoff,
		Pacer:       pacer,
		RefreshAuth: refreshAuth,
	})
	if err != nil {
		return Result{}, err
	}
	if detail.Queued == 0 {
		writeDetail(opts.Progress, "Detail phase: no missing order details")
	} else {
		writeDetail(opts.Progress, "Detail phase: %d fetched (%d new, %d updated)", detail.Fetched, detail.Inserted, detail.Updated)
	}

	finishedAt := clock().UTC()
	catalogCount, _ := getCatalogCount(ctx, db, userID)
	detailCount, _ := getDetailCount(ctx, db, userID)

	if err := withTx(ctx, db, func(tx *sql.Tx) error {
		newestPaymentTs, txErr := tx.QueryContext(ctx, `SELECT COALESCE(MAX(payment_time_ts), 0) FROM order_catalog WHERE user_id = ?`, userID)
		if txErr != nil {
			return txErr
		}
		var newest int
		for newestPaymentTs.Next() {
			var v sql.NullInt64
			if scanErr := newestPaymentTs.Scan(&v); scanErr != nil {
				_ = newestPaymentTs.Close()
				return scanErr
			}
			if v.Valid {
				newest = int(v.Int64)
			}
		}
		_ = newestPaymentTs.Close()

		if err := updateSyncState(ctx, tx, userID, syncStateUpdate{
			NewestPaymentTimeTs:   newest,
			FullBackfillCompleted: catalogCount > 0 && detailCount >= catalogCount,
			CatalogOrderCount:     catalogCount,
			DetailOrderCount:      detailCount,
			ExpectedOrderCount:    sql.NullInt64{},
			StartedAt:             startedISO,
		}, finishedAt); err != nil {
			return err
		}
		return insertSyncRun(ctx, tx, userID, profileName, runRecord{
			StartedAt:         startedISO,
			FinishedAt:        finishedAt.Format(time.RFC3339),
			PagesFetched:      catalog.PagesFetched,
			OrdersScanned:     catalog.OrdersScanned,
			DetailsFetched:    detail.Fetched,
			InsertedOrders:    detail.Inserted,
			UpdatedOrders:     detail.Updated,
			ReachedHistoryEnd: catalog.StopReason == "",
		})
	}); err != nil {
		return Result{}, fmt.Errorf("statssync: finalize sync state: %w", err)
	}

	return Result{
		Mode:              mode,
		PagesFetched:      catalog.PagesFetched,
		OrdersScanned:     catalog.OrdersScanned,
		DetailsFetched:    detail.Fetched,
		InsertedOrders:    detail.Inserted,
		UpdatedOrders:     detail.Updated,
		CatalogCount:      catalogCount,
		DetailCount:       detailCount,
		StopReason:        catalog.StopReason,
		ReachedHistoryEnd: catalog.StopReason == "",
		DurationMs:        finishedAt.Sub(startedAt).Milliseconds(),
	}, nil
}

func resolveCatalogScanMode(forceFull bool, knownCount, newestKnownTs int) string {
	if forceFull {
		return "full"
	}
	if knownCount <= 0 || newestKnownTs <= 0 {
		return "full"
	}
	return "incremental"
}

type catalogParams struct {
	UserID               string
	PageSize             int
	Mode                 string
	KnownPurchaseIDs     map[string]struct{}
	NewestKnownPaymentTs int
	Clock                func() time.Time
	Progress             io.Writer
	Verbose              bool
	Sleep                func(context.Context, time.Duration) error
	Backoff              backoffPolicy
	Pacer                *adjustablePacer
	RefreshAuth          func(context.Context) error
}

type catalogResult struct {
	PagesFetched  int
	OrdersScanned int
	StopReason    string
}

func runCatalogPhase(ctx context.Context, client WoltClient, db *sql.DB, auth *woltgateway.AuthContext, p catalogParams) (catalogResult, error) {
	var (
		result        catalogResult
		nextPageToken string
		seenTokens    = map[string]struct{}{}
	)
	incremental := p.Mode == "incremental"

	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := p.Pacer.wait(ctx, sleep); err != nil {
			return result, err
		}
		pageNumber := result.PagesFetched + 1
		page, err := callWithAuthAndBackoff(
			ctx,
			func(ctx context.Context) (map[string]any, error) {
				return client.OrderHistory(ctx, *auth, woltgateway.OrderHistoryOptions{
					Limit:     p.PageSize,
					PageToken: nextPageToken,
				})
			},
			p.RefreshAuth,
			sleep,
			p.Backoff,
			p.Pacer,
			p.Progress,
			fmt.Sprintf("order history page %d", pageNumber),
		)
		if err != nil {
			return result, fmt.Errorf("statssync: order history list: %w", err)
		}
		summaries := asSlice(page["orders"])
		if len(summaries) == 0 {
			break
		}
		result.PagesFetched++
		result.OrdersScanned += len(summaries)

		if p.Verbose {
			writeDetail(p.Progress, "Catalog page %d: %d order summaries", result.PagesFetched, len(summaries))
		} else {
			writeDetail(p.Progress, "Catalog page %d: %d summaries (running total %d)", result.PagesFetched, len(summaries), result.OrdersScanned)
		}

		now := p.Clock().UTC()
		if err := withTx(ctx, db, func(tx *sql.Tx) error {
			for _, raw := range summaries {
				summary := asMap(raw)
				purchaseID := strings.TrimSpace(asString(summary["purchase_id"]))
				if purchaseID == "" {
					continue
				}
				if err := upsertOrderCatalogEntry(ctx, tx, p.UserID, purchaseID, summary, now); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return result, fmt.Errorf("statssync: persist catalog page: %w", err)
		}

		stop := ""
		if incremental {
			stop = catalogStopDecision(summaries, p.KnownPurchaseIDs, p.NewestKnownPaymentTs)
		}
		if stop != "" {
			result.StopReason = stop
			break
		}
		pageToken := strings.TrimSpace(asString(page["next_page_token"]))
		if pageToken == "" {
			break
		}
		if _, seen := seenTokens[pageToken]; seen {
			return result, fmt.Errorf("statssync: pagination repeated next_page_token %q", pageToken)
		}
		seenTokens[pageToken] = struct{}{}
		nextPageToken = pageToken
	}
	return result, nil
}

// catalogStopDecision mirrors getCatalogStopDecision in
// wolt-stats/scripts/lib/wolt-sync-catalog.mjs. It returns an empty string
// when the loop should continue, or one of {"known_purchase",
// "checkpoint_reached"} when it should stop after the current page.
func catalogStopDecision(summaries []any, known map[string]struct{}, newestKnownTs int) string {
	if len(summaries) == 0 || newestKnownTs <= 0 {
		return ""
	}
	for _, raw := range summaries {
		summary := asMap(raw)
		id := strings.TrimSpace(asString(summary["purchase_id"]))
		if id == "" {
			continue
		}
		if _, ok := known[id]; ok {
			return "known_purchase"
		}
	}
	for _, raw := range summaries {
		summary := asMap(raw)
		ts := asInt(summary["payment_time_ts"])
		if ts > 0 && ts < newestKnownTs {
			return "checkpoint_reached"
		}
	}
	return ""
}

type detailParams struct {
	UserID      string
	ForceFull   bool
	Clock       func() time.Time
	Progress    io.Writer
	Verbose     bool
	Sleep       func(context.Context, time.Duration) error
	Backoff     backoffPolicy
	Pacer       *adjustablePacer
	RefreshAuth func(context.Context) error
}

type detailResult struct {
	Queued   int
	Fetched  int
	Inserted int
	Updated  int
}

func runDetailPhase(ctx context.Context, client WoltClient, db *sql.DB, auth *woltgateway.AuthContext, p detailParams) (detailResult, error) {
	queue, err := loadDetailQueue(ctx, db, p.UserID, p.ForceFull)
	if err != nil {
		return detailResult{}, err
	}
	result := detailResult{Queued: len(queue)}
	if len(queue) == 0 {
		return result, nil
	}
	writeDetail(p.Progress, "Fetching %d order detail%s", len(queue), plural(len(queue)))
	existing, err := loadKnownDetailedOrders(ctx, db, p.UserID)
	if err != nil {
		return detailResult{}, err
	}

	// Progress cadence: log every detail in --verbose, otherwise every 5%
	// of the queue OR every 10 seconds, whichever comes first. The time
	// floor is what keeps long throttle/backoff windows from looking like
	// a hung process.
	tick := len(queue) / 20
	if tick < 10 {
		tick = 10
	}
	const progressMaxGap = 10 * time.Second
	lastProgressAt := p.Clock()

	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	for i, entry := range queue {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := p.Pacer.wait(ctx, sleep); err != nil {
			return result, err
		}
		detail, err := callWithAuthAndBackoff(
			ctx,
			func(ctx context.Context) (map[string]any, error) {
				return client.OrderHistoryPurchase(ctx, entry.PurchaseID, *auth)
			},
			p.RefreshAuth,
			sleep,
			p.Backoff,
			p.Pacer,
			p.Progress,
			fmt.Sprintf("order detail %s (%d/%d)", entry.PurchaseID, i+1, len(queue)),
		)
		if err != nil {
			return result, fmt.Errorf("statssync: order detail %s: %w (processed %d of %d so far; rerun \"wolt stats\" later to resume)", entry.PurchaseID, err, i, len(queue))
		}
		now := p.Clock().UTC()
		if err := withTx(ctx, db, func(tx *sql.Tx) error {
			return upsertOrderBundle(ctx, tx, p.UserID, entry.PurchaseID, entry.Summary, detail, now)
		}); err != nil {
			return result, fmt.Errorf("statssync: persist detail %s: %w", entry.PurchaseID, err)
		}
		result.Fetched++
		if _, ok := existing[entry.PurchaseID]; ok {
			result.Updated++
		} else {
			result.Inserted++
			existing[entry.PurchaseID] = struct{}{}
		}

		idx := i + 1
		nowWall := p.Clock()
		switch {
		case p.Verbose:
			writeDetail(p.Progress, "  %d/%d %s", idx, len(queue), entry.PurchaseID)
			lastProgressAt = nowWall
		case idx == 1 || idx == len(queue) || idx%tick == 0 || nowWall.Sub(lastProgressAt) >= progressMaxGap:
			writeDetail(p.Progress, "  %d/%d details fetched", idx, len(queue))
			lastProgressAt = nowWall
		}
	}
	return result, nil
}

func loadKnownDetailedOrders(ctx context.Context, db *sql.DB, userID string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT purchase_id FROM orders WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// --- parse helpers ---

type localDateTime struct {
	Date     string
	Datetime string
	Month    string
	Weekday  int
	Hour     int
	Valid    bool
}

var localDateTimePattern = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4}),\s*(\d{2}):(\d{2})$`)

// parseLocalDateTime mirrors parseLocalDateTime in
// wolt-stats/scripts/lib/wolt-sync-db.mjs:812. Wolt's creation_time is the
// user's local clock in dd/mm/yyyy, hh:mm — not a wire timestamp — so this
// is the documented parser.
func parseLocalDateTime(raw string) localDateTime {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return localDateTime{}
	}
	m := localDateTimePattern.FindStringSubmatch(raw)
	if m == nil {
		return localDateTime{}
	}
	day, month, year, hour, minute := m[1], m[2], m[3], m[4], m[5]
	date := year + "-" + month + "-" + day
	hourInt, _ := strconv.Atoi(hour)
	dayInt, _ := strconv.Atoi(day)
	monthInt, _ := strconv.Atoi(month)
	yearInt, _ := strconv.Atoi(year)
	weekday := int(time.Date(yearInt, time.Month(monthInt), dayInt, 12, 0, 0, 0, time.UTC).Weekday())
	return localDateTime{
		Date:     date,
		Datetime: date + "T" + hour + ":" + minute + ":00",
		Month:    year + "-" + month,
		Weekday:  weekday,
		Hour:     hourInt,
		Valid:    true,
	}
}

// slugify mirrors the Node `slugify` so a given email maps to the same
// canonical user_id regardless of which implementation wrote the row.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" && s != "" {
		// Fall back to a stable hash when the input has zero ASCII alnum
		// (rare — e.g., all-Cyrillic email local-parts that survive Wolt).
		sum := sha1.Sum([]byte(s))
		out = "u" + hex.EncodeToString(sum[:6])
	}
	return out
}

// extractDetailMinor looks up an amount on the detail payload, preferring
// the flat top-level integer field Wolt actually returns
// (e.g. detail["total_price"] = 1609) and falling back to the legacy
// nested shape detail["totals"][totalsKey] used by older fixtures /
// hypothetical schemas. Returning zero from a flat hit still falls
// through to the nested map, so "credits: 0 + totals.credits: 500"
// yields 500 if anyone ever ships both.
func extractDetailMinor(detail map[string]any, flatKey, totalsKey string) int {
	if v, ok := detail[flatKey]; ok && v != nil {
		if n := extractMinor(v); n != 0 {
			return n
		}
	}
	totals := asMap(detail["totals"])
	if totals == nil {
		return 0
	}
	return extractMinor(totals[totalsKey])
}

// firstNonNilString returns the first map value at any of keys that is a
// non-blank string (after trim). Used to coalesce a flat top-level field
// with its nested-map equivalent, e.g.
// detail.venue_country → venue.country.
func firstNonNilString(sources []map[string]any, keys ...string) interface{} {
	for _, src := range sources {
		if src == nil {
			continue
		}
		for _, k := range keys {
			if v := nullableString(src[k]); v != nil {
				return v
			}
		}
	}
	return nil
}

func extractMinor(candidate any) int {
	if candidate == nil {
		return 0
	}
	switch v := candidate.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case map[string]any:
		if amt, ok := v["amount"]; ok {
			switch a := amt.(type) {
			case int:
				return a
			case int64:
				return int(a)
			case float64:
				return int(a)
			case map[string]any:
				return extractMinor(a)
			}
		}
		if value, ok := v["value"]; ok {
			if m, ok := value.(map[string]any); ok {
				if amt, ok := m["amount"]; ok {
					if n, ok := amt.(float64); ok {
						return int(n)
					}
				}
			}
		}
		if lt, ok := v["line_total"]; ok {
			if m, ok := lt.(map[string]any); ok {
				if amt, ok := m["amount"]; ok {
					if n, ok := amt.(float64); ok {
						return int(n)
					}
				}
			}
		}
	}
	return 0
}

func sumCollectionAmounts(value any) int {
	total := 0
	for _, raw := range asSlice(value) {
		total += extractMinor(raw)
	}
	return total
}

func asMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if s, ok := value.([]any); ok {
		return s
	}
	return nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func asIntOr(value any, fallback int) int {
	if value == nil {
		return fallback
	}
	if n := asInt(value); n != 0 {
		return n
	}
	if _, ok := value.(float64); ok {
		return asInt(value)
	}
	return fallback
}

func nullableString(value any) interface{} {
	if value == nil {
		return nil
	}
	s := strings.TrimSpace(asString(value))
	if s == "" {
		return nil
	}
	return s
}

func nullableStringFromMap(m map[string]any, key string) interface{} {
	if m == nil {
		return nil
	}
	return nullableString(m[key])
}

func nullableTime(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func stringOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
