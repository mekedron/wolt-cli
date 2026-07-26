// Command wolt-mcp serves wolt-cli's functionality over the Model Context
// Protocol so AI clients (Claude Desktop, Claude Code, Cursor, …) can drive
// Wolt searches, view orders, manage baskets, and preview checkouts.
//
// Wire it into Claude Desktop / Claude Code with:
//
//	{ "mcpServers": { "wolt": { "command": "wolt-mcp" } } }
//
// The server shares ~/.wolt/.wolt-config.json with the wolt CLI binary — log
// in once via `wolt login` and the MCP server inherits the same session.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/config"
	locationgateway "github.com/mekedron/wolt-cli/internal/gateway/location"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/mcpserver"
	"github.com/mekedron/wolt-cli/internal/service/profile"
)

var version = "dev"

const (
	defaultWoltHTTPMinInterval = 220 * time.Millisecond
	woltHTTPMinIntervalEnv     = "WOLT_HTTP_MIN_INTERVAL_MS"
	defaultLocale              = "en-FI"
	localeEnv                  = "WOLT_LOCALE"
	duplicateContentEnv        = "WOLT_MCP_DUPLICATE_CONTENT"
)

func main() {
	// CRITICAL: stdout is the MCP JSON-RPC transport. Anything that lands on
	// stdout outside of the SDK will corrupt the protocol. Force the stdlib
	// `log` package and the default slog handler to stderr before any other
	// init can fire a log line.
	log.SetOutput(os.Stderr)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	locale := resolveLocale()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println(version)
			return
		case "--help", "-h", "help":
			printHelp()
			return
		}
	}

	store, err := config.NewStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wolt-mcp:", err)
		os.Exit(1)
	}

	wolt := woltgateway.NewClient(
		woltgateway.WithRequestMinInterval(resolveWoltRequestMinInterval()),
		woltgateway.WithLocale(locale),
	)

	deps := mcpserver.Deps{
		Wolt:             wolt,
		Profiles:         profile.NewResolver(store),
		Location:         locationgateway.NewClient(),
		Config:           store,
		Version:          version,
		Locale:           locale,
		Logger:           logger,
		DuplicateContent: resolveDuplicateContent(),
	}

	srv := mcpserver.NewServer(deps)

	logger.Info("wolt-mcp starting", "version", version, "config", store.Path())
	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Error("wolt-mcp exited with error", "err", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`
wolt-mcp — Model Context Protocol server for wolt-cli.

Usage:
  wolt-mcp              Run the MCP server over stdio.
  wolt-mcp --version    Print version and exit.
  wolt-mcp --help       Print this message and exit.

Options:
  --locale <bcp47>      Response locale in BCP-47 format (default: en-FI).
                        Can also be set via the WOLT_LOCALE environment variable.

Environment:
  WOLT_MCP_DUPLICATE_CONTENT=1
                        Also serve the full typed payload as serialized JSON in
                        content, not just structuredContent. Set this only for
                        clients that read content alone — it roughly doubles
                        response size.

Wire into an MCP client (Claude Desktop, Claude Code, Cursor) with:

  { "mcpServers": { "wolt": { "command": "wolt-mcp" } } }

Authentication is shared with the wolt CLI — run 'wolt login' once to enable
the auth-gated tools (cart, favorites, account, checkout_preview).
`))
}

func resolveWoltRequestMinInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv(woltHTTPMinIntervalEnv))
	if raw == "" {
		return defaultWoltHTTPMinInterval
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return defaultWoltHTTPMinInterval
	}
	return time.Duration(ms) * time.Millisecond
}

// resolveDuplicateContent reads the opt-in that mirrors the full typed payload
// into Content. Only the documented truthy spellings enable it, so a stray or
// empty value keeps the compact default rather than silently doubling payloads.
func resolveDuplicateContent() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(duplicateContentEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func resolveLocale() string {
	const flag = "--locale"
	// Start at 1 to skip the program name in os.Args[0].
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == flag && i+1 < len(os.Args) {
			return strings.TrimSpace(os.Args[i+1])
		}
		if value, ok := strings.CutPrefix(arg, flag+"="); ok {
			return strings.TrimSpace(value)
		}
	}
	raw := strings.TrimSpace(os.Getenv(localeEnv))
	if raw != "" {
		return raw
	}
	return defaultLocale
}
