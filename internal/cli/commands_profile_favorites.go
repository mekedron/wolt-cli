package cli

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

var woltVenueIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{24}$`)

func newProfileFavoritesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:     "favorites",
		Aliases: []string{"favourites"},
		Short:   "List and manage favourite venues.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileFavoritesList(cmd, deps, flags, lat, lon, latSet, lonSet)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for favorites listing. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for favorites listing. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}

	cmd.AddCommand(newProfileFavoritesListCommand(deps))
	cmd.AddCommand(newProfileFavoritesAddCommand(deps))
	cmd.AddCommand(newProfileFavoritesRemoveCommand(deps))
	return cmd
}

func newProfileFavoritesListCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var lat float64
	var lon float64
	var latSet bool
	var lonSet bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show favourite venues for the authenticated account.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileFavoritesList(cmd, deps, flags, lat, lon, latSet, lonSet)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude override for favorites listing. Provide together with --lon.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude override for favorites listing. Provide together with --lat.")
	addGlobalFlags(cmd, &flags)
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		latSet = cmd.Flags().Changed("lat")
		lonSet = cmd.Flags().Changed("lon")
	}
	return cmd
}

func runProfileFavoritesList(
	cmd *cobra.Command,
	deps Dependencies,
	flags globalFlags,
	lat float64,
	lon float64,
	latSet bool,
	lonSet bool,
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

	payload, authWarnings, err := invokeWithAuthAutoRefresh(
		cmd.Context(),
		deps,
		flags,
		&auth,
		func(authCtx woltgateway.AuthContext) (map[string]any, error) {
			return deps.Wolt.FavoriteVenues(cmd.Context(), location, authCtx)
		},
	)
	if err != nil {
		return emitUpstreamError(cmd, format, profile, flags.Locale, flags.Output, flags.Verbose, err)
	}

	data := map[string]any{
		"favorites": extractFavoriteVenues(payload),
	}
	data["count"] = len(asSlice(data["favorites"]))

	if format == output.FormatTable {
		return writeTable(cmd, buildProfileFavoritesTable(data), flags.Output)
	}
	env := output.BuildEnvelope(profile, flags.Locale, data, authWarnings, nil)
	return writeMachinePayload(cmd, env, format, flags.Output)
}

func newProfileFavoritesAddCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "add <venue-id-or-slug>",
		Short: "Mark a venue as favourite.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFavoriteVenueMutation(cmd, deps, flags, args[0], "add")
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileFavoritesRemoveCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "remove <venue-id-or-slug>",
		Short: "Remove a venue from favourites.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFavoriteVenueMutation(cmd, deps, flags, args[0], "remove")
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func runFavoriteVenueMutation(
	cmd *cobra.Command,
	deps Dependencies,
	flags globalFlags,
	venueInput string,
	action string,
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

	resolution, err := resolveFavoriteVenueReference(cmd.Context(), deps, flags.Profile, flags.Address, &auth, venueInput)
	if err != nil {
		return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_INVALID_ARGUMENT", err.Error())
	}

	_, authWarnings, err := invokeWithAuthAutoRefresh(
		cmd.Context(),
		deps,
		flags,
		&auth,
		func(authCtx woltgateway.AuthContext) (map[string]any, error) {
			if action == "remove" {
				return deps.Wolt.FavoriteVenueRemove(cmd.Context(), resolution.VenueID, authCtx)
			}
			return deps.Wolt.FavoriteVenueAdd(cmd.Context(), resolution.VenueID, authCtx)
		},
	)
	if err != nil {
		return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
	}

	data := map[string]any{
		"action":      action,
		"venue_id":    resolution.VenueID,
		"slug":        resolution.Slug,
		"name":        resolution.Name,
		"is_favorite": action == "add",
	}

	if format == output.FormatTable {
		verb := "added"
		if action == "remove" {
			verb = "removed"
		}
		return writeTable(
			cmd,
			output.RenderTable(
				"Favourite venue "+verb,
				[]string{"Field", "Value"},
				[][]string{
					{"Venue ID", fallbackString(asString(data["venue_id"]), "-")},
					{"Slug", fallbackString(asString(data["slug"]), "-")},
					{"Name", fallbackString(asString(data["name"]), "-")},
					{"Is favourite", boolToYesNo(asBool(data["is_favorite"]))},
				},
			),
			flags.Output,
		)
	}
	env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
	return writeMachinePayload(cmd, env, format, flags.Output)
}

type favoriteVenueReference struct {
	VenueID string
	Slug    string
	Name    string
}

func resolveFavoriteVenueReference(
	ctx context.Context,
	deps Dependencies,
	selectedProfile string,
	addressOverride string,
	auth *woltgateway.AuthContext,
	rawInput string,
) (favoriteVenueReference, error) {
	input := strings.TrimSpace(rawInput)
	if input == "" {
		return favoriteVenueReference{}, fmt.Errorf("venue id or slug is required")
	}
	candidate := venueSlugFromInput(input)
	if candidate == "" {
		return favoriteVenueReference{}, fmt.Errorf("venue id or slug is required")
	}
	if woltVenueIDPattern.MatchString(candidate) {
		return favoriteVenueReference{
			VenueID: strings.ToLower(candidate),
		}, nil
	}

	if payload, err := deps.Wolt.VenuePageStatic(ctx, candidate); err == nil {
		reference := favoriteVenueReference{
			VenueID: strings.TrimSpace(asString(coalesceAny(
				asMap(payload["venue"])["id"],
				asMap(payload["venue_raw"])["id"],
				payload["venue_id"],
				payload["id"],
			))),
			Slug: strings.TrimSpace(asString(coalesceAny(
				asMap(payload["venue"])["slug"],
				asMap(payload["venue_raw"])["slug"],
				payload["slug"],
				candidate,
			))),
			Name: strings.TrimSpace(asString(coalesceAny(
				asMap(payload["venue"])["name"],
				asMap(payload["venue_raw"])["name"],
				payload["name"],
			))),
		}
		if reference.VenueID != "" {
			return reference, nil
		}
	}

	location, err := resolveFavoriteVenueLookupLocation(ctx, deps, selectedProfile, addressOverride, auth)
	if err != nil {
		return favoriteVenueReference{}, fmt.Errorf("unable to resolve venue slug %q to venue id", candidate)
	}
	item, itemErr := deps.Wolt.ItemBySlug(ctx, location, candidate)
	if itemErr != nil {
		return favoriteVenueReference{}, fmt.Errorf("unable to resolve venue slug %q to venue id", candidate)
	}
	if item == nil {
		return favoriteVenueReference{}, fmt.Errorf("unable to resolve venue slug %q to venue id", candidate)
	}

	reference := favoriteVenueReference{
		VenueID: strings.TrimSpace(asString(item.Link.Target)),
		Slug:    candidate,
		Name:    strings.TrimSpace(item.Title),
	}
	if item.Venue != nil {
		if strings.TrimSpace(asString(item.Venue.ID)) != "" {
			reference.VenueID = strings.TrimSpace(asString(item.Venue.ID))
		}
		if strings.TrimSpace(item.Venue.Slug) != "" {
			reference.Slug = strings.TrimSpace(item.Venue.Slug)
		}
		if strings.TrimSpace(item.Venue.Name) != "" {
			reference.Name = strings.TrimSpace(item.Venue.Name)
		}
	}
	if reference.VenueID == "" {
		return favoriteVenueReference{}, fmt.Errorf("unable to resolve venue slug %q to venue id", candidate)
	}
	return reference, nil
}

func resolveFavoriteVenueLookupLocation(
	ctx context.Context,
	deps Dependencies,
	selectedProfile string,
	addressOverride string,
	auth *woltgateway.AuthContext,
) (domain.Location, error) {
	if trimmed := strings.TrimSpace(addressOverride); trimmed != "" {
		if deps.Location == nil {
			return domain.Location{}, fmt.Errorf("location resolver is not available")
		}
		return deps.Location.Get(ctx, trimmed)
	}
	profile, err := deps.Profiles.Find(ctx, selectedProfile)
	if err != nil {
		return domain.Location{}, err
	}
	location, locationErr := resolveAccountLocation(ctx, deps, profile, auth)
	if locationErr == nil {
		return location, nil
	}
	return domain.Location{}, locationErr
}

func extractFavoriteVenues(payload map[string]any) []any {
	candidates := make([]map[string]any, 0)
	collectFavoriteVenueRows(asSlice(payload["items"]), &candidates)
	for _, sectionValue := range asSlice(payload["sections"]) {
		section := asMap(sectionValue)
		collectFavoriteVenueRows(asSlice(section["items"]), &candidates)
	}
	if len(candidates) == 0 {
		collectFavoriteVenueRows(asSlice(payload["results"]), &candidates)
	}

	seen := map[string]struct{}{}
	rows := make([]map[string]any, 0, len(candidates))
	for _, row := range candidates {
		venueID := strings.TrimSpace(asString(row["venue_id"]))
		slug := strings.ToLower(strings.TrimSpace(asString(row["slug"])))
		key := strings.ToLower(venueID)
		if key == "" {
			key = "slug:" + slug
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(asString(rows[i]["name"])))
		rightName := strings.ToLower(strings.TrimSpace(asString(rows[j]["name"])))
		if leftName == rightName {
			return strings.ToLower(strings.TrimSpace(asString(rows[i]["slug"]))) < strings.ToLower(strings.TrimSpace(asString(rows[j]["slug"])))
		}
		return leftName < rightName
	})

	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func collectFavoriteVenueRows(items []any, acc *[]map[string]any) {
	for _, value := range items {
		item := asMap(value)
		venue := asMap(coalesceAny(item["venue"], item["restaurant"]))
		if venue == nil {
			continue
		}

		link := asMap(item["link"])
		targetURL := strings.TrimSpace(asString(link["target"]))
		venueID := strings.TrimSpace(asString(venue["id"]))
		slug := strings.TrimSpace(asString(venue["slug"]))
		if slug == "" {
			slug = venueSlugFromInput(targetURL)
		}
		if venueID == "" && slug == "" {
			continue
		}

		row := map[string]any{
			"venue_id":           venueID,
			"slug":               slug,
			"name":               strings.TrimSpace(asString(coalesceAny(venue["name"], item["title"]))),
			"address":            strings.TrimSpace(asString(venue["address"])),
			"rating":             formatFavoriteRating(coalesceAny(asMap(venue["rating"])["score"], asMap(venue["rating"])["rating"])),
			"is_favorite":        asBool(coalesceAny(venue["favourite"], venue["favorite"], true)),
			"url":                targetURL,
			"price_range":        asInt(venue["price_range"]),
			"currency":           strings.TrimSpace(asString(venue["currency"])),
			"country":            strings.TrimSpace(asString(venue["country"])),
			"delivery_price_int": asInt(venue["delivery_price_int"]),
			"estimate": strings.TrimSpace(asString(coalesceAny(
				venue["estimate"],
				asMap(venue["estimate_box"])["subtitle"],
				asMap(venue["estimate_box"])["title"],
			))),
		}
		*acc = append(*acc, row)
	}
}

func formatFavoriteRating(value any) string {
	switch rating := value.(type) {
	case string:
		return strings.TrimSpace(rating)
	case float64:
		return fmt.Sprintf("%.1f", rating)
	case float32:
		return fmt.Sprintf("%.1f", rating)
	case int:
		return asString(rating)
	case int64:
		return asString(rating)
	default:
		return ""
	}
}

func buildProfileFavoritesTable(data map[string]any) string {
	headers := []string{"Name", "Slug", "Venue ID", "Rating", "Address"}
	rows := [][]string{}
	for _, value := range asSlice(data["favorites"]) {
		row := asMap(value)
		rows = append(rows, []string{
			fallbackString(asString(row["name"]), "-"),
			fallbackString(asString(row["slug"]), "-"),
			fallbackString(asString(row["venue_id"]), "-"),
			fallbackString(asString(row["rating"]), "-"),
			fallbackString(asString(row["address"]), "-"),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-", "-"})
	}
	return output.RenderTable("Favourite venues", headers, rows)
}

func venueSlugFromInput(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && strings.TrimSpace(parsed.Host) != "" {
		value = parsed.Path
	}
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char == '/'
	})
	if len(parts) == 0 {
		return strings.TrimSpace(value)
	}
	for i := 0; i < len(parts)-1; i++ {
		if strings.EqualFold(parts[i], "restaurant") {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return strings.TrimSpace(parts[len(parts)-1])
}
