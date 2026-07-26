package statssync

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	_ "modernc.org/sqlite"
)

func TestSyncCreatesSchemaAndInsertsOrders(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	res, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond, // keep tests fast
		Now:       fixedClock(),
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if res.OrdersScanned != 2 {
		t.Fatalf("expected 2 orders scanned, got %d", res.OrdersScanned)
	}
	if res.DetailsFetched != 2 {
		t.Fatalf("expected 2 details fetched, got %d", res.DetailsFetched)
	}
	if res.InsertedOrders != 2 {
		t.Fatalf("expected 2 inserted, got %d", res.InsertedOrders)
	}
	if res.UpdatedOrders != 0 {
		t.Fatalf("expected 0 updated on cold start, got %d", res.UpdatedOrders)
	}
	if res.Mode != "full" {
		t.Fatalf("expected mode=full on cold start, got %q", res.Mode)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open verify db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Schema-level invariants.
	for _, table := range []string{"users", "sync_state", "sync_runs", "order_catalog", "orders", "order_items", "order_item_option_values", "order_payments"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatalf("schema lookup %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	// User row exists and is keyed by slug of email.
	var (
		userID    string
		userLabel string
	)
	if err := db.QueryRow(`SELECT id, label FROM users`).Scan(&userID, &userLabel); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if userID != "user-example-com" {
		t.Fatalf("expected slugged user id, got %q", userID)
	}
	if userLabel != "user" {
		t.Fatalf("expected user label 'user', got %q", userLabel)
	}

	// orders rows
	var ordersCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM orders WHERE user_id=?`, userID).Scan(&ordersCount); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if ordersCount != 2 {
		t.Fatalf("expected 2 orders, got %d", ordersCount)
	}

	// items + payments
	var itemsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_items WHERE user_id=?`, userID).Scan(&itemsCount); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemsCount != 3 {
		t.Fatalf("expected 3 item rows (1+2), got %d", itemsCount)
	}

	// derived fields: order_local_date for the second order
	var date string
	if err := db.QueryRow(`SELECT order_local_date FROM orders WHERE purchase_id='p2'`).Scan(&date); err != nil {
		t.Fatalf("read date p2: %v", err)
	}
	if date != "2026-05-21" {
		t.Fatalf("expected date 2026-05-21, got %q", date)
	}

	// sync_runs has exactly one row capturing this run
	var runs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_runs`).Scan(&runs); err != nil {
		t.Fatalf("count sync_runs: %v", err)
	}
	if runs != 1 {
		t.Fatalf("expected 1 sync_runs row, got %d", runs)
	}
}

func TestSyncIncrementalSkipsKnownOrders(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	if _, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
	}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	client.resetCounts()
	res, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
	})
	if err != nil {
		t.Fatalf("incremental Sync: %v", err)
	}
	if res.Mode != "incremental" {
		t.Fatalf("expected mode=incremental, got %q", res.Mode)
	}
	if res.DetailsFetched != 0 {
		t.Fatalf("expected 0 details on warm rerun, got %d", res.DetailsFetched)
	}
	// One page fetched so we can detect the known purchase; no further calls.
	if client.listCalls != 1 {
		t.Fatalf("expected 1 list call on warm rerun, got %d", client.listCalls)
	}
	if client.purchaseCalls != 0 {
		t.Fatalf("expected 0 purchase calls on warm rerun, got %d", client.purchaseCalls)
	}
	if res.StopReason != "known_purchase" {
		t.Fatalf("expected stop reason known_purchase, got %q", res.StopReason)
	}
}

func TestSyncForceFullRefetchesDetails(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	if _, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
	}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	client.resetCounts()
	res, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		ForceFull: true,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
	})
	if err != nil {
		t.Fatalf("force-full Sync: %v", err)
	}
	if res.Mode != "full" {
		t.Fatalf("expected mode=full, got %q", res.Mode)
	}
	if res.DetailsFetched != 2 {
		t.Fatalf("expected 2 details refetched, got %d", res.DetailsFetched)
	}
	if res.UpdatedOrders != 2 {
		t.Fatalf("expected 2 updates on force-full rerun, got %d", res.UpdatedOrders)
	}
}

func TestSyncRecoversFrom429DuringDetailPhase(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	client.purchaseFailures = map[string]*purchaseFailureSpec{
		"p1": {Remaining: 2, Status: 429, RetryAfter: 2 * time.Second},
	}
	sleeper := &recordingSleeper{}
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	res, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
		sleep:     sleeper.sleep,
		backoff:   &backoffPolicy{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: 60 * time.Second},
	})
	if err != nil {
		t.Fatalf("Sync should recover from 429, got %v", err)
	}
	if res.DetailsFetched != 2 {
		t.Fatalf("expected 2 details after recovery, got %d", res.DetailsFetched)
	}
	if res.InsertedOrders != 2 {
		t.Fatalf("expected 2 inserts after recovery, got %d", res.InsertedOrders)
	}
	if client.purchaseFailures["p1"].Remaining != 0 {
		t.Fatalf("expected fake to have exhausted the 429 budget, %d remaining", client.purchaseFailures["p1"].Remaining)
	}

	// Two retries on p1 → exactly two backoff sleeps honoring Retry-After.
	// Inter-call pacing sleeps (RateLimit=1ms) are also recorded; count only
	// the >=Retry-After ones.
	retrySleeps := 0
	for _, d := range sleeper.durations {
		if d >= 2*time.Second {
			retrySleeps++
		}
	}
	if retrySleeps != 2 {
		t.Fatalf("expected 2 retry sleeps of 2s, got %v", sleeper.durations)
	}
}

func TestSyncSurfacesResumableHintWhen429PersistsThroughBackoff(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	client.purchaseFailures = map[string]*purchaseFailureSpec{
		"p1": {Remaining: 1000, Status: 429},
	}
	sleeper := &recordingSleeper{}
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	_, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
		sleep:     sleeper.sleep,
		backoff:   &backoffPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 4 * time.Second},
	})
	if err == nil {
		t.Fatal("expected Sync to surface persistent rate-limit error")
	}
	if !strings.Contains(err.Error(), "rerun \"wolt stats\"") {
		t.Fatalf("expected resumable-rerun hint in error, got %q", err.Error())
	}
}

func TestSyncRefreshesAccessTokenOn401InMemory(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	client.rejectAccessToken = "expired-wt"
	client.nextAccessToken = "fresh-wt"
	client.nextRefreshToken = "rotated-rt"

	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")

	res, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
		Auth:      woltgateway.AuthContext{WToken: "expired-wt", RefreshToken: "old-rt"},
		Refresher: client.Refresher,
		sleep:     (&recordingSleeper{}).sleep,
		backoff:   &backoffPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 10 * time.Second},
	})
	if err != nil {
		t.Fatalf("Sync should recover from 401, got %v", err)
	}
	if res.DetailsFetched != 2 {
		t.Fatalf("expected 2 details fetched after refresh, got %d", res.DetailsFetched)
	}
	if client.refreshCalls != 1 {
		t.Fatalf("expected exactly 1 refresh call, got %d", client.refreshCalls)
	}
}

func TestSyncSurfaces401WhenRefresherFails(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	client.rejectAccessToken = "expired-wt"
	client.refreshErr = errors.New("refresh token revoked")

	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	_, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
		Auth:      woltgateway.AuthContext{WToken: "expired-wt", RefreshToken: "old-rt"},
		Refresher: client.Refresher,
		sleep:     (&recordingSleeper{}).sleep,
		backoff:   &backoffPolicy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: 4 * time.Second},
	})
	if err == nil {
		t.Fatal("expected Sync to surface the 401 + refresh failure")
	}
	if !strings.Contains(err.Error(), "refresh token revoked") {
		t.Fatalf("expected refresher error in surfaced message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected mention of original 401, got %q", err.Error())
	}
}

func TestSyncWithoutRefresherSurfaces401Directly(t *testing.T) {
	client := newFakeClient(twoOrderCorpus())
	client.rejectAccessToken = "expired-wt"

	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	_, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
		Auth:      woltgateway.AuthContext{WToken: "expired-wt", RefreshToken: "old-rt"},
		sleep:     (&recordingSleeper{}).sleep,
		backoff:   &backoffPolicy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: 4 * time.Second},
	})
	if err == nil {
		t.Fatal("expected 401 to surface when no Refresher is wired")
	}
	if client.refreshCalls != 0 {
		t.Fatalf("no refresher → no refresh calls; got %d", client.refreshCalls)
	}
}

func TestOpenStoreRepairsLegacyItemsAndPayments(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	db1, err := openStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	// Pre-fix payload shape — same as what real Wolt returns.
	rawJSON := `{
	    "total_price": 1609, "subtotal": 1609, "items_price": 2190,
	    "delivery_price": 76, "service_fee": 76,
	    "venue_id": "v1", "venue_country": "FIN",
	    "items": [
	      {"id": "i1", "name": "Whopper", "count": 1, "price": 990, "end_amount": 990},
	      {"id": "i2", "name": "Fries L", "count": 1, "price": 600, "end_amount": 600},
	      {"id": "i3", "name": "Cutlery", "count": 1, "price": 0, "end_amount": 0}
	    ],
	    "payments": [
	      {"name": "Edenred", "amount": 1609, "method": {"id": "edenred-uuid", "provider": "edenred", "type": "edenred"}}
	    ]
	}`
	_, _ = db1.Exec(`INSERT INTO users (id, email, created_at, updated_at) VALUES ('u1','u@example.com','2026-05-21T00:00:00Z','2026-05-21T00:00:00Z')`)
	if _, err := db1.Exec(`
		INSERT INTO orders (
		  user_id, purchase_id, status, payment_time_ts, currency,
		  total_amount_minor, subtotal_minor, items_amount_minor,
		  delivery_fee_minor, service_fee_minor, fees_minor,
		  raw_json, synced_at
		) VALUES ('u1','pX','delivered',1716200000,'EUR',1609,1609,2190,76,76,152, ?, '2026-05-21T00:00:00Z')`, rawJSON); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	// Seed pre-fix child rows: line_total=0 for every item, payments with
	// blank provider/type/id (mirrors what the buggy version wrote).
	for i, p := range []struct {
		name  string
		count int
		price int
	}{
		{"Whopper", 1, 990},
		{"Fries L", 1, 600},
		{"Cutlery", 1, 0},
	} {
		if _, err := db1.Exec(`
			INSERT INTO order_items (user_id, purchase_id, item_index, item_name,
			                         quantity, unit_price_minor, line_total_minor)
			VALUES ('u1','pX',?,?,?,?,0)`, i, p.name, p.count, p.price); err != nil {
			t.Fatalf("seed item %d: %v", i, err)
		}
	}
	if _, err := db1.Exec(`
		INSERT INTO order_payments (user_id, purchase_id, payment_index, amount_minor)
		VALUES ('u1','pX',0,1609)`); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	_ = db1.Close()

	// Re-open: openStore runs repairLegacyZeroAmounts again.
	db2, err := openStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer func() { _ = db2.Close() }()

	// Items: line_total_minor backfilled from end_amount.
	rows, err := db2.Query(`SELECT item_name, line_total_minor FROM order_items WHERE purchase_id='pX' ORDER BY item_index`)
	if err != nil {
		t.Fatalf("scan items: %v", err)
	}
	defer func() { _ = rows.Close() }()
	want := map[string]int{"Whopper": 990, "Fries L": 600, "Cutlery": 0}
	for rows.Next() {
		var name string
		var lt int
		if err := rows.Scan(&name, &lt); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		if lt != want[name] {
			t.Errorf("item %q: want line_total %d, got %d", name, want[name], lt)
		}
	}

	// Payments: provider/method_type/method_id all populated from method.*.
	var provider, methodType, methodID sql.NullString
	if err := db2.QueryRow(`SELECT provider, method_type, method_id FROM order_payments WHERE purchase_id='pX'`).Scan(&provider, &methodType, &methodID); err != nil {
		t.Fatalf("scan payment: %v", err)
	}
	if provider.String != "edenred" {
		t.Errorf("provider: want edenred, got %q", provider.String)
	}
	if methodType.String != "edenred" {
		t.Errorf("method_type: want edenred, got %q", methodType.String)
	}
	if methodID.String != "edenred-uuid" {
		t.Errorf("method_id: want edenred-uuid, got %q", methodID.String)
	}
}

func TestEnsureSchemaRunsRepairWithoutSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	// Bootstrap schema, then seed a single broken row.
	db1, err := openStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	_, _ = db1.Exec(`INSERT INTO users (id, email, created_at, updated_at) VALUES ('u1','u@example.com','2026-05-21','2026-05-21')`)
	if _, err := db1.Exec(`
		INSERT INTO orders (user_id, purchase_id, status, payment_time_ts, currency,
		                    total_amount_minor, subtotal_minor, items_amount_minor,
		                    delivery_fee_minor, service_fee_minor, fees_minor,
		                    raw_json, synced_at)
		VALUES ('u1','pY','delivered',1716200000,'EUR',0,0,0,0,0,0,
		        '{"total_price":2500}','2026-05-21')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = db1.Close()

	if err := EnsureSchema(context.Background(), dbPath); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("verify open: %v", err)
	}
	defer func() { _ = db2.Close() }()
	var total int
	if err := db2.QueryRow(`SELECT total_amount_minor FROM orders WHERE purchase_id='pY'`).Scan(&total); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if total != 2500 {
		t.Fatalf("EnsureSchema should run repair: want total=2500, got %d", total)
	}
}

func TestOpenStoreRepairsLegacyZeroAmounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	// Bootstrap the schema by opening once.
	db1, err := openStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	// Seed a row that matches what the buggy version of statssync would
	// have written: zero amounts but a valid raw_json carrying real Wolt
	// flat fields. user_id FK requires a matching users row.
	rawJSON := `{
	    "total_price": 1609, "subtotal": 1609, "items_price": 2190,
	    "delivery_price": 76, "service_fee": 76, "credits": 0, "tokens": 0,
	    "venue_id": "venue-bk-iso-omena",
	    "venue_country": "FIN",
	    "venue_full_address": "Piispansilta 10, 02230 Espoo, Finland",
	    "venue_product_line": "restaurant",
	    "delivery_location": {"city": "Espoo", "alias": "home"},
	    "payments": [{"method": {"provider": "edenred", "type": "edenred"}}]
	}`
	_, _ = db1.Exec(`INSERT INTO users (id, email, created_at, updated_at) VALUES ('u1','u@example.com','2026-05-21T00:00:00Z','2026-05-21T00:00:00Z')`)
	_, err = db1.Exec(`
		INSERT INTO orders (
		  user_id, purchase_id, status, payment_time_ts, currency,
		  total_amount_minor, subtotal_minor, items_amount_minor,
		  delivery_fee_minor, service_fee_minor, fees_minor,
		  raw_json, synced_at
		) VALUES ('u1', 'pX', 'delivered', 1716200000, 'EUR',
		          0, 0, 0, 0, 0, 0, ?, '2026-05-21T00:00:00Z')`, rawJSON)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	_ = db1.Close()

	// Re-open: openStore runs repairLegacyZeroAmounts during init.
	db2, err := openStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer func() { _ = db2.Close() }()

	var total, items, delivery, service, fees int
	var venueID, venueCountry, venueAddress, venueProductLine sql.NullString
	var deliveryCity, deliveryAlias, provider, methodType sql.NullString
	row := db2.QueryRow(`
		SELECT total_amount_minor, items_amount_minor, delivery_fee_minor,
		       service_fee_minor, fees_minor,
		       venue_id, venue_country, venue_address, venue_product_line,
		       delivery_city, delivery_alias,
		       payment_provider, payment_method_type
		FROM orders WHERE purchase_id = 'pX'`)
	if err := row.Scan(
		&total, &items, &delivery, &service, &fees,
		&venueID, &venueCountry, &venueAddress, &venueProductLine,
		&deliveryCity, &deliveryAlias,
		&provider, &methodType,
	); err != nil {
		t.Fatalf("scan repaired row: %v", err)
	}
	if total != 1609 || items != 2190 || delivery != 76 || service != 76 || fees != 152 {
		t.Fatalf("repair did not populate amounts — total=%d items=%d delivery=%d service=%d fees=%d", total, items, delivery, service, fees)
	}
	if venueID.String != "venue-bk-iso-omena" {
		t.Fatalf("repair did not populate venue_id — got %q", venueID.String)
	}
	if venueCountry.String != "FIN" {
		t.Fatalf("repair did not populate venue_country — got %q", venueCountry.String)
	}
	if venueAddress.String != "Piispansilta 10, 02230 Espoo, Finland" {
		t.Fatalf("repair did not populate venue_address — got %q", venueAddress.String)
	}
	if venueProductLine.String != "restaurant" {
		t.Fatalf("repair did not populate venue_product_line — got %q", venueProductLine.String)
	}
	if deliveryCity.String != "Espoo" || deliveryAlias.String != "home" {
		t.Fatalf("repair did not populate delivery — city=%q alias=%q", deliveryCity.String, deliveryAlias.String)
	}
	if provider.String != "edenred" || methodType.String != "edenred" {
		t.Fatalf("repair did not populate payment — provider=%q type=%q", provider.String, methodType.String)
	}

	// Idempotent: re-running the repair must not change anything.
	if err := repairLegacyZeroAmounts(context.Background(), db2); err != nil {
		t.Fatalf("second repair: %v", err)
	}
	var totalAgain int
	if err := db2.QueryRow(`SELECT total_amount_minor FROM orders WHERE purchase_id = 'pX'`).Scan(&totalAgain); err != nil {
		t.Fatalf("scan after second repair: %v", err)
	}
	if totalAgain != 1609 {
		t.Fatalf("second repair clobbered the value: got %d", totalAgain)
	}
}

