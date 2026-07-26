package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func registerAccountTools(srv *mcp.Server, tc *ToolCtx) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_account_status",
		Title:       "Account status",
		Description: "Return the authenticated user profile (name, email, country). Requires 'wolt login' on the CLI first.",
		Annotations: readOnly,
	}, tc.handleAccountStatus)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_account_orders",
		Title:       "Order history",
		Description: "Return paginated order history. Use page_token returned from a prior call to fetch the next page.",
		Annotations: readOnly,
	}, tc.handleAccountOrders)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_account_order",
		Title:       "Order detail",
		Description: "Return one order's full details (items, totals, delivery info) by purchase id.",
		Annotations: readOnly,
	}, tc.handleAccountOrder)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_account_addresses",
		Title:       "Saved addresses",
		Description: "List the user's saved delivery addresses.",
		Annotations: readOnly,
	}, tc.handleAccountAddresses)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wolt_account_payments",
		Title:       "Saved payment methods",
		Description: "List the user's saved payment methods.",
		Annotations: readOnly,
	}, tc.handleAccountPayments)
}

// ---------------- wolt_account_status ----------------

type AccountStatusInput struct{}
type AccountStatusOutput struct {
	Summary string         `json:"summary"`
	User    map[string]any `json:"user"`
}

func (tc *ToolCtx) handleAccountStatus(ctx context.Context, _ *mcp.CallToolRequest, _ AccountStatusInput) (*mcp.CallToolResult, AccountStatusOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, AccountStatusOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.UserMe(ctx, a)
	})
	if err != nil {
		return nil, AccountStatusOutput{}, toolErr(err)
	}
	return nil, AccountStatusOutput{
		Summary: accountStatusSummary(payload),
		User:    payload,
	}, nil
}

func accountStatusSummary(payload map[string]any) string {
	if identity := accountIdentity(payload, 0); identity != "" {
		return "authenticated as " + identity
	}
	return "authenticated"
}

// accountIdentity checks the wrappers used by Wolt's account endpoints before
// falling back to identifiers on the current object. In particular, UserMe
// commonly returns {"user":{"name":...}} rather than putting name at the
// response root.
func accountIdentity(payload map[string]any, depth int) string {
	if payload == nil || depth > 3 {
		return ""
	}
	for _, key := range []string{"user", "profile", "account", "customer", "data"} {
		if nested := asMap(payload[key]); nested != nil {
			if identity := accountIdentity(nested, depth+1); identity != "" {
				return identity
			}
		}
	}
	for _, key := range []string{"name", "display_name", "full_name"} {
		if nested := asMap(payload[key]); nested != nil {
			if identity := accountIdentity(nested, depth+1); identity != "" {
				return identity
			}
		}
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			return value
		}
	}
	first := strings.TrimSpace(asString(coalesceAny(payload["first_name"], payload["firstName"])))
	last := strings.TrimSpace(asString(coalesceAny(payload["last_name"], payload["lastName"])))
	if full := strings.TrimSpace(strings.Join([]string{first, last}, " ")); full != "" {
		return full
	}
	for _, key := range []string{"email", "_id", "id"} {
		if value := strings.TrimSpace(asString(payload[key])); value != "" {
			return value
		}
	}
	return ""
}

// ---------------- wolt_account_orders ----------------

type AccountOrdersInput struct {
	Limit     int    `json:"limit,omitempty"      jsonschema:"max orders (default upstream)"`
	PageToken string `json:"page_token,omitempty" jsonschema:"pagination token from prior call"`
}
type AccountOrdersOutput struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

func (tc *ToolCtx) handleAccountOrders(ctx context.Context, _ *mcp.CallToolRequest, in AccountOrdersInput) (*mcp.CallToolResult, AccountOrdersOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, AccountOrdersOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.OrderHistory(ctx, a, woltgateway.OrderHistoryOptions{Limit: in.Limit, PageToken: in.PageToken})
	})
	if err != nil {
		return nil, AccountOrdersOutput{}, toolErr(err)
	}
	count := len(asSlice(coalesceAny(payload["results"], payload["orders"])))
	return nil, AccountOrdersOutput{
		Summary: humanCount(count, "order", "orders"),
		Data:    payload,
	}, nil
}

// ---------------- wolt_account_order ----------------

type AccountOrderInput struct {
	PurchaseID string `json:"purchase_id" jsonschema:"order/purchase id"`
}
type AccountOrderOutput struct {
	Summary string         `json:"summary"`
	Order   map[string]any `json:"order"`
}

func (tc *ToolCtx) handleAccountOrder(ctx context.Context, _ *mcp.CallToolRequest, in AccountOrderInput) (*mcp.CallToolResult, AccountOrderOutput, error) {
	if in.PurchaseID == "" {
		return nil, AccountOrderOutput{}, toolErrf("purchase_id is required")
	}
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, AccountOrderOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.OrderHistoryPurchase(ctx, in.PurchaseID, a)
	})
	if err != nil {
		return nil, AccountOrderOutput{}, toolErr(err)
	}
	return nil, AccountOrderOutput{
		Summary: "order " + in.PurchaseID,
		Order:   payload,
	}, nil
}

// ---------------- wolt_account_addresses ----------------

type AccountAddressesInput struct{}
type AccountAddressesOutput struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

func (tc *ToolCtx) handleAccountAddresses(ctx context.Context, _ *mcp.CallToolRequest, _ AccountAddressesInput) (*mcp.CallToolResult, AccountAddressesOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, AccountAddressesOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.DeliveryInfoList(ctx, a)
	})
	if err != nil {
		return nil, AccountAddressesOutput{}, toolErr(err)
	}
	count := len(asSlice(coalesceAny(payload["results"], payload["addresses"])))
	return nil, AccountAddressesOutput{
		Summary: humanCount(count, "saved address", "saved addresses"),
		Data:    payload,
	}, nil
}

// ---------------- wolt_account_payments ----------------

type AccountPaymentsInput struct{}
type AccountPaymentsOutput struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

func (tc *ToolCtx) handleAccountPayments(ctx context.Context, _ *mcp.CallToolRequest, _ AccountPaymentsInput) (*mcp.CallToolResult, AccountPaymentsOutput, error) {
	_, auth, err := tc.requireAuth(ctx)
	if err != nil {
		return nil, AccountPaymentsOutput{}, toolErr(err)
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(a woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.PaymentMethods(ctx, a)
	})
	if err != nil {
		return nil, AccountPaymentsOutput{}, toolErr(err)
	}
	return nil, AccountPaymentsOutput{
		Summary: "payment methods",
		Data:    payload,
	}, nil
}
