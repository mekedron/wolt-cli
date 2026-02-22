package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

func resolveAccountLocation(
	ctx context.Context,
	deps Dependencies,
	profile domain.Profile,
	authOverride *woltgateway.AuthContext,
) (domain.Location, error) {
	if deps.Wolt == nil {
		return domain.Location{}, fmt.Errorf("wolt api client is not available")
	}
	auth := authContextFromProfile(profile)
	if authOverride != nil && authOverride.HasCredentials() {
		auth = *authOverride
	}
	if !auth.HasCredentials() {
		return domain.Location{}, fmt.Errorf("no auth credentials available for Wolt account address lookup")
	}

	payload, _, err := invokeWithAuthAutoRefresh(
		ctx,
		deps,
		globalFlags{Profile: strings.TrimSpace(profile.Name)},
		&auth,
		func(authCtx woltgateway.AuthContext) (map[string]any, error) {
			return deps.Wolt.DeliveryInfoList(ctx, authCtx)
		},
	)
	if err != nil {
		return domain.Location{}, fmt.Errorf("wolt account address lookup failed: %w", err)
	}

	location, ok := deliveryInfoLocation(payload, strings.TrimSpace(profile.WoltAddressID))
	if !ok {
		return domain.Location{}, fmt.Errorf("wolt account has no saved address with coordinates")
	}
	return location, nil
}

func authContextFromProfile(profile domain.Profile) woltgateway.AuthContext {
	auth := woltgateway.AuthContext{
		WToken:       normalizeWToken(profile.WToken),
		RefreshToken: normalizeRefreshToken(profile.WRefreshToken),
		Cookies:      normalizeCookieInputs(profile.Cookies),
	}
	if strings.TrimSpace(auth.WToken) == "" {
		auth.WToken = extractWTokenFromCookieInputs(auth.Cookies)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshToken(profile.WToken)
	}
	if strings.TrimSpace(auth.RefreshToken) == "" {
		auth.RefreshToken = extractRefreshTokenFromCookieInputs(auth.Cookies)
	}
	return auth
}

func deliveryInfoLocation(payload map[string]any, preferredAddressID string) (domain.Location, bool) {
	rows := asSlice(payload["results"])
	if len(rows) == 0 {
		rows = asSlice(payload["addresses"])
	}

	preferredID := strings.TrimSpace(preferredAddressID)
	var first domain.Location
	hasFirst := false
	var selected domain.Location
	hasSelected := false

	for _, rawRow := range rows {
		row := asMap(rawRow)
		if row == nil {
			continue
		}
		location, ok := deliveryInfoEntryLocation(row)
		if !ok {
			continue
		}
		if !hasFirst {
			first = location
			hasFirst = true
		}
		entryID := strings.TrimSpace(asString(coalesceAny(row["id"], row["address_id"])))
		if preferredID != "" && strings.EqualFold(entryID, preferredID) {
			return location, true
		}
		if isDeliveryInfoSelected(row) && !hasSelected {
			selected = location
			hasSelected = true
		}
	}

	if hasSelected {
		return selected, true
	}
	return first, hasFirst
}

func isDeliveryInfoSelected(entry map[string]any) bool {
	if entry == nil {
		return false
	}
	keys := []string{"is_default", "default", "is_selected", "selected", "is_active", "active"}
	for _, key := range keys {
		if asBool(entry[key]) {
			return true
		}
	}
	location := asMap(entry["location"])
	for _, key := range keys {
		if asBool(location[key]) {
			return true
		}
	}
	return false
}

func deliveryInfoEntryLocation(entry map[string]any) (domain.Location, bool) {
	if entry == nil {
		return domain.Location{}, false
	}
	location := asMap(entry["location"])
	if lat, lon, ok := pointFromAny(location["user_coordinates"]); ok {
		return domain.Location{Lat: lat, Lon: lon}, true
	}
	if lat, lon, ok := pointFromAny(location["google_place_coordinates"]); ok {
		return domain.Location{Lat: lat, Lon: lon}, true
	}
	if lat, lon, ok := pointFromAny(location["coordinates"]); ok {
		return domain.Location{Lat: lat, Lon: lon}, true
	}
	lat, latOK := asFloat(location["lat"])
	lon, lonOK := asFloat(location["lon"])
	if !lonOK {
		lon, lonOK = asFloat(location["lng"])
	}
	if !lonOK {
		lon, lonOK = asFloat(location["longitude"])
	}
	if !latOK {
		lat, latOK = asFloat(location["latitude"])
	}
	if latOK && lonOK && !(lat == 0 && lon == 0) {
		return domain.Location{Lat: lat, Lon: lon}, true
	}
	return domain.Location{}, false
}

func pointFromAny(raw any) (float64, float64, bool) {
	point := asMap(raw)
	if point == nil {
		return 0, 0, false
	}
	coords := asSlice(point["coordinates"])
	if len(coords) < 2 {
		return 0, 0, false
	}
	lon, lonOK := asFloat(coords[0])
	lat, latOK := asFloat(coords[1])
	if !latOK || !lonOK || (lat == 0 && lon == 0) {
		return 0, 0, false
	}
	return lat, lon, true
}