func TestSyncPersistsRealWoltAmountShape(t *testing.T) {
	// Mirror the flat shape Wolt actually returns (total_price /
	// items_price / delivery_price / service_fee as top-level ints,
	// venue_country flat, payment.method.{provider,type} nested).
	pages := []fakePage{{
		Orders: []map[string]any{{
			"purchase_id":     "wolt-real-1",
			"payment_time_ts": 1716200000,
			"currency":        "EUR",
			"status":          "delivered",
			"received_at":     "2026-05-20T10:00:00Z",
			"items_summary":   "Edenred meal",
			"venue_name":      "Burger King Iso Omena",
		}},
	}}
	client := newFakeClient(pages)
	client.detailByID = map[string]map[string]any{
		"wolt-real-1": {
			"order_number":       "WLT-EDEN-1",
			"status":             "delivered",
			"creation_time":      "20/05/2026, 17:33",
			"delivery_time":      "20/05/2026, 18:10",
			"currency":           "EUR",
			"total_price":        1609,
			"subtotal":           1609,
			"items_price":        2190,
			"delivery_price":     76,
			"service_fee":        76,
			"credits":            0,
			"tokens":             0,
			"venue_id":           "venue-bk-iso-omena",
			"venue_country":      "FIN",
			"venue_address":      "Piispansilta 10",
			"venue_full_address": "Piispansilta 10, 02230 Espoo, Finland",
			"venue_product_line": "restaurant",
			"delivery_method":    "homedelivery",
			"delivery_location": map[string]any{
				"city":  "Espoo",
				"alias": "home",
			},
			"items": []any{
				map[string]any{
					"id":         "i1",
					"name":       "Whopper Meal",
					"count":      1.0,
					"price":      2190,
					"end_amount": 2190,
				},
			},
			"payments": []any{
				map[string]any{
					"name":   "Edenred",
					"amount": 1609,
					"method": map[string]any{
						"id":       "edenred-uuid",
						"provider": "edenred",
						"type":     "edenred",
					},
				},
			},
			"discounts": []any{
				map[string]any{"amount": 657, "title": "Discounts"},
			},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "wolt.sqlite")
	if _, err := Sync(context.Background(), client, Options{
		DBPath:    dbPath,
		UserEmail: "user@example.com",
		PageSize:  50,
		RateLimit: time.Millisecond,
		Now:       fixedClock(),
	}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open verify db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var (
		total, subtotal, items, delivery, service, fees, discount       int
		venueID, venueCountry, venueAddress, venueProductLine           sql.NullString
		deliveryCity, deliveryAlias, paymentProvider, paymentMethodType sql.NullString
	)
	row := db.QueryRow(`
		SELECT total_amount_minor, subtotal_minor, items_amount_minor,
		       delivery_fee_minor, service_fee_minor, fees_minor,
		       discount_amount_minor,
		       venue_id, venue_country, venue_address, venue_product_line,
		       delivery_city, delivery_alias,
		       payment_provider, payment_method_type
		FROM orders WHERE purchase_id = ?`, "wolt-real-1")
	if err := row.Scan(
		&total, &subtotal, &items, &delivery, &service, &fees, &discount,
		&venueID, &venueCountry, &venueAddress, &venueProductLine,
		&deliveryCity, &deliveryAlias,
		&paymentProvider, &paymentMethodType,
	); err != nil {
		t.Fatalf("scan orders row: %v", err)
	}
	if total != 1609 || subtotal != 1609 || items != 2190 || delivery != 76 || service != 76 || fees != 152 {
		t.Fatalf("amounts mismatch — total=%d subtotal=%d items=%d delivery=%d service=%d fees=%d", total, subtotal, items, delivery, service, fees)
	}
	if discount != 657 {
		t.Fatalf("discount want 657, got %d", discount)
	}
	if venueID.String != "venue-bk-iso-omena" {
		t.Fatalf("venue_id want venue-bk-iso-omena, got %q", venueID.String)
	}
	if venueCountry.String != "FIN" {
		t.Fatalf("venue_country want FIN, got %q", venueCountry.String)
	}
	if venueAddress.String != "Piispansilta 10, 02230 Espoo, Finland" {
		t.Fatalf("venue_address want full address, got %q", venueAddress.String)
	}
	if venueProductLine.String != "restaurant" {
		t.Fatalf("venue_product_line want restaurant, got %q", venueProductLine.String)
	}
	if deliveryCity.String != "Espoo" || deliveryAlias.String != "home" {
		t.Fatalf("delivery mismatch — city=%q alias=%q", deliveryCity.String, deliveryAlias.String)
	}
	if paymentProvider.String != "edenred" || paymentMethodType.String != "edenred" {
		t.Fatalf("payment mismatch — provider=%q method_type=%q", paymentProvider.String, paymentMethodType.String)
	}

	var lineTotal int
	if err := db.QueryRow(`SELECT line_total_minor FROM order_items WHERE purchase_id = ?`, "wolt-real-1").Scan(&lineTotal); err != nil {
		t.Fatalf("scan order_items: %v", err)
	}
	if lineTotal != 2190 {
		t.Fatalf("item line_total want 2190 (from end_amount), got %d", lineTotal)
	}
}

func TestExtractMinorHandlesShapes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
	}{
		{"raw int", 1234, 1234},
		{"raw float", 12.0, 12},
		{"map amount int", map[string]any{"amount": 999.0}, 999},
		{"nested map", map[string]any{"amount": map[string]any{"amount": 42.0}}, 42},
		{"unrelated map", map[string]any{"label": "x"}, 0},
		{"nil", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractMinor(c.in); got != c.want {
				t.Fatalf("extractMinor(%v): want %d, got %d", c.in, c.want, got)
			}
		})
	}
}

