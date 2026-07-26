package observability

import (
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func deliveryAvailableAtLocation(
	staticPayload map[string]any,
	dynamicPayload map[string]any,
	location *domain.Location,
) (bool, bool) {
	if location == nil {
		return false, false
	}
	for _, geoRange := range []any{
		venuePayload(staticPayload)["delivery_geo_range"],
		toMap(toMap(staticPayload["venue_raw"])["delivery_specs"])["geo_range"],
		venuePayload(dynamicPayload)["delivery_geo_range"],
		toMap(toMap(dynamicPayload["venue_raw"])["delivery_specs"])["geo_range"],
	} {
		if inside, known := pointInGeoRange(location.Lon, location.Lat, geoRange); known {
			return inside, true
		}
	}
	return false, false
}

func pointInGeoRange(lon float64, lat float64, raw any) (bool, bool) {
	geometry := toMap(raw)
	if geometry == nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(geometry["type"]))) {
	case "feature":
		return pointInGeoRange(lon, lat, geometry["geometry"])
	case "polygon":
		return pointInPolygonCoordinates(lon, lat, toSlice(geometry["coordinates"]))
	case "multipolygon":
		return pointInMultiPolygonCoordinates(lon, lat, toSlice(geometry["coordinates"]))
	}

	// Some Wolt payloads contain a GeoJSON geometry without the optional type.
	coordinates := toSlice(geometry["coordinates"])
	if len(coordinates) == 0 {
		return false, false
	}
	first := toSlice(coordinates[0])
	if len(first) == 0 {
		return false, false
	}
	if _, _, ok := coordinatePair(first[0]); ok {
		return pointInPolygonCoordinates(lon, lat, coordinates)
	}
	return pointInMultiPolygonCoordinates(lon, lat, coordinates)
}

func pointInMultiPolygonCoordinates(lon float64, lat float64, polygons []any) (bool, bool) {
	if len(polygons) == 0 {
		return false, false
	}
	allKnown := true
	for _, rawPolygon := range polygons {
		inside, polygonKnown := pointInPolygonCoordinates(lon, lat, toSlice(rawPolygon))
		if !polygonKnown {
			allKnown = false
		}
		if polygonKnown && inside {
			return true, true
		}
	}
	return false, allKnown
}

func pointInPolygonCoordinates(lon float64, lat float64, rings []any) (bool, bool) {
	if len(rings) == 0 {
		return false, false
	}
	insideOuter, outerKnown := pointInRing(lon, lat, toSlice(rings[0]))
	if !outerKnown || !insideOuter {
		return false, outerKnown
	}
	allHolesKnown := true
	for _, rawHole := range rings[1:] {
		insideHole, holeKnown := pointInRing(lon, lat, toSlice(rawHole))
		if !holeKnown {
			allHolesKnown = false
		}
		if holeKnown && insideHole {
			return false, true
		}
	}
	if !allHolesKnown {
		return false, false
	}
	return true, true
}

func pointInRing(lon float64, lat float64, rawPoints []any) (bool, bool) {
	if len(rawPoints) < 3 {
		return false, false
	}
	points := make([][2]float64, 0, len(rawPoints))
	for _, rawPoint := range rawPoints {
		pointLon, pointLat, ok := coordinatePair(rawPoint)
		if !ok {
			return false, false
		}
		points = append(points, [2]float64{pointLon, pointLat})
	}

	inside := false
	j := len(points) - 1
	for i := range points {
		xi, yi := points[i][0], points[i][1]
		xj, yj := points[j][0], points[j][1]
		if pointOnSegment(lon, lat, xi, yi, xj, yj) {
			return true, true
		}
		if (yi > lat) != (yj > lat) && lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside, true
}

func coordinatePair(raw any) (float64, float64, bool) {
	values := toSlice(raw)
	if len(values) < 2 {
		return 0, 0, false
	}
	lon, lonOK := numericValue(values[0])
	lat, latOK := numericValue(values[1])
	return lon, lat, lonOK && latOK
}

func numericValue(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

func pointOnSegment(px float64, py float64, ax float64, ay float64, bx float64, by float64) bool {
	const epsilon = 1e-10
	cross := (px-ax)*(by-ay) - (py-ay)*(bx-ax)
	if cross < -epsilon || cross > epsilon {
		return false
	}
	return px >= minFloat(ax, bx)-epsilon &&
		px <= maxFloat(ax, bx)+epsilon &&
		py >= minFloat(ay, by)-epsilon &&
		py <= maxFloat(ay, by)+epsilon
}

func minFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
