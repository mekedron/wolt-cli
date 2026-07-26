package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// LocationInput is the canonical lat/lon/address triple accepted by every
// location-bound MCP tool. It is embedded into tool-specific Input structs.
type LocationInput struct {
	Lat     float64 `json:"lat,omitempty"     jsonschema:"latitude in decimal degrees; if set, lon must also be set"`
	Lon     float64 `json:"lon,omitempty"     jsonschema:"longitude in decimal degrees; if set, lat must also be set"`
	Address string  `json:"address,omitempty" jsonschema:"address string to geocode (alternative to lat/lon); cannot be combined with lat/lon"`
}

// resolveLocation walks the standard precedence:
//  1. explicit lat/lon (both required if either is given)
//  2. address → geocode via Nominatim
//  3. persisted profile.Location
//  4. live DeliveryInfoList lookup (if the user is logged in)
//
// Returns the resolved location and a label identifying which source supplied it.
func (tc *ToolCtx) resolveLocation(ctx context.Context, in LocationInput) (domain.Location, string, error) {
	address := strings.TrimSpace(in.Address)
	hasLat := in.Lat != 0
	hasLon := in.Lon != 0

	if address != "" && (hasLat || hasLon) {
		return domain.Location{}, "", fmt.Errorf("address cannot be combined with lat/lon; pass either an address or both lat and lon")
	}

	if hasLat && hasLon {
		return domain.Location{Lat: in.Lat, Lon: in.Lon}, "explicit", nil
	}
	if hasLat != hasLon {
		return domain.Location{}, "", fmt.Errorf("both lat and lon must be provided together; got lat=%v lon=%v", in.Lat, in.Lon)
	}

	if address != "" {
		if tc.location == nil {
			return domain.Location{}, "", fmt.Errorf("location resolver is unavailable; cannot geocode address")
		}
		loc, err := tc.location.Get(ctx, address)
		if err != nil {
			return domain.Location{}, "", fmt.Errorf("geocode %q failed: %w", address, err)
		}
		return loc, "address", nil
	}

	// No explicit args — fall back to the profile.
	profile, err := tc.loadProfile(ctx)
	if err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			return domain.Location{}, "", fmt.Errorf("no location provided and no saved profile: pass lat/lon or address, or run 'wolt login'")
		}
		return domain.Location{}, "", err
	}
	if profile.Location.Lat != 0 || profile.Location.Lon != 0 {
		return profile.Location, "profile", nil
	}

	auth := buildAuthContext(profile)
	if !auth.CanAuthenticate() {
		return domain.Location{}, "", fmt.Errorf("no saved location and no Wolt session; pass lat/lon or address, or run 'wolt login'")
	}
	payload, err := invokeWithRefresh(ctx, tc, &auth, func(authCtx woltgateway.AuthContext) (map[string]any, error) {
		return tc.wolt.DeliveryInfoList(ctx, authCtx)
	})
	if err != nil {
		return domain.Location{}, "", fmt.Errorf("wolt account address lookup failed: %w", err)
	}
	loc, ok := deliveryInfoLocation(payload, strings.TrimSpace(profile.WoltAddressID))
	if !ok {
		return domain.Location{}, "", fmt.Errorf("wolt account has no saved address with coordinates")
	}
	return loc, "wolt-account", nil
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
		loc, ok := deliveryInfoEntryLocation(row)
		if !ok {
			continue
		}
		if !hasFirst {
			first = loc
			hasFirst = true
		}
		entryID := strings.TrimSpace(asString(coalesceAny(row["id"], row["address_id"])))
		if preferredID != "" && strings.EqualFold(entryID, preferredID) {
			return loc, true
		}
		if isDeliveryInfoSelected(row) && !hasSelected {
			selected = loc
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