func TestSlugifyMatchesNode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user@example.com", "user-example-com"},
		{"  Mixed.Case+Tag@DOMAIN.io ", "mixed-case-tag-domain-io"},
		{"unicode-örjan@x.io", "unicode-rjan-x-io"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := slugify(c.in); got != c.want {
				t.Fatalf("slugify(%q): want %q, got %q", c.in, c.want, got)
			}
		})
	}
}

func TestParseLocalDateTimeRoundTrip(t *testing.T) {
	got := parseLocalDateTime("21/05/2026, 14:30")
	if !got.Valid {
		t.Fatal("expected valid result")
	}
	if got.Date != "2026-05-21" {
		t.Fatalf("Date: %q", got.Date)
	}
	if got.Month != "2026-05" {
		t.Fatalf("Month: %q", got.Month)
	}
	if got.Datetime != "2026-05-21T14:30:00" {
		t.Fatalf("Datetime: %q", got.Datetime)
	}
	if got.Hour != 14 {
		t.Fatalf("Hour: %d", got.Hour)
	}
	// 2026-05-21 is a Thursday → weekday 4
	if got.Weekday != 4 {
		t.Fatalf("Weekday: %d", got.Weekday)
	}
}

func TestParseLocalDateTimeRejectsGarbage(t *testing.T) {
	bad := parseLocalDateTime("yesterday")
	if bad.Valid {
		t.Fatal("expected invalid for garbage input")
	}
}

