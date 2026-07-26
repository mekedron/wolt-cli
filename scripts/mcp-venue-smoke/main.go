// Command mcp-venue-smoke drives the wolt-mcp server's read-only venue tools
// against the live Wolt backend. It exists because the MCP surface can rot
// independently of the CLI: wolt_venue_detail and wolt_venue_hours are built on
// Wolt's rich restaurant document, which is retired upstream (HTTP 410), and
// both tools returned "venue not found" for every venue while the equivalent
// CLI commands kept working off their static-payload fallback. Nothing in the
// smoke covered them, so the breakage was invisible.
//
// A non-zero exit means an MCP venue tool stopped returning usable data.
//
// It is strictly read-only: neither tool mutates anything.
//
// Usage:
//
//	mcp-venue-smoke <venue-slug-or-id>
//
// Both identifier forms are exercised when the venue resolves to an id, because
// they take different resolution paths inside the server.
//
// Environment:
//
//	WOLT_MCP_BIN          path to the wolt-mcp binary (default ./bin/wolt-mcp)
//	WOLT_SMOKE_LAT/_LON   coordinates to resolve the venue against (both together)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-venue-smoke: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || strings.TrimSpace(os.Args[1]) == "" {
		return fmt.Errorf("usage: mcp-venue-smoke <venue-slug-or-id>")
	}
	venue := strings.TrimSpace(os.Args[1])

	binary := os.Getenv("WOLT_MCP_BIN")
	if binary == "" {
		binary = "./bin/wolt-mcp"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-venue-smoke", Version: "1"}, nil)
	transport := &mcp.CommandTransport{Command: exec.CommandContext(ctx, binary)}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", binary, err)
	}
	defer func() { _ = session.Close() }()

	detailArgs := map[string]any{"venue": venue}
	if lat, lon, ok, coordErr := coordsFromEnv(); coordErr != nil {
		return coordErr
	} else if ok {
		detailArgs["lat"] = lat
		detailArgs["lon"] = lon
	}

	detail, err := callTool(ctx, session, "wolt_venue_detail", detailArgs)
	if err != nil {
		return err
	}
	venueData := mapField(detail, "venue")
	if strings.TrimSpace(asString(venueData["name"])) == "" {
		return fmt.Errorf("wolt_venue_detail returned no venue name: %s", compact(detail))
	}

	hours, err := callTool(ctx, session, "wolt_venue_hours", map[string]any{"venue": venue})
	if err != nil {
		return err
	}
	windows, _ := mapField(hours, "hours")["opening_windows"].([]any)
	if len(windows) == 0 {
		return fmt.Errorf("wolt_venue_hours returned no opening windows: %s", compact(hours))
	}

	// The id form takes a different resolution path than the slug form, so it is
	// exercised too once the detail call has surfaced a real venue id.
	if venueID := strings.TrimSpace(asString(venueData["venue_id"])); venueID != "" && venueID != venue {
		byID, hoursErr := callTool(ctx, session, "wolt_venue_hours", map[string]any{"venue": venueID})
		if hoursErr != nil {
			return fmt.Errorf("venue id form: %w", hoursErr)
		}
		idWindows, _ := mapField(byID, "hours")["opening_windows"].([]any)
		if len(idWindows) == 0 {
			return fmt.Errorf("wolt_venue_hours by id %s returned no opening windows", venueID)
		}
	}

	fmt.Printf("%s / %d opening windows\n", asString(venueData["name"]), len(windows))
	return nil
}

func callTool(ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (map[string]any, error) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", name, err)
	}
	if result.IsError {
		return nil, fmt.Errorf("%s returned an error: %s", name, firstText(result))
	}
	data := structuredData(result)
	if len(data) == 0 {
		return nil, fmt.Errorf("%s returned no structured data: %s", name, firstText(result))
	}
	return data, nil
}

func coordsFromEnv() (lat float64, lon float64, ok bool, err error) {
	latStr := strings.TrimSpace(os.Getenv("WOLT_SMOKE_LAT"))
	lonStr := strings.TrimSpace(os.Getenv("WOLT_SMOKE_LON"))
	if latStr == "" && lonStr == "" {
		return 0, 0, false, nil
	}
	if latStr == "" || lonStr == "" {
		return 0, 0, false, fmt.Errorf("WOLT_SMOKE_LAT and WOLT_SMOKE_LON must be set together")
	}
	lat, err = strconv.ParseFloat(latStr, 64)
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid WOLT_SMOKE_LAT %q: %w", latStr, err)
	}
	lon, err = strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid WOLT_SMOKE_LON %q: %w", lonStr, err)
	}
	return lat, lon, true, nil
}

// structuredData normalises the tool's StructuredContent (which the SDK may hand
// back as a json.RawMessage or an already-decoded map) into a map.
func structuredData(result *mcp.CallToolResult) map[string]any {
	switch sc := result.StructuredContent.(type) {
	case nil:
		return nil
	case map[string]any:
		return sc
	case json.RawMessage:
		decoded := map[string]any{}
		if err := json.Unmarshal(sc, &decoded); err != nil {
			return nil
		}
		return decoded
	default:
		encoded, err := json.Marshal(sc)
		if err != nil {
			return nil
		}
		decoded := map[string]any{}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return nil
		}
		return decoded
	}
}

func mapField(payload map[string]any, key string) map[string]any {
	if nested, ok := payload[key].(map[string]any); ok {
		return nested
	}
	return map[string]any{}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func compact(payload map[string]any) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "(unencodable)"
	}
	if len(encoded) > 300 {
		return string(encoded[:300]) + "..."
	}
	return string(encoded)
}

func firstText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "(no text content)"
}
