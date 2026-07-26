package main

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestValidateCheckoutResultAcceptsSummaryOnlyContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "checkout ready"}},
		StructuredContent: map[string]any{
			"summary":                  "checkout ready",
			"status":                   "ready",
			"requested_delivery_mode":  "standard",
			"applied_delivery_mode":    "standard",
			"available_delivery_modes": []any{"standard", "priority"},
			"data":                     map[string]any{"payable_amount": 500},
		},
	}
	if err := validateCheckoutResult(result, "standard"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCheckoutResultRejectsDuplicatedContent(t *testing.T) {
	structured := map[string]any{
		"summary":                  "checkout ready",
		"status":                   "ready",
		"requested_delivery_mode":  "standard",
		"applied_delivery_mode":    "standard",
		"available_delivery_modes": []any{"standard"},
		"data":                     map[string]any{"payable_amount": 500},
	}
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: `{"applied_delivery_mode":"standard","available_delivery_modes":["standard"],"data":{"payable_amount":500},"requested_delivery_mode":"standard","status":"ready","summary":"checkout ready"}`,
		}},
		StructuredContent: structured,
	}
	err := validateCheckoutResult(result, "standard")
	if err == nil || !strings.Contains(err.Error(), "content is not the structured summary") {
		t.Fatalf("error = %v", err)
	}
}
