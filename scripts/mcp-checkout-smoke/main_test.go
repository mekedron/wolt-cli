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

func TestValidateCheckoutResultAcceptsTypedCheckoutOutcomes(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{name: "delivery mode unavailable", data: deliveryUnavailableData()},
		{name: "unavailable items", data: blockedCheckoutData()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCheckoutResult(typedCheckoutResult(test.data), "standard"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateCheckoutResultRejectsMalformedTypedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		data   func() map[string]any
		mutate func(map[string]any)
	}{
		{
			name: "wrong code",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["error"].(map[string]any)["code"] = "UPSTREAM_ERROR"
			},
		},
		{
			name: "missing message",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				delete(data["error"].(map[string]any), "message")
			},
		},
		{
			name: "missing retryable",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				delete(data["error"].(map[string]any), "retryable")
			},
		},
		{
			name: "retryable error",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["error"].(map[string]any)["retryable"] = true
			},
		},
		{
			name: "unavailable mode reported as applied",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["applied_delivery_mode"] = "standard"
			},
		},
		{
			name: "unknown available mode",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["available_delivery_modes"] = []any{"express"}
			},
		},
		{
			name: "empty available modes",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["available_delivery_modes"] = []any{}
			},
		},
		{
			name: "empty unavailable items",
			data: blockedCheckoutData,
			mutate: func(data map[string]any) {
				data["unavailable_items"] = []any{}
			},
		},
		{
			name: "malformed unavailable item",
			data: blockedCheckoutData,
			mutate: func(data map[string]any) {
				data["unavailable_items"] = []any{
					map[string]any{"item_id": "item-1", "name": "", "reason": "unavailable"},
				}
			},
		},
		{
			name: "untyped status",
			data: deliveryUnavailableData,
			mutate: func(data map[string]any) {
				data["status"] = "error"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := test.data()
			test.mutate(data)
			if err := validateCheckoutResult(typedCheckoutResult(data), "standard"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func typedCheckoutResult(data map[string]any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: data["summary"].(string)}},
		StructuredContent: data,
		IsError:           true,
	}
}

func deliveryUnavailableData() map[string]any {
	return map[string]any{
		"summary":                  "standard delivery was not confirmed",
		"status":                   "delivery_mode_unavailable",
		"requested_delivery_mode":  "standard",
		"available_delivery_modes": []any{"priority"},
		"error": map[string]any{
			"code":      "DELIVERY_MODE_UNAVAILABLE",
			"message":   "The checkout response did not select standard delivery.",
			"retryable": false,
		},
	}
}

func blockedCheckoutData() map[string]any {
	return map[string]any{
		"summary":                  "checkout preview blocked",
		"status":                   "blocked",
		"requested_delivery_mode":  "standard",
		"available_delivery_modes": []any{"standard"},
		"unavailable_items": []any{
			map[string]any{
				"item_id": "item-1",
				"name":    "Unavailable item",
				"reason":  "unavailable",
			},
		},
		"error": map[string]any{
			"code":      "UNAVAILABLE_ITEMS",
			"message":   "Remove or replace the unavailable items.",
			"retryable": false,
		},
	}
}
