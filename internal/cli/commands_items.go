package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/mekedron/wolt-cli/internal/service/searchload"
	"github.com/spf13/cobra"
)

const globalItemTableRowsPerVenue = 3

func newItemsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var query string
	var limit int
	var availableOnly bool

	cmd := &cobra.Command{
		Use:   "items",
		Short: "Search items across nearby venues.",
		Long: "Search items across nearby Wolt venues while preserving Wolt's global relevance order.\n\n" +
			"Wolt does not expose a continuation token or an exact total for this endpoint, so result completeness is reported as unknown.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			query = strings.TrimSpace(query)
			if query == "" {
				return fmt.Errorf("--query must not be empty")
			}
			if limit < 1 || limit > domain.GlobalItemSearchMaxLimit {
				return fmt.Errorf("--limit must be between 1 and %d", domain.GlobalItemSearchMaxLimit)
			}
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			location, profile, err := resolveProfileLocation(
				cmd.Context(),
				deps,
				flags.Address,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				&auth,
				cmd,
			)
			if err != nil {
				return err
			}
			searchAPI, ok := deps.Wolt.(searchload.API)
			if !ok {
				return fmt.Errorf("global item search is unavailable in the configured Wolt gateway")
			}
			payload, err := searchload.RequestItems(cmd.Context(), searchAPI, location, query, limit, auth)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			data, warnings := observability.BuildGlobalItemSearchResult(query, limit, payload, availableOnly)
			if format == output.FormatTable {
				return writeTable(cmd, buildGlobalItemSearchTable(data), flags.Output)
			}
			envelope := output.BuildEnvelope(profile, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, envelope, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Item search query.")
	cmd.Flags().IntVar(&limit, "limit", domain.GlobalItemSearchDefaultLimit, "Maximum globally ranked matches to request (1-200).")
	cmd.Flags().BoolVar(&availableOnly, "available-only", false, "Exclude items that Wolt explicitly marks unavailable.")
	if err := cmd.MarkFlagRequired("query"); err != nil {
		panic(err)
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildGlobalItemSearchTable(data map[string]any) string {
	itemsByRank := map[int]map[string]any{}
	for _, rawItem := range asSlice(data["items"]) {
		item := asMap(rawItem)
		itemsByRank[asInt(item["global_rank"])] = item
	}
	rows := [][]string{}
	for _, rawGroup := range asSlice(data["venue_groups"]) {
		group := asMap(rawGroup)
		venueName := fallbackString(asString(group["venue_name"]), asString(group["venue_id"]))
		for index, rawRank := range asSlice(group["item_ranks"]) {
			if index >= globalItemTableRowsPerVenue {
				break
			}
			rank := asInt(rawRank)
			item := itemsByRank[rank]
			if item == nil {
				continue
			}
			availability := "unknown"
			if available, known := item["is_available"].(bool); known {
				availability = boolToYesNo(available)
			}
			price := asString(asMap(item["base_price"])["formatted_amount"])
			if price == "" {
				price = "-"
			}
			shownVenue := venueName
			if index > 0 {
				shownVenue = ""
			}
			rows = append(rows, []string{
				strconv.Itoa(rank),
				shownVenue,
				truncateForTable(asString(item["name"]), 42),
				price,
				availability,
			})
		}
	}
	title := fmt.Sprintf(
		"Global item search: %s (%d matches; up to %d per venue shown)",
		asString(data["query"]),
		asInt(data["returned_count"]),
		globalItemTableRowsPerVenue,
	)
	return output.RenderTable(title, []string{"Rank", "Venue", "Item", "Price", "Available"}, rows)
}
