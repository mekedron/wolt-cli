// Command mcp-venue-smoke drives the wolt-mcp server's read-only venue tools
// against the live Wolt backend. It validates the structured detail and hours
// contracts without assuming that a venue is currently open or has a
// particular weekly schedule.
//
// A non-zero exit means an MCP venue tool stopped returning usable data.
//
// It is strictly read-only: neither tool mutates anything.
//
// Usage:
//
//	mcp-venue-smoke <venue-slug-id-or-url>
//
// Slug and id forms are always exercised after resolution; the canonical URL
// form is exercised when Wolt surfaces one.
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

	"github.com/mekedron/wolt-cli/internal/domain"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-venue-smoke: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || strings.TrimSpace(os.Args[1]) == "" {
		return fmt.Errorf("usage: mcp-venue-smoke <venue-slug-id-or-url>")
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
	identity, err := venueIdentityFromDetail(detail)
	if err != nil {
		return err
	}

	for _, ref := range uniqueVenueReferences(venue, identity.Slug, identity.ID, identity.CanonicalURL) {
		hours, hoursErr := callTool(ctx, session, "wolt_venue_hours", map[string]any{"venue": ref})
		if hoursErr != nil {
			return fmt.Errorf("venue reference %q: %w", referenceKind(ref, identity), hoursErr)
		}
		if validationErr := validateHours(hours, identity.ID); validationErr != nil {
			return fmt.Errorf("venue reference %q: %w", referenceKind(ref, identity), validationErr)
		}
	}

	fmt.Println("ok")
	return nil
}

type venueIdentity struct {
	ID           string
	Slug         string
	CanonicalURL string
}

func venueIdentityFromDetail(payload map[string]any) (venueIdentity, error) {
	venue, ok := payload["venue"].(map[string]any)
	if !ok {
		return venueIdentity{}, fmt.Errorf("wolt_venue_detail returned no venue object")
	}
	identity := venueIdentity{
		ID:           strings.TrimSpace(asString(venue["venue_id"])),
		Slug:         strings.TrimSpace(asString(venue["slug"])),
		CanonicalURL: strings.TrimSpace(asString(venue["canonical_url"])),
	}
	if !domain.IsObjectID(identity.ID) {
		return venueIdentity{}, fmt.Errorf("wolt_venue_detail returned a non-canonical venue id")
	}
	if identity.Slug == "" {
		return venueIdentity{}, fmt.Errorf("wolt_venue_detail returned no venue slug")
	}
	return identity, nil
}

func validateHours(payload map[string]any, expectedVenueID string) error {
	hours, ok := payload["hours"].(map[string]any)
	if !ok {
		return fmt.Errorf("wolt_venue_hours returned no hours object")
	}
	venueID := strings.TrimSpace(asString(hours["venue_id"]))
	if !strings.EqualFold(venueID, expectedVenueID) {
		return fmt.Errorf("wolt_venue_hours returned a different venue id")
	}
	windows, ok := hours["opening_windows"].([]any)
	if !ok {
		return fmt.Errorf("wolt_venue_hours returned a non-array opening_windows field")
	}
	for index, raw := range windows {
		if _, ok := raw.(map[string]any); !ok {
			return fmt.Errorf("wolt_venue_hours opening_windows[%d] is not an object", index)
		}
	}
	return nil
}

func uniqueVenueReferences(values ...string) []string {
	refs := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, value)
	}
	return refs
}

func referenceKind(ref string, identity venueIdentity) string {
	switch {
	case strings.EqualFold(ref, identity.ID):
		return "id"
	case strings.EqualFold(ref, identity.Slug):
		return "slug"
	case strings.EqualFold(ref, identity.CanonicalURL):
		return "url"
	default:
		return "input"
	}
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

func asString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
func firstText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "(no text content)"
}
