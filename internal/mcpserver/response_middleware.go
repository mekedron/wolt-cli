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

// toolResultMiddleware exposes classified tool errors in result metadata.
// Successful results default to a short Content summary while retaining the
// complete typed output in StructuredContent. Content-only clients can request
// the SDK's JSON compatibility duplicate through request metadata.
func toolResultMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
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
		if duplicateContentRequested(req) {
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
		if contentDuplicatesStructured(toolResult.Content, structuredJSON) {
			toolResult.Content = []mcp.Content{&mcp.TextContent{Text: compactText(summary)}}
		}
		return result, nil
	}
}

func duplicateContentRequested(req mcp.Request) bool {
	if req == nil || req.GetParams() == nil {
		return false
	}
	enabled, _ := req.GetParams().GetMeta()[duplicateContentMetaKey].(bool)
	return enabled
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

func contentDuplicatesStructured(content []mcp.Content, structuredJSON []byte) bool {
	if len(content) != 1 {
		return false
	}
	text, ok := content[0].(*mcp.TextContent)
	if !ok {
		return false
	}
	return strings.TrimSpace(text.Text) == strings.TrimSpace(string(structuredJSON))
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
