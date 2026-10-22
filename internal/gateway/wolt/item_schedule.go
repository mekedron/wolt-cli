package wolt

import (
	"context"
	"fmt"
	"strings"
	"time"

	// Static venue pages name an IANA zone; Windows hosts ship no zoneinfo.
	_ "time/tzdata"
)

const millisPerDay = 24 * 60 * 60 * 1000

// itemScheduleSource tags a disabled_info the client derives from
// available_times, so it is distinguishable from one Wolt sent.
const itemScheduleSource = "available_times"

// Assortment items may carry available_times: weekly sale windows in the
// venue's local time. days are ISO weekdays (1 = Monday ... 7 = Sunday);
// start_time and end_time are milliseconds since local midnight. Outside its
// windows an item keeps disabled_info = null in the assortment, yet Wolt
// marks the basket line unavailable and leaves it out of the basket total.
// The client therefore writes a disabled_info for such items, which every
// availability check downstream already honours.
type itemWindow struct {
	days  map[time.Weekday]struct{}
	start int64
	end   int64
}

// offsetZoneTTL bounds how long a UTC offset read from a venue's current
// time is trusted; a long-running MCP server may outlive a DST switch.
const offsetZoneTTL = time.Hour

type venueZone struct {
	location *time.Location
	expires  time.Time // zero for an IANA zone, which never goes stale
}

func (c *Client) withItemSchedules(
	ctx context.Context,
	slug string,
	payload map[string]any,
	err error,
) (map[string]any, error) {
	if err == nil && payload != nil {
		c.markOffScheduleItems(ctx, slug, payload)
	}
	return payload, err
}

// markOffScheduleItems disables the top-level assortment items whose
// available_times exclude the current venue-local time. The venue zone is
// looked up only when some item is restricted at all; when it cannot be
// resolved, items stay as Wolt returned them.
func (c *Client) markOffScheduleItems(ctx context.Context, slug string, payload map[string]any) {
	type scheduledItem struct {
		item    map[string]any
		windows []itemWindow
	}
	restricted := []scheduledItem{}
	items, _ := payload["items"].([]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item["disabled_info"] != nil {
			continue
		}
		windows, ok := parseItemWindows(item["available_times"])
		if !ok || coversWholeWeek(windows) {
			continue
		}
		restricted = append(restricted, scheduledItem{item: item, windows: windows})
	}
	if len(restricted) == 0 {
		return
	}

	location, ok := c.venueLocation(ctx, slug)
	if !ok {
		return
	}
	now := c.clock().In(location)
	for _, entry := range restricted {
		if windowsContain(entry.windows, now) {
			continue
		}
		entry.item["disabled_info"] = map[string]any{
			"disable_text": "not sold now; available " + describeWindows(entry.windows) + " venue time",
			"source":       itemScheduleSource,
		}
	}
}

func (c *Client) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

// venueLocation returns the venue's zone. Venue pages the client already
// loaded supply it, so a menu load normally costs no extra request. Otherwise
// the dynamic venue page is asked for the venue's current time, and the
// static page, which Wolt rate-limits harder, is the last resort.
func (c *Client) venueLocation(ctx context.Context, slug string) (*time.Location, bool) {
	if location, ok := c.lookupVenueZone(slug); ok {
		return location, true
	}
	if _, err := c.VenuePageDynamic(ctx, slug, VenuePageDynamicOptions{}); err == nil {
		if location, ok := c.lookupVenueZone(slug); ok {
			return location, true
		}
	}
	if _, err := c.VenuePageStatic(ctx, slug); err == nil {
		return c.lookupVenueZone(slug)
	}
	return nil, false
}

func (c *Client) lookupVenueZone(reference string) (*time.Location, bool) {
	key := venueZoneKey(reference)
	if key == "" {
		return nil, false
	}
	cached, ok := c.venueZones.Load(key)
	if !ok {
		return nil, false
	}
	zone := cached.(venueZone)
	if !zone.expires.IsZero() && c.clock().After(zone.expires) {
		c.venueZones.Delete(key)
		return nil, false
	}
	return zone.location, true
}