func TestCatalogStopDecisionDetectsKnownPurchase(t *testing.T) {
	known := map[string]struct{}{"p1": {}}
	res := catalogStopDecision([]any{
		map[string]any{"purchase_id": "p9", "payment_time_ts": 200},
		map[string]any{"purchase_id": "p1", "payment_time_ts": 100},
	}, known, 50)
	if res != "known_purchase" {
		t.Fatalf("expected known_purchase, got %q", res)
	}
}

func TestCatalogStopDecisionDetectsCheckpoint(t *testing.T) {
	known := map[string]struct{}{"p1": {}}
	res := catalogStopDecision([]any{
		map[string]any{"purchase_id": "p9", "payment_time_ts": 30},
	}, known, 50)
	if res != "checkpoint_reached" {
		t.Fatalf("expected checkpoint_reached, got %q", res)
	}
}

// ----- test fixtures -----

func twoOrderCorpus() []fakePage {
	return []fakePage{
		{
			Orders: []map[string]any{
				{
					"purchase_id":     "p1",
					"payment_time_ts": 1700000000,
					"currency":        "EUR",
					"status":          "delivered",
					"received_at":     "2026-05-20T10:00:00Z",
					"items_summary":   "Pizza Margherita, Coke",
					"venue_name":      "Pizzeria",
				},
				{
					"purchase_id":     "p2",
					"payment_time_ts": 1700100000,
					"currency":        "EUR",
					"status":          "delivered",
					"received_at":     "2026-05-21T14:30:00Z",
					"items_summary":   "Ramen",
					"venue_name":      "Ramen House",
				},
			},
		},
	}
}

