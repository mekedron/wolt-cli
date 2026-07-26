package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates CLI tests from the developer's browser, config, and
// venue-slug cache.
func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "wolt-cli-tests-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv(envDisableChromeSync, "1"); err != nil {
		panic(err)
	}
	if err := os.Setenv("WOLT_CONFIG_PATH", filepath.Join(tempDir, "config.json")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(tempDir)
	os.Exit(code)
}