// rememberVenueStaticZone records the IANA zone of a static venue page under
// the requested reference and the venue's own slug and id.
func (c *Client) rememberVenueStaticZone(reference string, payload map[string]any) {
	venue, _ := payload["venue"].(map[string]any)
	name, _ := venue["timezone"].(string)
	location, err := time.LoadLocation(strings.TrimSpace(name))
	if strings.TrimSpace(name) == "" || err != nil {
		return
	}
	c.storeVenueZone(venueZone{location: location}, reference, venue["slug"], venue["id"])
}

// rememberVenueDynamicZone records the UTC offset of the venue's current time,
// which the dynamic venue page reports as open_status.now.
func (c *Client) rememberVenueDynamicZone(reference string, payload map[string]any) {
	venue, _ := payload["venue"].(map[string]any)
	for _, key := range []string{"open_status", "delivery_open_status"} {
		status, _ := venue[key].(map[string]any)
		raw, _ := status["now"].(string)
		venueNow, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		_, offset := venueNow.Zone()
		zone := venueZone{
			location: time.FixedZone(venueNow.Format("-07:00"), offset),
			expires:  c.clock().Add(offsetZoneTTL),
		}
		c.storeVenueZone(zone, reference, venue["slug"], venue["id"])
		return
	}
}

func (c *Client) storeVenueZone(zone venueZone, references ...any) {
	for _, reference := range references {
		name, _ := reference.(string)
		if key := venueZoneKey(name); key != "" {
			c.venueZones.Store(key, zone)
		}
	}
}

func venueZoneKey(reference string) string {
	return strings.ToLower(strings.TrimSpace(reference))
}

// parseItemWindows reads available_times. Missing, empty or malformed
// schedules report false and leave the item unrestricted.
func parseItemWindows(raw any) ([]itemWindow, bool) {
	entries, ok := raw.([]any)
	if !ok || len(entries) == 0 {
		return nil, false
	}
	windows := make([]itemWindow, 0, len(entries))
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return nil, false
		}
		start, startOK := wholeNumber(entry["start_time"])
		end, endOK := wholeNumber(entry["end_time"])
		rawDays, daysOK := entry["days"].([]any)
		if !startOK || !endOK || !daysOK || start < 0 || end < 0 || start > millisPerDay || end > millisPerDay {
			return nil, false
		}
		window := itemWindow{days: map[time.Weekday]struct{}{}, start: start, end: end}
		for _, rawDay := range rawDays {
			day, ok := wholeNumber(rawDay)
			if !ok || day < 1 || day > 7 {
				return nil, false
			}
			window.days[time.Weekday(day%7)] = struct{}{}
		}
		windows = append(windows, window)
	}
	return windows, true
}

func wholeNumber(raw any) (int64, bool) {
	value, ok := raw.(float64)
	if !ok || value != float64(int64(value)) {
		return 0, false
	}
	return int64(value), true
}

func coversWholeWeek(windows []itemWindow) bool {
	for day := time.Sunday; day <= time.Saturday; day++ {
		covered := false
		for _, window := range windows {
			if _, ok := window.days[day]; ok && window.start == 0 && window.end == millisPerDay {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// windowsContain reports whether now (venue-local) falls inside a window. A
// window whose end is not after its start runs past midnight into the next
// day.
func windowsContain(windows []itemWindow, now time.Time) bool {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sinceMidnight := now.Sub(midnight).Milliseconds()
	today := now.Weekday()
	yesterday := (today + 6) % 7
	for _, window := range windows {
		_, onToday := window.days[today]
		if window.start < window.end {
			if onToday && sinceMidnight >= window.start && sinceMidnight < window.end {
				return true
			}
			continue
		}
		if onToday && sinceMidnight >= window.start {
			return true
		}
		if _, onYesterday := window.days[yesterday]; onYesterday && sinceMidnight < window.end {
			return true
		}
	}
	return false
}

func describeWindows(windows []itemWindow) string {
	parts := make([]string, 0, len(windows))
	for _, window := range windows {
		days := []string{}
		for _, day := range []time.Weekday{
			time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday,
		} {
			if _, ok := window.days[day]; ok {
				days = append(days, day.String()[:3])
			}
		}
		parts = append(parts, fmt.Sprintf(
			"%s %s-%s",
			strings.Join(days, ","),
			clockLabel(window.start),
			clockLabel(window.end),
		))
	}
	return strings.Join(parts, "; ")
}

func clockLabel(millis int64) string {
	minutes := millis / 60000
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