type fakePage struct {
	Orders    []map[string]any
	NextToken string
}

type fakeClient struct {
	pages         []fakePage
	mu            sync.Mutex
	listCalls     int
	purchaseCalls int
	refreshCalls  int
	// purchaseFailures lets a test inject N transient failures for a given
	// purchase ID before the canned success payload is returned. Used to
	// exercise the 429-retry path without forking the existing corpus.
	purchaseFailures map[string]*purchaseFailureSpec
	// rejectAccessToken, when non-empty, makes OrderHistoryPurchase return
	// 401 whenever it sees this token. Exercises the 401-then-refresh path.
	rejectAccessToken string
	// refreshErr, when non-nil, is what the Refresher closure returns.
	// Otherwise the Refresher swaps the access token to nextAccessToken
	// (and refresh token to nextRefreshToken if set) and returns success.
	refreshErr       error
	nextAccessToken  string
	nextRefreshToken string
	// detailByID overrides the canned detail payload for a given purchase
	// ID. Used when a test needs a specific Wolt-shape payload.
	detailByID map[string]map[string]any
}

func (f *fakeClient) Refresher(_ context.Context, refreshToken string, _ woltgateway.AuthContext) (woltgateway.TokenRefreshResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCalls++
	if f.refreshErr != nil {
		return woltgateway.TokenRefreshResult{}, f.refreshErr
	}
	return woltgateway.TokenRefreshResult{
		AccessToken:  f.nextAccessToken,
		RefreshToken: f.nextRefreshToken,
	}, nil
}

