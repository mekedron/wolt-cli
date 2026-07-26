package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const callToolMethod = "tools/call"
const duplicateContentMetaKey = "wolt/duplicate_content"

// newToolResultMiddleware exposes classified tool errors in result metadata.
// Successful results carry a short Content summary while retaining the complete
// typed output in StructuredContent.
//
// duplicateDefault turns on the SDK's serialized-JSON compatibility duplicate
// in Content for every call. It exists for clients that read only Content and
// cannot be configured to send request metadata; per-request metadata
// (duplicateContentMetaKey) overrides it in either direction.
func newToolResultMiddleware(duplicateDefault bool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || method != callToolMethod {
				return result, err
			}
			toolResult, ok := result.(*mcp.CallToolResult)
			if !ok || toolResult == nil {
				return result, nil
			}
			// A non-nil SDK-side error identifies ordinary handlers that returned
			// error. Some handlers intentionally return IsError=true together with
			// a typed output (for example checkout blockers); preserve that richer
			// StructuredContent.
			if toolResult.IsError && toolResult.GetError() != nil {
				normalizeToolErrorResult(toolResult)
				return result, nil
			}
			if duplicateContentRequested(req, duplicateDefault) {
				return result, nil
			}

			structuredJSON, ok := marshalStructuredContent(toolResult.StructuredContent)
			if !ok {
				return result, nil
			}
			summary := summaryFromStructuredJSON(structuredJSON)
			if summary == "" {
				summary = "request completed"
			}
			toolResult.Content = []mcp.Content{&mcp.TextContent{Text: compactText(summary)}}
			return result, nil
		}
	}
}

// duplicateContentRequested resolves the effective duplication setting. An
// explicit request-metadata boolean wins so a single client can opt in or out
// without restarting the server; anything else falls back to the server default.
func duplicateContentRequested(req mcp.Request, duplicateDefault bool) bool {
	if req == nil || req.GetParams() == nil {
		return duplicateDefault
	}
	if enabled, ok := req.GetParams().GetMeta()[duplicateContentMetaKey].(bool); ok {
		return enabled
	}
	return duplicateDefault
}

func normalizeToolErrorResult(result *mcp.CallToolResult) {
	if result == nil {
		return
	}
	err := result.GetError()
	info := ToolErrorInfo{
		Code:      "TOOL_ERROR",
		Message:   errorContentMessage(result.Content),
		Retryable: false,
	}
	var classified *classifiedToolError
	if errors.As(err, &classified) {
		info = classified.info
	} else if err != nil {
		info.Message = compactText(err.Error())
	}
	if info.Message == "" {
		info.Message = "tool request failed"
	}
	result.Content = []mcp.Content{&mcp.TextContent{Text: info.Message}}
	result.StructuredContent = nil
	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}
	result.Meta["wolt_error"] = info
}

func marshalStructuredContent(value any) ([]byte, bool) {
	if value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case json.RawMessage:
		if len(typed) == 0 {
			return nil, false
		}
		return typed, true
	case []byte:
		if len(typed) == 0 {
			return nil, false
		}
		return typed, true
	default:
		encoded, err := json.Marshal(value)
		return encoded, err == nil
	}
}

func summaryFromStructuredJSON(encoded []byte) string {
	var envelope struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Summary)
}

func errorContentMessage(content []mcp.Content) string {
	for _, item := range content {
		if text, ok := item.(*mcp.TextContent); ok {
			if message := compactText(text.Text); message != "" {
				return message
			}
		}
	}
	return ""
}
