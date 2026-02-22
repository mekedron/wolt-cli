package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/output"
	"github.com/spf13/cobra"
)

func newProfileCommand(deps Dependencies) *cobra.Command {
	profile := &cobra.Command{
		Use:   "profile",
		Short: "Inspect profile, orders, address, and payment details.",
	}
	profile.AddCommand(newProfileStatusCommand(deps))
	profile.AddCommand(newProfileShowCommand(deps))
	profile.AddCommand(newProfileOrdersCommand(deps))
	profile.AddCommand(newProfileAddressesCommand(deps))
	profile.AddCommand(newProfilePaymentsCommand(deps))
	profile.AddCommand(newProfileFavoritesCommand(deps))
	return profile
}

func newProfileStatusCommand(deps Dependencies) *cobra.Command {
	cmd := newAuthStatusCommand(deps)
	cmd.Short = "Check whether selected credentials are authenticated."
	return cmd
}

func newProfileShowCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var include string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show authenticated profile summary.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.UserMe(cmd.Context(), authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			data := buildProfileSummary(payload, splitCSV(include))

			if format == output.FormatTable {
				return writeTable(cmd, buildProfileSummaryTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&include, "include", "", "Include fields: personal,settings")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfilePaymentsCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var maskSensitive bool
	var labelFilter string

	cmd := &cobra.Command{
		Use:   "payments",
		Short: "List available payment methods.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}

			result, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (profilePaymentsPayload, error) {
					return fetchProfilePaymentsPayload(cmd.Context(), deps, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			authWarnings = append(authWarnings, result.Warnings...)
			methods := extractPaymentMethods(result.Payload, maskSensitive)
			methods = filterPaymentMethodsByLabel(methods, labelFilter)
			data := map[string]any{"methods": methods}

			if format == output.FormatTable {
				return writeTable(cmd, buildProfilePaymentsTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().BoolVar(&maskSensitive, "mask-sensitive", false, "Mask sensitive payment labels.")
	cmd.Flags().StringVar(&labelFilter, "label", "", "Filter payment methods by case-insensitive label text.")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileAddressesCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var activeOnly bool

	cmd := &cobra.Command{
		Use:   "addresses",
		Short: "List and manage Wolt account addresses.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}
			profile, _ := deps.Profiles.Find(cmd.Context(), flags.Profile)
			profileAddressID := strings.TrimSpace(profile.WoltAddressID)

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeliveryInfoList(cmd.Context(), authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			rows := extractDeliveryAddresses(payload, profileAddressID)
			if activeOnly && profileAddressID != "" {
				filtered := make([]any, 0, 1)
				for _, value := range rows {
					row := asMap(value)
					if strings.EqualFold(asString(row["address_id"]), profileAddressID) {
						filtered = append(filtered, row)
						break
					}
				}
				rows = filtered
			}

			data := map[string]any{
				"addresses":                  rows,
				"profile_default_address_id": profileAddressID,
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildProfileAddressesTable(data), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "Only return profile-selected default Wolt address.")
	addGlobalFlags(cmd, &flags)
	cmd.AddCommand(newProfileAddressesAddCommand(deps))
	cmd.AddCommand(newProfileAddressesLinksCommand(deps))
	cmd.AddCommand(newProfileAddressesRemoveCommand(deps))
	cmd.AddCommand(newProfileAddressesUseCommand(deps))
	cmd.AddCommand(newProfileAddressesUpdateCommand(deps))
	return cmd
}

func newProfileAddressesLinksCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "links [address-id]",
		Short: "Generate Google Maps validation links for address and entrance.",
		Args:  cobra.MaximumNArgs(1),
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
			profile, _ := deps.Profiles.Find(cmd.Context(), flags.Profile)
			addressID := strings.TrimSpace(profile.WoltAddressID)
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				addressID = strings.TrimSpace(args[0])
			}
			if addressID == "" {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_INVALID_ARGUMENT", "address id is required (pass argument or set profile default via `profile addresses use`)")
			}

			payload, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeliveryInfoList(cmd.Context(), authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			entry := findDeliveryAddressByID(payload, addressID)
			if entry == nil {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_NOT_FOUND", fmt.Sprintf("address id %q not found", addressID))
			}
			links := buildAddressMapLinks(entry)
			data := map[string]any{
				"address_id": addressID,
				"links":      links,
			}

			if format == output.FormatTable {
				rows := [][]string{
					{"Address link", fallbackString(asString(links["address_link"]), "-")},
					{"Entrance link", fallbackString(asString(links["entrance_link"]), "-")},
					{"Coordinates link", fallbackString(asString(links["coordinates_link"]), "-")},
				}
				return writeTable(cmd, output.RenderTable("Maps links", []string{"Type", "URL"}, rows), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileAddressesAddCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var address string
	var lat float64
	var lon float64
	var locationType string
	var details []string
	var setDefault bool
	var label string
	var alias string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a saved Wolt delivery address.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			auth := buildAuthContextWithProfile(cmd.Context(), deps, flags)
			if err := requireAuth(cmd, format, profileName, flags.Locale, flags.Output, auth); err != nil {
				return err
			}
			payload, err := buildDeliveryInfoPayload(address, lat, lon, locationType, details, label, alias, "")
			if err != nil {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_INVALID_ARGUMENT", err.Error())
			}

			created, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeliveryInfoCreate(cmd.Context(), payload, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			if setDefault {
				_ = setProfileWoltAddressID(cmd.Context(), deps, flags.Profile, asString(created["id"]))
			}
			data := map[string]any{
				"address": map[string]any{
					"address_id": asString(created["id"]),
					"label":      asString(created["label_type"]),
					"street":     asString(asMap(created["location"])["address"]),
					"is_default": setDefault,
				},
			}

			if format == output.FormatTable {
				return writeTable(cmd, buildProfileAddressMutationTable("Address added", data["address"]), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}

	cmd.Flags().StringVar(&address, "address", "", "Formatted address text.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude for the saved address.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude for the saved address.")
	cmd.Flags().StringVar(&locationType, "type", "other", "Location type: apartment, office, house, outdoor, other.")
	cmd.Flags().StringArrayVar(&details, "detail", nil, "Address-form key=value pair (repeatable), e.g. --detail other_address_details=back door.")
	cmd.Flags().BoolVar(&setDefault, "set-default-profile", false, "Save created address as default Wolt address for the selected profile.")
	cmd.Flags().StringVar(&label, "label", "other", "Address label type: home, work, or other.")
	cmd.Flags().StringVar(&alias, "alias", "", "Address label alias text (used for other/custom labels).")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("lat")
	_ = cmd.MarkFlagRequired("lon")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileAddressesRemoveCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "remove <address-id>",
		Short: "Remove a saved Wolt delivery address.",
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

			addressID := strings.TrimSpace(args[0])
			_, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeliveryInfoDelete(cmd.Context(), addressID, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			profile, _ := deps.Profiles.Find(cmd.Context(), flags.Profile)
			if strings.EqualFold(strings.TrimSpace(profile.WoltAddressID), addressID) {
				_ = setProfileWoltAddressID(cmd.Context(), deps, flags.Profile, "")
			}

			data := map[string]any{"address_id": addressID, "removed": true}
			if format == output.FormatTable {
				return writeTable(cmd, output.RenderTable("Address removed", []string{"Address ID", "Removed"}, [][]string{{addressID, "yes"}}), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileAddressesUseCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags

	cmd := &cobra.Command{
		Use:   "use <address-id>",
		Short: "Set profile default Wolt address ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(flags.Format)
			if err != nil {
				return err
			}
			profileName := defaultProfileName(flags.Profile)
			addressID := strings.TrimSpace(args[0])
			if err := setProfileWoltAddressID(cmd.Context(), deps, flags.Profile, addressID); err != nil {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_PROFILE_ERROR", err.Error())
			}

			data := map[string]any{"profile_default_address_id": addressID}
			if format == output.FormatTable {
				return writeTable(cmd, output.RenderTable("Profile address", []string{"Field", "Value"}, [][]string{{"Default Wolt address ID", addressID}}), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, nil, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	addGlobalFlags(cmd, &flags)
	return cmd
}

func newProfileAddressesUpdateCommand(deps Dependencies) *cobra.Command {
	var flags globalFlags
	var address string
	var lat float64
	var lon float64
	var locationType string
	var details []string
	var setDefault bool
	var label string
	var alias string

	cmd := &cobra.Command{
		Use:   "update <address-id>",
		Short: "Update an address by replacing it with a new saved entry.",
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
			oldID := strings.TrimSpace(args[0])
			payload, err := buildDeliveryInfoPayload(address, lat, lon, locationType, details, label, alias, oldID)
			if err != nil {
				return emitError(cmd, format, profileName, flags.Locale, flags.Output, "WOLT_INVALID_ARGUMENT", err.Error())
			}

			created, authWarnings, err := invokeWithAuthAutoRefresh(
				cmd.Context(),
				deps,
				flags,
				&auth,
				func(authCtx woltgateway.AuthContext) (map[string]any, error) {
					return deps.Wolt.DeliveryInfoCreate(cmd.Context(), payload, authCtx)
				},
			)
			if err != nil {
				return emitUpstreamError(cmd, format, profileName, flags.Locale, flags.Output, flags.Verbose, err)
			}
			newID := strings.TrimSpace(asString(created["id"]))
			if setDefault || oldID != "" {
				profile, _ := deps.Profiles.Find(cmd.Context(), flags.Profile)
				if setDefault || strings.EqualFold(strings.TrimSpace(profile.WoltAddressID), oldID) {
					_ = setProfileWoltAddressID(cmd.Context(), deps, flags.Profile, newID)
				}
			}

			data := map[string]any{"replaced_address_id": oldID, "new_address_id": newID}
			if format == output.FormatTable {
				return writeTable(cmd, output.RenderTable("Address updated", []string{"Field", "Value"}, [][]string{{"Replaced ID", oldID}, {"New ID", fallbackString(newID, "-")}}), flags.Output)
			}
			env := output.BuildEnvelope(profileName, flags.Locale, data, authWarnings, nil)
			return writeMachinePayload(cmd, env, format, flags.Output)
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Formatted address text.")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude for the saved address.")
	cmd.Flags().Float64Var(&lon, "lon", 0, "Longitude for the saved address.")
	cmd.Flags().StringVar(&locationType, "type", "other", "Location type: apartment, office, house, outdoor, other.")
	cmd.Flags().StringArrayVar(&details, "detail", nil, "Address-form key=value pair (repeatable).")
	cmd.Flags().BoolVar(&setDefault, "set-default-profile", false, "Save updated address as default Wolt address for the selected profile.")
	cmd.Flags().StringVar(&label, "label", "other", "Address label type: home, work, or other.")
	cmd.Flags().StringVar(&alias, "alias", "", "Address label alias text (used for other/custom labels).")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("lat")
	_ = cmd.MarkFlagRequired("lon")
	addGlobalFlags(cmd, &flags)
	return cmd
}

func buildProfileSummary(payload map[string]any, include map[string]struct{}) map[string]any {
	user := asMap(payload["user"])
	name := asMap(user["name"])
	first := strings.TrimSpace(asString(name["first_name"]))
	last := strings.TrimSpace(asString(name["last_name"]))
	fullName := strings.TrimSpace(strings.TrimSpace(first + " " + last))

	data := map[string]any{
		"user_id":      domain.NormalizeID(coalesceAny(user["_id"], user["id"])),
		"name":         fullName,
		"email_masked": maskEmail(asString(user["email"])),
		"phone_masked": maskPhone(asString(user["phone_number"])),
		"country":      asString(user["country"]),
	}
	if _, ok := include["personal"]; ok {
		data["personal"] = map[string]any{
			"name":         name,
			"email":        asString(user["email"]),
			"phone_number": asString(user["phone_number"]),
		}
	}
	if _, ok := include["settings"]; ok {
		data["settings"] = coalesceAny(user["settings"], map[string]any{})
	}
	return data
}

func buildProfileSummaryTable(data map[string]any) string {
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"User ID", fallbackString(asString(data["user_id"]), "-")},
		{"Name", fallbackString(asString(data["name"]), "-")},
		{"Email", fallbackString(asString(data["email_masked"]), "-")},
		{"Phone", fallbackString(asString(data["phone_masked"]), "-")},
		{"Country", fallbackString(asString(data["country"]), "-")},
	}
	return output.RenderTable("Profile", headers, rows)
}

type profilePaymentsPayload struct {
	Payload  map[string]any
	Warnings []string
}

func fetchProfilePaymentsPayload(ctx context.Context, deps Dependencies, auth woltgateway.AuthContext) (profilePaymentsPayload, error) {
	result := profilePaymentsPayload{
		Payload:  map[string]any{},
		Warnings: []string{},
	}

	savedPayload, err := deps.Wolt.PaymentMethods(ctx, auth)
	if err != nil {
		return result, err
	}
	result.Payload["saved"] = savedPayload

	country := paymentCountryFromToken(auth.WToken)
	if country == "" {
		userPayload, userErr := deps.Wolt.UserMe(ctx, auth)
		if userErr != nil {
			if isUnauthorizedUpstream(userErr) {
				return result, userErr
			}
			result.Warnings = append(result.Warnings, "payment profile lookup skipped: unable to resolve user country")
		} else {
			result.Payload["user"] = userPayload
			country = paymentCountryFromUserMe(userPayload)
		}
	}

	if country == "" {
		return result, nil
	}

	profilePayload, profileErr := deps.Wolt.PaymentMethodsProfile(
		ctx,
		auth,
		woltgateway.PaymentMethodsProfileOptions{
			Country: country,
		},
	)
	if profileErr != nil {
		if isUnauthorizedUpstream(profileErr) {
			return result, profileErr
		}
		result.Warnings = append(result.Warnings, "payment profile lookup failed; showing saved methods only")
		return result, nil
	}
	result.Payload["profile"] = profilePayload
	return result, nil
}

func paymentCountryFromUserMe(payload map[string]any) string {
	user := asMap(payload["user"])
	return strings.ToUpper(strings.TrimSpace(asString(coalesceAny(user["country"], payload["country"]))))
}

func paymentCountryFromToken(token string) string {
	claims := tokenClaims(token)
	if claims == nil {
		return ""
	}
	user := asMap(claims["user"])
	country := strings.TrimSpace(asString(coalesceAny(user["country"], claims["country"])))
	return strings.ToUpper(country)
}

func tokenClaims(token string) map[string]any {
	token = normalizeWToken(token)
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return nil
	}
	return claims
}

func extractPaymentMethods(payload map[string]any, maskSensitive bool) []any {
	profileCandidates := []map[string]any{}
	if profilePayload := asMap(payload["profile"]); profilePayload != nil {
		profileCandidates = append(profileCandidates, extractProfilePaymentMethodCandidates(profilePayload)...)
	}
	if len(profileCandidates) == 0 {
		profileCandidates = append(profileCandidates, extractProfilePaymentMethodCandidates(payload)...)
	}
	if len(profileCandidates) > 0 {
		return normalizePaymentMethodList(profileCandidates, maskSensitive)
	}

	legacyCandidates := []map[string]any{}
	if savedPayload := asMap(payload["saved"]); savedPayload != nil {
		legacyCandidates = append(legacyCandidates, extractLegacyPaymentMethodCandidates(savedPayload)...)
	}
	if len(legacyCandidates) == 0 {
		legacyCandidates = append(legacyCandidates, extractLegacyPaymentMethodCandidates(payload)...)
	}
	return normalizePaymentMethodList(legacyCandidates, maskSensitive)
}

func normalizePaymentMethodList(candidates []map[string]any, maskSensitive bool) []any {
	methods := make([]any, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		normalized := normalizePaymentMethod(candidate, maskSensitive)
		if normalized == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(asString(normalized["method_id"])))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(asString(normalized["type"]) + "|" + asString(normalized["label"])))
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		methods = append(methods, normalized)
	}
	return methods
}

func filterPaymentMethodsByLabel(methods []any, rawFilter string) []any {
	filter := strings.ToLower(strings.TrimSpace(rawFilter))
	if filter == "" {
		return methods
	}
	filtered := make([]any, 0, len(methods))
	for _, value := range methods {
		method := asMap(value)
		if method == nil {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(asString(method["label"])))
		if strings.Contains(label, filter) {
			filtered = append(filtered, method)
		}
	}
	return filtered
}

func extractLegacyPaymentMethodCandidates(payload map[string]any) []map[string]any {
	source := asSlice(payload["methods"])
	if len(source) == 0 {
		source = asSlice(payload["payment_methods"])
	}
	if len(source) == 0 {
		source = asSlice(payload["results"])
	}
	if len(source) == 0 {
		results := asMap(payload["results"])
		source = asSlice(results["cards"])
		if len(source) == 0 {
			source = asSlice(results["payment_methods"])
		}
	}
	if len(source) == 0 {
		data := asMap(payload["data"])
		source = asSlice(data["cards"])
	}
	candidates := make([]map[string]any, 0, len(source))
	for _, value := range source {
		method := asMap(value)
		if method == nil {
			continue
		}
		candidates = append(candidates, method)
	}
	return candidates
}

func extractProfilePaymentMethodCandidates(payload map[string]any) []map[string]any {
	var nodes []map[string]any
	root := coalesceAny(payload["root_element"], payload["rootElement"])
	if root != nil {
		collectPaymentMethodNodes(root, &nodes)
	}
	if len(nodes) == 0 {
		collectPaymentMethodNodes(payload, &nodes)
	}
	return nodes
}

func collectPaymentMethodNodes(raw any, acc *[]map[string]any) {
	switch value := raw.(type) {
	case map[string]any:
		elementType := strings.TrimSpace(asString(value["element_type"]))
		if strings.EqualFold(elementType, "payment-method") {
			*acc = append(*acc, value)
		}
		for _, nested := range value {
			collectPaymentMethodNodes(nested, acc)
		}
	case []any:
		for _, entry := range value {
			collectPaymentMethodNodes(entry, acc)
		}
	}
}

func normalizePaymentMethod(method map[string]any, maskSensitive bool) map[string]any {
	methodID := strings.TrimSpace(asString(coalesceAny(
		method["method_id"],
		method["payment_method_id"],
		method["id"],
		nestedMapValue(method, "method_id", "payment_method_id", "id"),
	)))
	methodType := strings.TrimSpace(asString(coalesceAny(
		method["type"],
		method["payment_method_type"],
		method["method_type"],
		nestedMapValue(method, "type", "payment_method_type", "method_type"),
	)))
	if strings.EqualFold(methodType, "payment-method") || methodType == "" {
		methodType = strings.TrimSpace(asString(coalesceAny(
			method["method"],
			method["type_id"],
			method["id"],
			nestedMapValue(method, "method", "method_id", "type_id", "id"),
		)))
	}

	label := strings.TrimSpace(asString(coalesceAny(
		method["label"],
		method["name"],
		method["card_brand"],
		method["display_name"],
		method["title"],
		method["subtitle"],
		method["description"],
		method["text"],
		nestedMapValue(method, "label", "name", "display_name", "title", "subtitle", "description", "text"),
	)))
	last4 := strings.TrimSpace(asString(coalesceAny(
		method["card_last_four"],
		method["last4"],
		method["last_four"],
		nestedMapValue(method, "card_last_four", "last4", "last_four"),
	)))
	if label == "" && last4 != "" {
		label = "**** " + last4
	}
	if label == "" {
		label = strings.TrimSpace(asString(coalesceAny(methodType, methodID)))
	}
	if label == "" {
		return nil
	}
	methodType = normalizePaymentMethodType(methodType, methodID, label)
	if maskSensitive {
		label = maskPaymentLabel(label)
	}

	return map[string]any{
		"method_id": methodID,
		"type":      methodType,
		"label":     label,
		"is_default": asBool(coalesceAny(
			method["is_default"],
			method["default"],
			method["selected"],
			method["is_selected"],
			nestedMapValue(method, "is_default", "default", "selected", "is_selected"),
		)),
		"is_available_for_checkout": coalesceAny(
			method["is_available_for_checkout"],
			method["is_available"],
			method["enabled"],
			nestedMapValue(method, "is_available_for_checkout", "is_available", "enabled"),
			true,
		),
	}
}

func nestedMapValue(raw any, keys ...string) any {
	if len(keys) == 0 {
		return nil
	}
	targets := map[string]struct{}{}
	for _, key := range keys {
		targets[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	var visit func(any) any
	visit = func(value any) any {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if _, ok := targets[strings.ToLower(strings.TrimSpace(key))]; !ok {
					continue
				}
				if text, isText := nested.(string); isText && strings.TrimSpace(text) == "" {
					continue
				}
				return nested
			}
			for _, nested := range typed {
				if found := visit(nested); found != nil {
					return found
				}
			}
		case []any:
			for _, nested := range typed {
				if found := visit(nested); found != nil {
					return found
				}
			}
		}
		return nil
	}
	return visit(raw)
}

func normalizePaymentMethodType(methodType string, methodID string, label string) string {
	normalizedType := strings.ToLower(strings.TrimSpace(methodType))
	switch normalizedType {
	case "edit_card", "add_card", "update_card", "remove_card":
		return "card"
	}
	if normalizedType != "" && !isActionPaymentType(normalizedType) {
		return normalizedType
	}
	candidateID := strings.ToLower(strings.TrimSpace(methodID))
	if candidateID != "" && !isActionPaymentType(candidateID) && !isOpaquePaymentIdentifier(candidateID) {
		return candidateID
	}
	return paymentLabelType(label)
}

func isActionPaymentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "link_method", "unlink_method", "set_default_method", "remove_method", "edit_card", "add_card", "update_card", "remove_card":
		return true
	default:
		return false
	}
}

func isOpaquePaymentIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if len(value) >= 20 {
		return true
	}
	letters := 0
	digits := 0
	symbols := 0
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			letters++
		case char >= 'A' && char <= 'Z':
			letters++
		case char >= '0' && char <= '9':
			digits++
		default:
			symbols++
		}
	}
	return letters > 0 && digits > 0 && symbols > 0
}

func paymentLabelType(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(label))
	for _, char := range label {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func buildProfilePaymentsTable(data map[string]any) string {
	headers := []string{"Label", "Type", "Default", "Available"}
	rows := [][]string{}
	for _, value := range asSlice(data["methods"]) {
		method := asMap(value)
		rows = append(rows, []string{
			fallbackString(asString(method["label"]), "-"),
			fallbackString(asString(method["type"]), "-"),
			boolToYesNo(asBool(method["is_default"])),
			boolToYesNo(asBool(method["is_available_for_checkout"])),
		})
	}
	return output.RenderTable("Payment methods", headers, rows)
}

func buildProfileAddressesTable(data map[string]any) string {
	headers := []string{"ID", "Type", "Address", "Profile default"}
	rows := [][]string{}
	for _, value := range asSlice(data["addresses"]) {
		address := asMap(value)
		rows = append(rows, []string{
			fallbackString(asString(address["address_id"]), "-"),
			fallbackString(asString(address["label"]), "-"),
			fallbackString(asString(address["street"]), "-"),
			boolToYesNo(asBool(address["is_default"])),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"-", "-", "-", "-"})
	}
	return output.RenderTable("Addresses", headers, rows)
}

func buildProfileAddressMutationTable(title string, raw any) string {
	row := asMap(raw)
	headers := []string{"Field", "Value"}
	rows := [][]string{
		{"Address ID", fallbackString(asString(row["address_id"]), "-")},
		{"Type", fallbackString(asString(row["label"]), "-")},
		{"Address", fallbackString(asString(row["street"]), "-")},
		{"Profile default", boolToYesNo(asBool(row["is_default"]))},
	}
	return output.RenderTable(title, headers, rows)
}

func extractDeliveryAddresses(payload map[string]any, profileAddressID string) []any {
	rows := asSlice(payload["results"])
	if len(rows) == 0 {
		rows = asSlice(payload["addresses"])
	}
	parsed := make([]any, 0, len(rows))
	for _, value := range rows {
		address := asMap(value)
		location := asMap(address["location"])
		id := strings.TrimSpace(asString(address["id"]))
		parsed = append(parsed, map[string]any{
			"address_id": id,
			"label":      fallbackString(asString(coalesceAny(address["label_type"], location["location_type"])), "other"),
			"street":     asString(location["address"]),
			"is_default": id != "" && strings.EqualFold(id, strings.TrimSpace(profileAddressID)),
		})
	}
	return parsed
}

func findDeliveryAddressByID(payload map[string]any, addressID string) map[string]any {
	addressID = strings.TrimSpace(addressID)
	if addressID == "" {
		return nil
	}
	for _, value := range asSlice(payload["results"]) {
		entry := asMap(value)
		if strings.EqualFold(strings.TrimSpace(asString(entry["id"])), addressID) {
			return entry
		}
	}
	for _, value := range asSlice(payload["addresses"]) {
		entry := asMap(value)
		if strings.EqualFold(strings.TrimSpace(asString(entry["id"])), addressID) {
			return entry
		}
	}
	return nil
}

func buildAddressMapLinks(entry map[string]any) map[string]any {
	location := asMap(entry["location"])
	address := strings.TrimSpace(asString(location["address"]))
	addressForm := asMap(coalesceAny(location["address_form_data"], location["address_form_fields"]))
	lat, lon := extractGeoPoint(location["user_coordinates"])
	if (lat == 0 || lon == 0) && address != "" {
		lat, lon = extractGeoPoint(location["google_place_coordinates"])
	}

	addressQuery := address
	if lat != 0 || lon != 0 {
		addressQuery = fmt.Sprintf("%f,%f", lat, lon)
	}
	entranceParts := make([]string, 0, 4)
	if detail := strings.TrimSpace(asString(addressForm["other_address_details"])); detail != "" {
		entranceParts = append(entranceParts, detail)
	}
	if entrance := strings.TrimSpace(asString(addressForm["entrance"])); entrance != "" {
		entranceParts = append(entranceParts, "Entrance "+entrance)
	}
	if floor := strings.TrimSpace(asString(addressForm["floor"])); floor != "" {
		entranceParts = append(entranceParts, "Floor "+floor)
	}
	if apartment := strings.TrimSpace(asString(addressForm["apartment"])); apartment != "" {
		entranceParts = append(entranceParts, "Apartment "+apartment)
	}
	if notes := strings.TrimSpace(asString(addressForm["additional_instructions"])); notes != "" {
		entranceParts = append(entranceParts, notes)
	}
	entranceQuery := strings.TrimSpace(strings.Join(append([]string{address}, entranceParts...), ", "))
	if entranceQuery == "" {
		entranceQuery = addressQuery
	}
	return map[string]any{
		"address_link":     googleMapsSearchLink(addressQuery),
		"entrance_link":    googleMapsSearchLink(entranceQuery),
		"coordinates_link": googleMapsSearchLink(fmt.Sprintf("%f,%f", lat, lon)),
	}
}

func extractGeoPoint(raw any) (float64, float64) {
	point := asMap(raw)
	coords := asSlice(point["coordinates"])
	if len(coords) < 2 {
		return 0, 0
	}
	lon, okLon := coords[0].(float64)
	lat, okLat := coords[1].(float64)
	if !okLon || !okLat {
		return 0, 0
	}
	return lat, lon
}

func googleMapsSearchLink(query string) string {
	query = strings.TrimSpace(query)
	if query == "" || strings.EqualFold(query, "0.000000,0.000000") {
		return ""
	}
	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(query)
}

func buildDeliveryInfoPayload(
	address string,
	lat float64,
	lon float64,
	locationType string,
	detailInputs []string,
	labelType string,
	alias string,
	previousVersion string,
) (map[string]any, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}
	locationType = normalizeDeliveryLocationType(locationType)
	if locationType == "" {
		return nil, fmt.Errorf("type must be one of apartment, office, house, outdoor, other")
	}
	labelType = normalizeAddressLabel(labelType)
	if labelType == "" {
		return nil, fmt.Errorf("label must be one of home, work, other")
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		switch labelType {
		case "home":
			alias = "Home"
		case "work":
			alias = "Work"
		}
	}
	details, err := parseDetailPairs(detailInputs)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"label_type": labelType,
		"location": map[string]any{
			"location_type":                     locationType,
			"address_form_fields":               details,
			"address":                           address,
			"city":                              "",
			"country":                           "FIN",
			"override_google_place_coordinates": false,
			"user_coordinates": map[string]any{
				"type":        "Point",
				"coordinates": []any{lon, lat},
			},
			"google_place_coordinates": map[string]any{
				"type":        "Point",
				"coordinates": []any{lon, lat},
			},
		},
	}
	if alias != "" {
		payload["alias"] = alias
	}
	if strings.TrimSpace(previousVersion) != "" {
		payload["previous_version"] = strings.TrimSpace(previousVersion)
	}
	return payload, nil
}

func parseDetailPairs(inputs []string) (map[string]any, error) {
	details := map[string]any{}
	for _, input := range inputs {
		trimmed := strings.TrimSpace(input)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --detail %q, expected key=value", input)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid --detail %q, key is required", input)
		}
		details[key] = value
	}
	return details, nil
}

func normalizeDeliveryLocationType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "apartment", "office", "house", "outdoor", "other":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeAddressLabel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "home", "work", "other":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func setProfileWoltAddressID(ctx context.Context, deps Dependencies, selectedProfile string, addressID string) error {
	if deps.Config == nil {
		return fmt.Errorf("config store is not available")
	}
	cfg, err := deps.Config.Load(ctx)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(selectedProfile)
	idx := -1
	if target != "" {
		for i, profile := range cfg.Profiles {
			if strings.EqualFold(strings.TrimSpace(profile.Name), target) {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		for i, profile := range cfg.Profiles {
			if profile.IsDefault {
				idx = i
				break
			}
		}
	}
	if idx < 0 && len(cfg.Profiles) == 1 {
		idx = 0
	}
	if idx < 0 {
		return fmt.Errorf("profile %q not found", defaultProfileName(selectedProfile))
	}
	cfg.Profiles[idx].WoltAddressID = strings.TrimSpace(addressID)
	return deps.Config.Save(ctx, cfg)
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	if len(local) <= 2 {
		return "***@" + parts[1]
	}
	return local[:2] + "***@" + parts[1]
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}

func maskPaymentLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if len(label) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(label)-4) + label[len(label)-4:]
}
