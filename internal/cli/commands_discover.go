package cli

import (
	"github.com/Valaraucoo/wolt-cli/internal/service/observability"
	"github.com/Valaraucoo/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newDiscoverCommand(deps Dependencies) *cobra.Command {
	discover := &cobra.Command{
		Use:   "discover",
		Short: "Read discovery feed and browse categories.",
	}
	discover.AddCommand(newDiscoverFeedCommand(deps))
	discover.AddCommand(newDiscoverCategoriesCommand(deps))
	return discover
}

func newDiscoverFeedCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool
	var limit int
	var limitSet bool

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Show discovery feed sections and venues.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			var latPtr *float64
			var lonPtr *float64
			if latSet {
				latPtr = &lat
			}
			if lonSet {
				lonPtr = &lon
			}

			location, profile, err := resolveLocation(
				cmd.Context(),
				deps,
				latPtr,
				lonPtr,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				cmd,
			)
			if err != nil {
				return err
			}

			page, err := deps.Wolt.FrontPage(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			sections, err := deps.Wolt.Sections(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}

			city := asString(asMap(page["city_data"])["name"])
			if city == "" {
				city = asString(page["city"])
			}
			var limitPtr *int
			if limitSet {
				limitPtr = &limit
			}
			data := observability.BuildDiscoveryFeed(sections, city, limitPtr)

			if format == output.FormatTable {
				return writeTable(cmd, buildDiscoveryFeedTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, []string{}, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for location lookup. Provide together with --lon.")
	cmd.Flags().Lookup("lat").NoOptDefVal = "0"
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for location lookup. Provide together with --lat.")
	cmd.Flags().Lookup("lon").NoOptDefVal = "0"
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit sections and items")
	addGlobalFlags(cmd, &flags)

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
		limitSet = cmd.Flags().Changed("limit")
		return nil
	}

	return cmd
}

func newDiscoverCategoriesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List available discovery categories.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}

			var latPtr *float64
			var lonPtr *float64
			if latSet {
				latPtr = &lat
			}
			if lonSet {
				lonPtr = &lon
			}
			location, profile, err := resolveLocation(
				cmd.Context(),
				deps,
				latPtr,
				lonPtr,
				flags.Profile,
				format,
				flags.Locale,
				flags.Output,
				cmd,
			)
			if err != nil {
				return err
			}

			sections, err := deps.Wolt.Sections(cmd.Context(), location)
			if err != nil {
				return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
			}
			data := observability.BuildCategoryList(sections)

			if format == output.FormatTable {
				return writeTable(cmd, buildCategoryTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profile, flags.Locale, data, []string{}, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for location lookup. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for location lookup. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func buildDiscoveryFeedTable(data map[string]any) string {
	headers := []string{"Section", "Venue", "Rating", "Delivery estimate", "Delivery fee"}
	rows := [][]string{}
	for _, sectionValue := range asSlice(data["sections"]) {
		section := asMap(sectionValue)
		sectionName := asString(section["title"])
		items := asSlice(section["items"])
		if len(items) == 0 {
			rows = append(rows, []string{sectionName, "-", "-", "-", "-"})
			continue
		}
		for idx, itemValue := range items {
			item := asMap(itemValue)
			rating := asString(item["rating"])
			if rating == "" {
				rating = "-"
			}
			fee := asString(asMap(item["delivery_fee"])["formatted_amount"])
			if fee == "" {
				fee = "-"
			}
			name := asString(item["name"])
			if idx > 0 {
				sectionName = ""
			}
			rows = append(rows, []string{sectionName, name, rating, asString(item["delivery_estimate"]), fee})
		}
	}
	title := "Discover feed: " + asString(data["city"])
	return output.RenderTable(title, headers, rows)
}

func buildCategoryTable(data map[string]any) string {
	headers := []string{"Category", "Slug", "ID"}
	rows := [][]string{}
	for _, value := range asSlice(data["categories"]) {
		category := asMap(value)
		rows = append(rows, []string{asString(category["name"]), asString(category["slug"]), asString(category["id"])})
	}
	return output.RenderTable("Discover categories", headers, rows)
}
