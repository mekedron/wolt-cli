package cli

import (
	"fmt"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

const (
	profileOrdersDefaultLimit = 50
	profileOrdersMaxLimit     = 50
)

func newProfileOrdersCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var limit int
	var pageToken string
	var statusFilter string

	cmd := &cobra.Command{
		Use:     "orders",
		Aliases: []string{"history", "order-history"},
		Short:   "Browse account order history.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileOrdersList(cmd, deps, flags, limit, pageToken, statusFilter)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", profileOrdersDefaultLimit, "Number of orders to return per page (1-50).")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Pagination token for older orders.")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter orders by status (case-insensitive).")
	addGlobalFlags(cmd, &flags)
	cmd.AddCommand(newProfileOrdersListCommand(deps))
	cmd.AddCommand(newProfileOrdersShowCommand(deps))
	return cmd
}

func newProfileOrdersListCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var limit int
	var pageToken string
	var statusFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List account order history entries.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileOrdersList(cmd, deps, flags, limit, pageToken, statusFilter)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", profileOrdersDefaultLimit, "Number of orders to return per page (1-50).")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "Pagination token for older orders.")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter orders by status (case-insensitive).")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func runProfileOrdersList(
	cmd *cobra.Command,
	deps Dependencies,
	flags globalFlags,
	limit int,
	pageToken string,
	statusFilter string,
) error {
	format, err := parseOutputFormat(flags.Format)
	if err != nil {
		return err
	}
	profileName := defaultProfileName(flags.Profile)
	auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
	if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
		return err
	}

	if limit < 1 || limit > profileOrdersMaxLimit {
		return emitError(
			cmd,
			format,
			profileName,
			flags.Locale,
			flags.Output,
			"WOLT_INVALID_ARGUMENT",
			fmt.Sprintf("limit must be between 1 and %d", profileOrdersMaxLimit),
		)
	}

	payload, authWarnings, err := invokeWithAuthAutoRefresh(
		cmd.Context(),
		deps,
		flags,
		&auth,
		func(authCtx woltgateway.AuthContext) (map[string]any, error) {
			return deps.Wolt.OrderHistory(
				cmd.Context(),
				authCtx,
				woltgateway.OrderHistoryOptions{Limit: limit, PageToken: pageToken},
			)
		},
	)
	if err != nil {
		return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
	}

	orders := extractOrderHistoryOrders(payload, statusFilter)
	data := map[string]any{
		"orders": orders,
		"count":  len(orders),
	}
	if token := strings.TrimSpace(asString(payload["next_page_token"])); token != "" {
		data["next_page_token"] = token
	}
	if filter := strings.TrimSpace(statusFilter); filter != "" {
		data["status_filter"] = strings.ToLower(filter)
	}

	if format == output.FormatTable {
		return writeTable(cmd, buildProfileOrdersTable(data), flags.Output)
	}
	env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
	return writeMachinePayload(cmd, env, format, flags.Output)
}

func newProfileOrdersShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "show <purchase-id>",
		Short: "Show one order details from history.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}
			purchaseID := strings.TrimSpace(args[0])
			if purchaseID == "" {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_INVALID_ARGUMENT", "purchase id is required")
			}

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.OrderHistoryPurchase(cmd.Context(), purchaseID, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data := buildOrderHistoryDetail(payload)
			if format == output.FormatTable {
				return writeTable(cmd, buildProfileOrderDetailTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func extractOrderHistoryOrders(payload map[string]any, statusFilter string) []any {
	filter := strings.ToLower(strings.TrimSpace(statusFilter))
	rows := make([]any, 0)
	for _, value := range asSlice(payload["orders"]) {
		order := asMap(value)
		if order == nil {
			continue
		}
		status := strings.TrimSpace(asString(order["status"]))
		if filter != "" && !strings.EqualFold(status, filter) {
			continue
		}
		rows = append(rows, map[string]any{
			"purchase_id":         strings.TrimSpace(asString(coalesceAny(order["purchase_id"], order["order_id"], order["id"]))),
			"received_at":         strings.TrimSpace(asString(order["received_at"])),
			"status":              status,
			"venue_name":          strings.TrimSpace(asString(order["venue_name"])),
			"total_amount":        strings.TrimSpace(asString(coalesceAny(order["total_amount"], order["total"]))),
			"is_active":           asBool(order["is_active"]),
			"items_summary":       orderHistoryItemsSummary(order),
			"payment_time_ts":     asInt(order["payment_time_ts"]),
			"main_image":          strings.TrimSpace(asString(order["main_image"])),
			"main_image_blurhash": strings.TrimSpace(asString(order["main_image_blurhash"])),
		})
	}
	return rows
}

func orderHistoryItemsSummary(order map[string]any) string {
	if rawSummary, ok := order["items"].(string); ok {
		summary := strings.TrimSpace(rawSummary)
		if summary != "" {
			return summary
		}
	}
	itemNames := make([]string, 0)
	for _, value := range asSlice(order["items"]) {
		item := asMap(value)
		name := strings.TrimSpace(asString(item["name"]))
		if name == "" {
			continue
		}
		itemNames = append(itemNames, name)
	}
	return strings.Join(itemNames, ", ")
}

func buildProfileOrdersTable(data map[string]any) string {
	headers := []string{"Purchase ID", "Received", "Status", "Venue", "Total"}
	rows := make([][]string, 0)
	for _, value := range asSlice(data["orders"]) {
		order := asMap(value)
		rows = append(rows, []string{
			fallbackString(asString(order["purchase_id"]), "-"),
			fallbackString(asString(order["received_at"]), "-"),
			fallbackString(asString(order["status"]), "-"),
			fallbackString(asString(order["venue_name"]), "-"),
			fallbackString(asString(order["total_amount"]), "-"),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-"})
	}
	return output.RenderTable("Order history", headers, rows)
}

func buildOrderHistoryDetail(payload map[string]any) map[string]any {
	currency := strings.TrimSpace(asString(payload["currency"]))
	if currency == "" {
		currency = "EUR"
	}
	delivery := asMap(payload["delivery_location"])

	data := map[string]any{
		"order_id":        strings.TrimSpace(asString(coalesceAny(payload["order_id"], payload["purchase_id"], payload["id"]))),
		"order_number":    strings.TrimSpace(asString(payload["order_number"])),
		"status":          strings.TrimSpace(asString(payload["status"])),
		"creation_time":   strings.TrimSpace(asString(payload["creation_time"])),
		"delivery_time":   strings.TrimSpace(asString(payload["delivery_time"])),
		"delivery_method": strings.TrimSpace(asString(payload["delivery_method"])),
		"currency":        currency,
		"venue": map[string]any{
			"id":           strings.TrimSpace(asString(payload["venue_id"])),
			"name":         strings.TrimSpace(asString(payload["venue_name"])),
			"address":      strings.TrimSpace(asString(payload["venue_full_address"])),
			"phone":        strings.TrimSpace(asString(payload["venue_phone"])),
			"country":      strings.TrimSpace(asString(payload["venue_country"])),
			"product_line": strings.TrimSpace(asString(payload["venue_product_line"])),
		},
		"totals": map[string]any{
			"items":       orderHistoryAmount(asInt(payload["items_price"]), currency),
			"delivery":    orderHistoryAmount(asInt(payload["delivery_price"]), currency),
			"service_fee": orderHistoryAmount(asInt(payload["service_fee"]), currency),
			"subtotal":    orderHistoryAmount(asInt(payload["subtotal"]), currency),
			"credits":     orderHistoryAmount(asInt(payload["credits"]), currency),
			"tokens":      orderHistoryAmount(asInt(payload["tokens"]), currency),
			"total":       orderHistoryAmount(asInt(payload["total_price"]), currency),
		},
		"items":      extractOrderHistoryDetailItems(payload, currency),
		"payments":   extractOrderHistoryDetailPayments(payload, currency),
		"discounts":  extractOrderHistoryAdjustmentRows(asSlice(payload["discounts"]), currency),
		"surcharges": extractOrderHistoryAdjustmentRows(asSlice(payload["surcharges"]), currency),
		"delivery": map[string]any{
			"alias":   strings.TrimSpace(asString(delivery["alias"])),
			"address": strings.TrimSpace(asString(coalesceAny(delivery["street"], delivery["address"]))),
			"city":    strings.TrimSpace(asString(delivery["city"])),
			"comment": strings.TrimSpace(asString(payload["delivery_comment"])),
		},
	}

	return data
}

func extractOrderHistoryDetailItems(payload map[string]any, currency string) []any {
	rows := make([]any, 0)
	for _, value := range asSlice(payload["items"]) {
		item := asMap(value)
		if item == nil {
			continue
		}
		price := asInt(item["price"])
		endAmount := asInt(item["end_amount"])
		rows = append(rows, map[string]any{
			"id":         strings.TrimSpace(asString(item["id"])),
			"name":       strings.TrimSpace(asString(item["name"])),
			"count":      asInt(item["count"]),
			"price":      orderHistoryAmount(price, currency),
			"line_total": orderHistoryAmount(endAmount, currency),
			"options":    asSlice(item["options"]),
		})
	}
	return rows
}

func extractOrderHistoryDetailPayments(payload map[string]any, currency string) []any {
	rows := make([]any, 0)
	for _, value := range asSlice(payload["payments"]) {
		payment := asMap(value)
		if payment == nil {
			continue
		}
		method := asMap(payment["method"])
		rows = append(rows, map[string]any{
			"name":         strings.TrimSpace(asString(payment["name"])),
			"amount":       orderHistoryAmount(asInt(payment["amount"]), currency),
			"method_type":  strings.TrimSpace(asString(method["type"])),
			"method_id":    strings.TrimSpace(asString(method["id"])),
			"provider":     strings.TrimSpace(asString(method["provider"])),
			"payment_time": strings.TrimSpace(asString(payment["payment_time"])),
		})
	}
	return rows
}

func extractOrderHistoryAdjustmentRows(values []any, currency string) []any {
	rows := make([]any, 0)
	for _, value := range values {
		entry := asMap(value)
		if entry == nil {
			continue
		}
		rows = append(rows, map[string]any{
			"title":  strings.TrimSpace(asString(entry["title"])),
			"amount": orderHistoryAmount(asInt(entry["amount"]), currency),
		})
	}
	return rows
}

func orderHistoryAmount(amount int, currency string) map[string]any {
	return map[string]any{
		"amount":           amount,
		"formatted_amount": formatMinorAmount(amount, currency),
	}
}

func buildProfileOrderDetailTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	venue := asMap(data["venue"])
	totals := asMap(data["totals"])
	rows := [][]string{
		{"Order ID", fallbackString(asString(data["order_id"]), "-")},
		{"Order number", fallbackString(asString(data["order_number"]), "-")},
		{"Status", fallbackString(asString(data["status"]), "-")},
		{"Created", fallbackString(asString(data["creation_time"]), "-")},
		{"Delivered", fallbackString(asString(data["delivery_time"]), "-")},
		{"Delivery method", fallbackString(asString(data["delivery_method"]), "-")},
		{"Venue", fallbackString(asString(venue["name"]), "-")},
		{"Total", fallbackString(asString(asMap(totals["total"])["formatted_amount"]), "-")},
	}

	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		rows = append(rows, []string{
			"Item",
			fmt.Sprintf(
				"%s x%d (%s)",
				fallbackString(asString(item["name"]), "-"),
				asInt(item["count"]),
				fallbackString(asString(asMap(item["line_total"])["formatted_amount"]), "-"),
			),
		})
	}
	for _, value := range asSlice(data["payments"]) {
		payment := asMap(value)
		rows = append(rows, []string{
			"Payment",
			fmt.Sprintf(
				"%s (%s)",
				fallbackString(asString(payment["name"]), "-"),
				fallbackString(asString(asMap(payment["amount"])["formatted_amount"]), "-"),
			),
		})
	}

	return output.RenderTable("Order details", headers, rows)
}
