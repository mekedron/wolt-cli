package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Valaraucoo/wolt-cli/internal/service/observability"
	"github.com/Valaraucoo/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newVenueCommand(deps Dependencies) *cobra.Command {
	venue := &cobra.Command{
		Use:   "venue",
		Short: "Inspect venue details, menus, and opening hours.",
	}
	venue.AddCommand(newVenueShowCommand(deps))
	venue.AddCommand(newVenueMenuCommand(deps))
	venue.AddCommand(newVenueHoursCommand(deps))
	return venue
}

func newItemCommand(deps Dependencies) *cobra.Command {
	item := &cobra.Command{
		Use:   "item",
		Short: "Inspect a single menu item for a venue.",
	}
	item.AddCommand(newItemShowCommand(deps))
	item.AddCommand(newItemOptionsCommand(deps))
	return item
}

func newVenueShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var include string

	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Show venue details by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			item, err := deps.Wolt.ItemBySlug(cmd.Context(), profile.Location, slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", slug, profile.Name)
			}
			restaurant, err := deps.Wolt.RestaurantByID(cmd.Context(), item.Link.Target)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data, warnings, err := observability.BuildVenueDetail(item, restaurant, splitCSV(include))
			if err != nil {
				return err
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueDetailTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&include, "include", "", "Include sections: hours,tags,rating,fees")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newVenueMenuCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var category string
	var includeOptions bool
	var limit int
	var limitSet bool

	cmd := &cobra.Command{
		Use:   "menu <slug>",
		Short: "Show venue menu by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			item, err := deps.Wolt.ItemBySlug(cmd.Context(), profile.Location, slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", slug, profile.Name)
			}

			payloads := []map[string]any{}
			warnings := []string{}
			if payload, err := deps.Wolt.VenuePageStatic(cmd.Context(), slug); err == nil {
				payloads = append(payloads, payload)
			} else {
				warnings = append(warnings, "venue static page endpoint unavailable")
			}
			if payload, err := deps.Wolt.VenuePageDynamic(cmd.Context(), slug); err == nil {
				payloads = append(payloads, payload)
			} else {
				warnings = append(warnings, "venue dynamic page endpoint unavailable")
			}

			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			data, menuWarnings := observability.BuildVenueMenu(item.Link.Target, payloads, category, includeOptions, limitPtr)
			warnings = append(warnings, menuWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildVenueMenuTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "Category slug")
	cmd.Flags().BoolVar(&includeOptions, "include-options", false, "Include option group IDs")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit returned rows")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		limitSet = cmd.Flags().Changed("limit")
	}
	return cmd
}

func newVenueHoursCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var timezone string

	cmd := &cobra.Command{
		Use:   "hours <slug>",
		Short: "Show venue opening hours by slug.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			item, err := deps.Wolt.ItemBySlug(cmd.Context(), profile.Location, slug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", slug, profile.Name)
			}
			restaurant, err := deps.Wolt.RestaurantByID(cmd.Context(), item.Link.Target)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}

			data := observability.BuildVenueHours(restaurant, timezone)
			if format == output.FormatTable {
				return writeTable(cmd, buildVenueHoursTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, []string{}, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&timezone, "timezone", "", "Timezone override")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newItemShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var includeUpsell bool

	cmd := &cobra.Command{
		Use:   "show <venue-slug> <item-id>",
		Short: "Show item details by venue slug and item ID.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueSlug := args[0]
			itemID := args[1]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			item, err := deps.Wolt.ItemBySlug(cmd.Context(), profile.Location, venueSlug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", venueSlug, profile.Name)
			}

			payload := map[string]any{}
			warnings := []string{}
			if itemPayload, err := deps.Wolt.VenueItemPage(cmd.Context(), item.Link.Target, itemID); err == nil {
				payload = itemPayload
			} else {
				warnings = append(warnings, "item endpoint unavailable; falling back to venue payloads")
				if fallback, fallbackErr := deps.Wolt.VenuePageDynamic(cmd.Context(), venueSlug); fallbackErr == nil {
					payload = fallback
				} else {
					warnings = append(warnings, "venue payload fallback unavailable")
				}
			}

			data, itemWarnings := observability.BuildItemDetail(itemID, item.Link.Target, payload, includeUpsell)
			warnings = append(warnings, itemWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildItemDetailTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().BoolVar(&includeUpsell, "include-upsell", false, "Include upsell items")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newItemOptionsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "options <venue-slug> <item-id>",
		Short: "Show full option groups/values for an item.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			venueSlug := args[0]
			itemID := args[1]
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			profile, err := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if err != nil {
				return profileError(err, format, flags.Profile, flags.Locale, flags.Output, cmd)
			}
			item, err := deps.Wolt.ItemBySlug(cmd.Context(), profile.Location, venueSlug)
			if err != nil {
				return emitUpstreamError(cmd, format, profile.Name, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if item == nil {
				return fmt.Errorf("venue slug %q was not found in profile %q catalog", venueSlug, profile.Name)
			}

			payload := map[string]any{}
			warnings := []string{}
			if itemPayload, err := deps.Wolt.VenueItemPage(cmd.Context(), item.Link.Target, itemID); err == nil {
				payload = itemPayload
			} else {
				warnings = append(warnings, "item endpoint unavailable; falling back to venue payloads")
				if fallback, fallbackErr := deps.Wolt.VenuePageDynamic(cmd.Context(), venueSlug); fallbackErr == nil {
					payload = fallback
				} else {
					warnings = append(warnings, "venue payload fallback unavailable")
				}
			}

			data, optionWarnings := buildItemOptionsData(item.Link.Target, itemID, payload)
			warnings = append(warnings, optionWarnings...)

			if format == output.FormatTable {
				return writeTable(cmd, buildItemOptionsTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile.Name, flags.Locale, data, warnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildVenueDetailTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Venue ID", asString(data["venue_id"])},
		{"Slug", asString(data["slug"])},
		{"Address", asString(data["address"])},
		{"Currency", asString(data["currency"])},
		{"Rating", fallbackString(asString(data["rating"]), "-")},
		{"Delivery methods", stringsJoin(asSlice(data["delivery_methods"]), ", ")},
		{"Order minimum", fallbackString(asString(asMap(data["order_minimum"])["formatted_amount"]), "-")},
	}
	optional := []string{"tags", "opening_windows", "rating_details", "delivery_fee"}
	for _, field := range optional {
		if value, ok := data[field]; ok {
			rows = append(rows, []string{field, fmt.Sprintf("%v", value)})
		}
	}
	return output.RenderTable("Venue: "+asString(data["name"]), headers, rows)
}

func buildVenueMenuTable(data map[string]any) string {
	headers := []string{"Item ID", "Name", "Price", "Option groups"}
	rows := [][]string{}
	for _, value := range asSlice(data["items"]) {
		item := asMap(value)
		optionGroups := "-"
		if _, ok := item["option_group_ids"]; ok {
			optionGroups = stringsJoin(asSlice(item["option_group_ids"]), ", ")
			if optionGroups == "" {
				optionGroups = "-"
			}
		}
		rows = append(rows, []string{
			asString(item["item_id"]),
			asString(item["name"]),
			fallbackString(asString(asMap(item["base_price"])["formatted_amount"]), "-"),
			optionGroups,
		})
	}
	return output.RenderTable("Venue menu: "+asString(data["venue_id"]), headers, rows)
}

func buildVenueHoursTable(data map[string]any) string {
	headers := []string{"Day", "Open", "Close"}
	rows := [][]string{}
	for _, value := range asSlice(data["opening_windows"]) {
		window := asMap(value)
		rows = append(rows, []string{asString(window["day"]), asString(window["open"]), asString(window["close"])})
	}
	return output.RenderTable("Venue hours ("+asString(data["timezone"])+")", headers, rows)
}

func buildItemDetailTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Item ID", asString(data["item_id"])},
		{"Venue ID", asString(data["venue_id"])},
		{"Description", fallbackString(asString(data["description"]), "-")},
		{"Price", fallbackString(asString(asMap(data["price"])["formatted_amount"]), "-")},
		{"Option groups", fmt.Sprintf("%v", data["option_groups"])},
		{"Upsell items", fmt.Sprintf("%v", data["upsell_items"])},
	}
	return output.RenderTable("Item: "+asString(data["name"]), headers, rows)
}

func buildItemOptionsData(venueID string, itemID string, payload map[string]any) (map[string]any, []string) {
	warnings := []string{}
	optionSpecs := extractOptionSpecs(payload)
	currency := strings.TrimSpace(asString(asMap(payload["price"])["currency"]))
	if currency == "" {
		currency = "EUR"
	}
	groupIDs := make([]string, 0, len(optionSpecs))
	for groupID := range optionSpecs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	optionGroups := make([]any, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		spec := optionSpecs[groupID]
		valueIDs := make([]string, 0, len(spec.Values))
		for valueID := range spec.Values {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)

		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			value := spec.Values[valueID]
			values = append(values, map[string]any{
				"value_id": valueID,
				"name":     emptyToNil(value.Name),
				"price": map[string]any{
					"amount":   value.Price,
					"currency": currency,
				},
				"example_option": fmt.Sprintf("%s=%s", groupID, valueID),
			})
		}

		optionGroups = append(optionGroups, map[string]any{
			"group_id": groupID,
			"name":     emptyToNil(spec.Name),
			"required": spec.Required || spec.MinSelect > 0,
			"min":      spec.MinSelect,
			"max":      spec.MaxSelect,
			"values":   values,
		})
	}
	if len(optionGroups) == 0 {
		warnings = append(warnings, "no option groups were discovered in the item payload")
	}

	return map[string]any{
		"venue_id":      venueID,
		"item_id":       itemID,
		"option_groups": optionGroups,
		"group_count":   len(optionGroups),
		"currency":      currency,
	}, warnings
}

func buildItemOptionsTable(data map[string]any) string {
	summary := output.RenderTable("Item option groups", []string{"Field", "Value"}, [][]string{
		{"Venue ID", fallbackString(asString(data["venue_id"]), "-")},
		{"Item ID", fallbackString(asString(data["item_id"]), "-")},
		{"Groups", asString(data["group_count"])},
		{"Currency", fallbackString(asString(data["currency"]), "-")},
	})

	headers := []string{"Group", "Value ID", "Value name", "Price", "Example --option"}
	rows := [][]string{}
	for _, groupValue := range asSlice(data["option_groups"]) {
		group := asMap(groupValue)
		if group == nil {
			continue
		}
		groupLabel := fallbackString(asString(group["group_id"]), "-")
		if groupName := asString(group["name"]); groupName != "" {
			groupLabel = fmt.Sprintf("%s (%s)", groupLabel, groupName)
		}
		for _, valueNode := range asSlice(group["values"]) {
			valueMap := asMap(valueNode)
			if valueMap == nil {
				continue
			}
			currency := asString(asMap(valueMap["price"])["currency"])
			rows = append(rows, []string{
				groupLabel,
				fallbackString(asString(valueMap["value_id"]), "-"),
				fallbackString(asString(valueMap["name"]), "-"),
				fallbackString(formatMinorAmount(asInt(asMap(valueMap["price"])["amount"]), currency), "-"),
				"--option " + asString(valueMap["example_option"]),
			})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-"})
	}
	return summary + "\n\n" + output.RenderTable("Selectable values", headers, rows)
}

func fallbackString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func stringsJoin(values []any, separator string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, asString(value))
	}
	return join(parts, separator)
}

func join(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}