type purchaseFailureSpec struct {
	Remaining  int
	Status     int
	RetryAfter time.Duration
}

func newFakeClient(pages []fakePage) *fakeClient {
	return &fakeClient{pages: pages}
}

func (f *fakeClient) resetCounts() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls = 0
	f.purchaseCalls = 0
}

func (f *fakeClient) OrderHistory(_ context.Context, _ woltgateway.AuthContext, opts woltgateway.OrderHistoryOptions) (map[string]any, error) {
	f.mu.Lock()
	f.listCalls++
	idx := 0
	if opts.PageToken != "" {
		for i, p := range f.pages {
			if p.NextToken == opts.PageToken {
				idx = i + 1
				break
			}
		}
	}
	if idx >= len(f.pages) {
		f.mu.Unlock()
		return map[string]any{"orders": []any{}}, nil
	}
	p := f.pages[idx]
	f.mu.Unlock()

	out := make([]any, 0, len(p.Orders))
	for _, o := range p.Orders {
		out = append(out, o)
	}
	payload := map[string]any{"orders": out}
	if p.NextToken != "" {
		payload["next_page_token"] = p.NextToken
	}
	return payload, nil
}

func (f *fakeClient) OrderHistoryPurchase(_ context.Context, purchaseID string, auth woltgateway.AuthContext) (map[string]any, error) {
	f.mu.Lock()
	f.purchaseCalls++
	if spec, ok := f.purchaseFailures[purchaseID]; ok && spec.Remaining > 0 {
		spec.Remaining--
		f.mu.Unlock()
		return nil, &woltgateway.UpstreamRequestError{
			Method:     "GET",
			URL:        "https://consumer-api.wolt.com/order-tracking-api/v1/order_history/purchase/" + purchaseID,
			StatusCode: spec.Status,
			RetryAfter: spec.RetryAfter,
		}
	}
	// Reject any access token the test marked as expired. Lets the
	// 401-refresh test path force exactly one 401 before refresh.
	if f.rejectAccessToken != "" && auth.WToken == f.rejectAccessToken {
		f.mu.Unlock()
		return nil, &woltgateway.UpstreamRequestError{
			Method:     "GET",
			URL:        "https://consumer-api.wolt.com/order-tracking-api/v1/order_history/purchase/" + purchaseID,
			StatusCode: 401,
		}
	}
	if override, ok := f.detailByID[purchaseID]; ok {
		f.mu.Unlock()
		return override, nil
	}
	f.mu.Unlock()
	switch purchaseID {
	case "p1":
		return map[string]any{
			"order_number":  "WLT-1",
			"status":        "delivered",
			"creation_time": "20/05/2026, 10:00",
			"delivery_time": "20/05/2026, 10:30",
			"currency":      "EUR",
			"totals": map[string]any{
				"total":       map[string]any{"amount": 1599.0},
				"subtotal":    map[string]any{"amount": 1399.0},
				"items":       map[string]any{"amount": 1399.0},
				"delivery":    map[string]any{"amount": 100.0},
				"service_fee": map[string]any{"amount": 100.0},
			},
			"venue": map[string]any{"id": "v1", "name": "Pizzeria", "country": "FIN"},
			"items": []any{
				map[string]any{"id": "i1", "name": "Pizza Margherita", "count": 1.0, "price": map[string]any{"amount": 999.0}, "line_total": map[string]any{"amount": 999.0}},
			},
			"payments": []any{
				map[string]any{"name": "Mastercard ••••1234", "amount": map[string]any{"amount": 1599.0}, "method_type": "card", "provider": "stripe"},
			},
		}, nil
	case "p2":
		return map[string]any{
			"order_number":  "WLT-2",
			"status":        "delivered",
			"creation_time": "21/05/2026, 14:30",
			"delivery_time": "21/05/2026, 15:10",
			"currency":      "EUR",
			"totals": map[string]any{
				"total":       map[string]any{"amount": 2499.0},
				"subtotal":    map[string]any{"amount": 2099.0},
				"items":       map[string]any{"amount": 2099.0},
				"delivery":    map[string]any{"amount": 200.0},
				"service_fee": map[string]any{"amount": 200.0},
			},
			"venue": map[string]any{"id": "v2", "name": "Ramen House", "country": "FIN"},
			"items": []any{
				map[string]any{"id": "i2", "name": "Tonkotsu", "count": 1.0, "price": map[string]any{"amount": 1499.0}, "line_total": map[string]any{"amount": 1499.0}},
				map[string]any{"id": "i3", "name": "Gyoza", "count": 1.0, "price": map[string]any{"amount": 600.0}, "line_total": map[string]any{"amount": 600.0}},
			},
			"payments": []any{
				map[string]any{"name": "Wolt+ credits", "amount": map[string]any{"amount": 2499.0}, "method_type": "credits", "provider": "internal"},
			},
		}, nil
	}
	return nil, nil
}

func fixedClock() func() time.Time {
	t := time.Date(2026, 5, 21, 21, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}
