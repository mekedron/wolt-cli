// Command mcp-checkout-smoke drives the wolt-mcp server's wolt_checkout_preview
// tool over stdio against the live Wolt backend. It exists so the daily
// live-smoke can exercise the MCP code path — not just the CLI — for checkout
// preview. That path is the one issue/PR #23 fixed: the MCP handler used to POST
// a flat body that Wolt rejected with `('body','purchase_plan'): Field required`.
// A non-zero exit here means the MCP checkout preview broke again.
//
// It is read-only: wolt_checkout_preview only previews pricing and never places
// an order. The basket it prices must already exist on the account (live-smoke
// creates one via `wolt cart add` just before invoking this).
//
// Usage:
//
//	mcp-checkout-smoke <venue-slug-or-id>
//
// Environment:
//
//	WOLT_MCP_BIN             path to the wolt-mcp binary (default ./bin/wolt-mcp)
//	WOLT_SMOKE_LAT/_LON      coordinates to price against (both required together)
//	WOLT_SMOKE_DELIVERY_MODE standard | priority (default standard)
//
// The wolt-mcp server reads the same ~/.wolt/.wolt-config.json the CLI does, so
// authentication is whatever session that file holds.
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
		fmt.Fprintf(os.Stderr, "mcp-checkout-smoke: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || strings.TrimSpace(os.Args[1]) == "" {
		return fmt.Errorf("usage: mcp-checkout-smoke <venue-slug-or-id>")
	}
	venue := strings.TrimSpace(os.Args[1])

	binary := os.Getenv("WOLT_MCP_BIN")
	if binary == "" {
		binary = "./bin/wolt-mcp"
	}

	deliveryMode := strings.TrimSpace(os.Getenv("WOLT_SMOKE_DELIVERY_MODE"))
	if deliveryMode == "" {
		deliveryMode = "standard"
	}

	args := map[string]any{
		"venue":         venue,
		"delivery_mode": deliveryMode,
	}
	// Pin the coordinates when provided so the preview is deterministic rather
	// than depending on the account's saved address.
	if lat, lon, ok, err := coordsFromEnv(); err != nil {
		return err
	} else if ok {
		args["lat"] = lat
		args["lon"] = lon
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-checkout-smoke", Version: "1"}, nil)
	transport := &mcp.CommandTransport{Command: exec.CommandContext(ctx, binary)}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", binary, err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wolt_checkout_preview",
		Arguments: args,
	})
	if err != nil {
		return fmt.Errorf("call wolt_checkout_preview: %w", err)
	}
	if err := validateCheckoutResult(result, deliveryMode); err != nil {
		return err
	}

	data := structuredData(result)
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func validateCheckoutResult(result *mcp.CallToolResult, requestedMode string) error {
	if result == nil {
		return fmt.Errorf("wolt_checkout_preview returned no result")
	}

	data := structuredData(result)
	if len(data) == 0 {
		return fmt.Errorf("wolt_checkout_preview returned no structured data: %s", firstText(result))
	}
	summary := strings.TrimSpace(stringValue(data["summary"]))
	if summary == "" {
		return fmt.Errorf("structured result has no summary")
	}
	if text := firstText(result); text != summary {
		return fmt.Errorf("content is not the structured summary: content=%q summary=%q", text, summary)
	}
	compact, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode structured result: %w", err)
	}
	if strings.TrimSpace(firstText(result)) == strings.TrimSpace(string(compact)) {
		return fmt.Errorf("content duplicates the full structured result")
	}

	requestedMode = strings.ToLower(strings.TrimSpace(requestedMode))
	if requestedMode == "" {
		requestedMode = "standard"
	}
	if got := strings.TrimSpace(stringValue(data["requested_delivery_mode"])); got != requestedMode {
		return fmt.Errorf("requested_delivery_mode=%q, want %q", got, requestedMode)
	}
	if !validDeliveryModes(data["available_delivery_modes"]) {
		return fmt.Errorf("available_delivery_modes contract is invalid")
	}
	if result.IsError {
		return validateTypedCheckoutError(data)
	}
	if status := strings.TrimSpace(stringValue(data["status"])); status != "ready" {
		return fmt.Errorf("unexpected checkout status %q", status)
	}
	if got := strings.TrimSpace(stringValue(data["applied_delivery_mode"])); got != requestedMode {
		return fmt.Errorf("applied_delivery_mode=%q, want %q", got, requestedMode)
	}
	if !stringSliceContains(data["available_delivery_modes"], requestedMode) {
		return fmt.Errorf("available_delivery_modes does not contain %q", requestedMode)
	}
	if preview, ok := data["data"].(map[string]any); !ok || len(preview) == 0 {
		return fmt.Errorf("structured result has no checkout preview data")
	}
	return nil
}

