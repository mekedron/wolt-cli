package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	profileservice "github.com/mekedron/wolt-cli/internal/service/profile"
)

func TestCartShowSupportsCompactAndCompatibilityContentModes(t *testing.T) {
	const marker = "basket-detail-required-by-content-only-clients"
	payload := map[string]any{
		"baskets": []any{
			map[string]any{
				"id":    "basket-1",
				"items": []any{map[string]any{"name": marker, "description": strings.Repeat("x", 4096)}},
			},
		},
	}
	_, client := connectInMemory(t, Deps{
		Wolt: &stubWolt{
			basketsPageFn: func(context.Context, domain.Location, woltgateway.AuthContext) (map[string]any, error) {
				return payload, nil
			},
		},
		Profiles: &stubProfiles{profile: domain.Profile{
			Name:   "default",
			WToken: "access-token",
		}},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	compact, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "wolt_cart_show",
		Arguments: map[string]any{
			"lat": 10.25,
			"lon": 20.5,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if compact.IsError {
		t.Fatalf("unexpected tool error: %s", textContent(compact))
	}
	if got := textContent(compact); got != "1 basket" {
		t.Fatalf("default Content = %q, want short summary", got)
	}
	if strings.Contains(textContent(compact), marker) {
		t.Fatal("default Content duplicated the structured basket payload")
	}
	compactStructured, err := json.Marshal(compact.StructuredContent)
	if err != nil {
		t.Fatalf("marshal default StructuredContent: %v", err)
	}
	if !strings.Contains(string(compactStructured), marker) {
		t.Fatal("default StructuredContent lost the full basket payload")
	}

	compatible, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Meta: mcp.Meta{duplicateContentMetaKey: true},
		Name: "wolt_cart_show",
		Arguments: map[string]any{
			"lat": 10.25,
			"lon": 20.5,
		},
	})
	if err != nil {
		t.Fatalf("CallTool with compatibility metadata: %v", err)
	}
	if compatible.IsError {
		t.Fatalf("unexpected compatibility-mode tool error: %s", textContent(compatible))
	}
	var contentOnly map[string]any
	if err := json.Unmarshal([]byte(textContent(compatible)), &contentOnly); err != nil {
		t.Fatalf("content-only client could not decode result: %v", err)
	}
	var structured map[string]any
	encoded, err := json.Marshal(compatible.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	if err := json.Unmarshal(encoded, &structured); err != nil {
		t.Fatalf("decode StructuredContent: %v", err)
	}
	if !reflect.DeepEqual(contentOnly, structured) {
		t.Fatalf("content fallback differs from StructuredContent:\ncontent=%#v\nstructured=%#v", contentOnly, structured)
	}
	if !strings.Contains(textContent(compatible), marker) {
		t.Fatal("content-only client lost the full basket payload")
	}
}

func TestStructuredResultUsesSummaryWhenSDKContentDiffers(t *testing.T) {
	const summary = "resolved venue"
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: `{"venue":{"name":"example"},"summary":"resolved venue"}`,
			}},
			StructuredContent: map[string]any{
				"summary": summary,
				"venue":   map[string]any{"name": "example"},
			},
		}, nil
	}

	result, err := toolResultMiddleware(next)(context.Background(), callToolMethod, nil)
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	toolResult, ok := result.(*mcp.CallToolResult)
	if !ok {
		t.Fatalf("result type = %T, want *mcp.CallToolResult", result)
	}
	if got := textContent(toolResult); got != summary {
		t.Fatalf("Content = %q, want summary %q", got, summary)
	}
}

func TestClassifiedToolErrorUsesMetadataWithoutViolatingOutputSchema(t *testing.T) {
	_, client := connectInMemory(t, Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{findErr: profileservice.ErrDefaultProfileNotFound},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	schemaAdvertised := false
	for _, tool := range tools.Tools {
		if tool.Name == "wolt_account_status" {
			schemaAdvertised = tool.OutputSchema != nil
			break
		}
	}
	if !schemaAdvertised {
		t.Fatal("wolt_account_status did not advertise its success output schema")
	}

	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "wolt_account_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
	if result.StructuredContent != nil {
		t.Fatalf("error StructuredContent = %#v, want nil so the success schema does not apply", result.StructuredContent)
	}
	var info ToolErrorInfo
	encoded, err := json.Marshal(result.Meta["wolt_error"])
	if err != nil {
		t.Fatalf("marshal _meta.wolt_error: %v", err)
	}
	if err := json.Unmarshal(encoded, &info); err != nil {
		t.Fatalf("decode _meta.wolt_error: %v", err)
	}
	if info.Code != "AUTH_REQUIRED" || info.Message == "" || info.Retryable || info.RetryAfterMS != 0 {
		t.Fatalf("unexpected ToolErrorInfo: %+v", info)
	}
	if got := textContent(result); got != info.Message {
		t.Fatalf("Content = %q, want short error message %q", got, info.Message)
	}
}

func TestRateLimitErrorCarriesRetryMetadata(t *testing.T) {
	err := toolErr(&woltgateway.UpstreamRequestError{
		StatusCode: 429,
		RetryAfter: 1500 * time.Millisecond,
		Body:       "must not leak",
	})
	result := &mcp.CallToolResult{IsError: true}
	result.SetError(err)
	normalizeToolErrorResult(result)

	info, ok := result.Meta["wolt_error"].(ToolErrorInfo)
	if !ok {
		t.Fatalf("_meta.wolt_error type = %T, want ToolErrorInfo", result.Meta["wolt_error"])
	}
	if info.Code != "RATE_LIMITED" || !info.Retryable || info.RetryAfterMS != 1500 {
		t.Fatalf("unexpected ToolErrorInfo: %+v", info)
	}
	if result.StructuredContent != nil {
		t.Fatalf("error StructuredContent = %#v, want nil", result.StructuredContent)
	}
	if strings.Contains(info.Message, "must not leak") {
		t.Fatalf("upstream body leaked: %q", info.Message)
	}
}

func TestOutdatedClientErrorIsDistinctFromUnsupportedEndpoint(t *testing.T) {
	err := toolErr(&woltgateway.UpstreamRequestError{
		StatusCode: 410,
		Body:       `{"message":"This version of the app is no longer supported. Please update your app."}`,
	})
	var classified *classifiedToolError
	if !errors.As(err, &classified) {
		t.Fatalf("error type = %T, want classifiedToolError", err)
	}
	if classified.info.Code != "CLIENT_OUTDATED" {
		t.Fatalf("code = %q, want CLIENT_OUTDATED", classified.info.Code)
	}
}
