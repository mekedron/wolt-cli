package wolt

import (
	"context"
	"fmt"
	"strings"
	"time"

	// Venue timezones are resolved by IANA name; Windows hosts ship no zoneinfo.
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
// available_times exclude the current venue-local time. The venue timezone
// is fetched only when some item is restricted at all; when it cannot be
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
	clock := c.now
	if clock == nil {
		clock = time.Now
	}
	now := clock().In(location)
	for _, entry := range restricted {
		if windowsContain(entry.windows, now) {
			continue
		}
		entry.item["disabled_info"] = map[string]any{
			"disable_text": "not sold now; available " + describeWindows(entry.windows) + " " + location.String(),
			"source":       itemScheduleSource,
		}
	}
}

// venueLocation resolves the venue timezone from its static page and caches
// it for the client's lifetime.
func (c *Client) venueLocation(ctx context.Context, slug string) (*time.Location, bool) {
	key := strings.ToLower(strings.TrimSpace(slug))
	if key == "" {
		return nil, false
	}
	if cached, ok := c.venueLocations.Load(key); ok {
		return cached.(*time.Location), true
	}
	page, err := c.VenuePageStatic(ctx, slug)
	if err != nil {
		return nil, false
	}
	venue, _ := page["venue"].(map[string]any)
	name, _ := venue["timezone"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	c.venueLocations.Store(key, location)
	return location, true
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
