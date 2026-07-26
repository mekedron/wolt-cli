package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/service/statssync"
)

func TestResolveStatsDirPrecedence(t *testing.T) {
	t.Setenv(envStatsDir, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Run("default falls back to ~/.wolt/stats", func(t *testing.T) {
		dir, err := resolveStatsDir("")
		if err != nil {
			t.Fatalf("resolveStatsDir: %v", err)
		}
		want := filepath.Join(home, ".wolt", "stats")
		if dir != want {
			t.Fatalf("default: want %q, got %q", want, dir)
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Fatalf("expected dir to exist: %v", statErr)
		}
	})

	t.Run("env override beats default", func(t *testing.T) {
		custom := filepath.Join(t.TempDir(), "envdir")
		t.Setenv(envStatsDir, custom)
		dir, err := resolveStatsDir("")
		if err != nil {
			t.Fatalf("resolveStatsDir: %v", err)
		}
		if dir != custom {
			t.Fatalf("env: want %q, got %q", custom, dir)
		}
	})

	t.Run("flag override beats env", func(t *testing.T) {
		envDir := filepath.Join(t.TempDir(), "envdir")
		flagDir := filepath.Join(t.TempDir(), "flagdir")
		t.Setenv(envStatsDir, envDir)
		dir, err := resolveStatsDir(flagDir)
		if err != nil {
			t.Fatalf("resolveStatsDir: %v", err)
		}
		if dir != flagDir {
			t.Fatalf("flag: want %q, got %q", flagDir, dir)
		}
	})
}

func TestSyncSummaryShape(t *testing.T) {
	res := statssync.Result{
		Mode:              "incremental",
		PagesFetched:      3,
		OrdersScanned:     42,
		DetailsFetched:    7,
		InsertedOrders:    5,
		UpdatedOrders:     2,
		CatalogCount:      870,
		DetailCount:       870,
		StopReason:        "known_purchase",
		ReachedHistoryEnd: false,
		DurationMs:        8230,
	}
	summary := syncSummary(res)
	want := map[string]any{
		"mode":            "incremental",
		"pages_fetched":   3,
		"orders_scanned":  42,
		"details_fetched": 7,
		"inserted_orders": 5,
		"updated_orders":  2,
	}
	for key, expected := range want {
		if summary[key] != expected {
			t.Fatalf("syncSummary[%q]: want %v, got %v", key, expected, summary[key])
		}
	}
	if summary["stop_reason"] != "known_purchase" {
		t.Fatalf("expected stop_reason populated, got %v", summary["stop_reason"])
	}
}

func TestSyncSummaryOmitsEmptyStopReason(t *testing.T) {
	summary := syncSummary(statssync.Result{Mode: "full"})
	if _, ok := summary["stop_reason"]; ok {
		t.Fatal("expected stop_reason absent when empty")
	}
}

func TestBuildStatsTableContainsKeyFields(t *testing.T) {
	data := map[string]any{
		"stats_dir": "/tmp/x",
		"bundle": map[string]any{
			"version":     "v0.1.0",
			"source":      "github-release",
			"downloaded":  true,
			"active_path": "/tmp/x/bundles/v0.1.0",
		},
		"db_path": "/tmp/x/db/wolt-history.sqlite",
		"email":   "user@example.com",
		"sync": map[string]any{
			"mode":            "incremental",
			"orders_scanned":  10,
			"details_fetched": 2,
			"inserted_orders": 2,
			"updated_orders":  0,
			"stop_reason":     "known_purchase",
		},
		"server": map[string]any{
			"url":  "http://127.0.0.1:5173",
			"port": 5173,
			"pid":  12345,
		},
	}
	out := buildStatsTable(data)
	for _, needle := range []string{
		"v0.1.0",
		"github-release",
		"user@example.com",
		"http://127.0.0.1:5173",
		"incremental",
		"known_purchase",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected table to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestWriteStepFormatsBanner(t *testing.T) {
	var buf bytes.Buffer
	writeStep(&buf, 2, 3, "Syncing your Wolt order history")
	want := "\n[2/3] Syncing your Wolt order history\n"
	if buf.String() != want {
		t.Fatalf("writeStep output mismatch:\nwant %q\ngot  %q", want, buf.String())
	}
}

func TestWriteStepNilSinkIsNoOp(t *testing.T) {
	// Should be safe to call with a nil writer (JSON / YAML mode).
	writeStep(nil, 1, 3, "unused")
}
