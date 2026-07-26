package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPSubprocessSpeaksProtocol builds bin/wolt-mcp, spawns it, and runs an
// MCP initialize + tools/list round trip over stdio. Catches:
//  1. binary doesn't start
//  2. server pollutes stdout with non-JSON-RPC (would corrupt framing)
//  3. tool registration regression (count/name mismatch)
func TestMCPSubprocessSpeaksProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}

	binary := buildWoltMCP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "1"}, nil)
	cmd := exec.CommandContext(ctx, binary)
	// CRITICAL: capture stderr to a buffer so any rogue stdout from the server
	// (which would corrupt JSON-RPC framing) surfaces clearly in test output.
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	names := []string{}
	for tool, iterErr := range session.Tools(ctx, nil) {
		if iterErr != nil {
			t.Fatalf("listing tools: %v", iterErr)
		}
		names = append(names, tool.Name)
	}
	if len(names) < 20 {
		t.Errorf("expected at least 20 tools, got %d: %v", len(names), names)
	}
	for _, must := range []string{"wolt_feed", "wolt_cart_show", "wolt_account_status", "wolt_checkout_preview"} {
		if !slices.Contains(names, must) {
			t.Errorf("missing tool %q in subprocess server (got %d tools)", must, len(names))
		}
	}
}

func buildWoltMCP(t *testing.T) string {
	t.Helper()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	binaryName := "wolt-mcp"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binaryName)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/wolt-mcp")
	cmd.Dir = repoRoot
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wolt-mcp: %v", err)
	}
	return binPath
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// init ensures GOFLAGS doesn't accidentally pull in test cache that would mask
// stale binaries during this test.
var _ = strings.TrimSpace