func validateTypedCheckoutError(data map[string]any) error {
	status := strings.TrimSpace(stringValue(data["status"]))
	errorData, ok := data["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("checkout error has no structured error details")
	}
	code := strings.TrimSpace(stringValue(errorData["code"]))
	switch status {
	case "delivery_mode_unavailable":
		if code != "DELIVERY_MODE_UNAVAILABLE" {
			return fmt.Errorf("delivery-mode error code=%q", code)
		}
		if strings.TrimSpace(stringValue(data["applied_delivery_mode"])) != "" {
			return fmt.Errorf("unavailable delivery mode was reported as applied")
		}
	case "blocked":
		if code != "UNAVAILABLE_ITEMS" {
			return fmt.Errorf("blocked checkout error code=%q", code)
		}
		items, ok := data["unavailable_items"].([]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("blocked checkout has no unavailable_items")
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok ||
				strings.TrimSpace(stringValue(item["item_id"])) == "" ||
				strings.TrimSpace(stringValue(item["name"])) == "" ||
				strings.TrimSpace(stringValue(item["reason"])) == "" {
				return fmt.Errorf("blocked checkout has a malformed unavailable item")
			}
		}
	default:
		return fmt.Errorf("unexpected checkout error status %q", status)
	}
	if strings.TrimSpace(stringValue(errorData["message"])) == "" {
		return fmt.Errorf("checkout error message is empty")
	}
	retryable, ok := errorData["retryable"].(bool)
	if !ok || retryable {
		return fmt.Errorf("checkout error retryable contract is invalid")
	}
	return nil
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
		var out map[string]any
		if err := json.Unmarshal(sc, &out); err != nil {
			return nil
		}
		return out
	case []byte:
		var out map[string]any
		if err := json.Unmarshal(sc, &out); err != nil {
			return nil
		}
		return out
	default:
		// Re-marshal whatever concrete type the SDK used, then decode to a map.
		raw, err := json.Marshal(sc)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		return out
	}
}

func firstText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return strings.TrimSpace(tc.Text)
		}
	}
	return "(no text content)"
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func stringSliceContains(value any, want string) bool {
	switch values := value.(type) {
	case []any:
		for _, raw := range values {
			if strings.EqualFold(strings.TrimSpace(stringValue(raw)), want) {
				return true
			}
		}
	case []string:
		for _, raw := range values {
			if strings.EqualFold(strings.TrimSpace(raw), want) {
				return true
			}
		}
	}
	return false
}

func validDeliveryModes(value any) bool {
	var modes []string
	switch values := value.(type) {
	case []any:
		modes = make([]string, 0, len(values))
		for _, raw := range values {
			mode, ok := raw.(string)
			if !ok {
				return false
			}
			modes = append(modes, mode)
		}
	case []string:
		modes = values
	default:
		return false
	}
	if len(modes) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(modes))
	for _, raw := range modes {
		mode := strings.ToLower(strings.TrimSpace(raw))
		if mode != "standard" && mode != "priority" {
			return false
		}
		if _, exists := seen[mode]; exists {
			return false
		}
		seen[mode] = struct{}{}
	}
	return true
}
