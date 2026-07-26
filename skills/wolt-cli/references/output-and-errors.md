# Output and Errors

## Machine-Readable Envelope

Use `--format json` for automation. Every response follows:

```json
{
  "meta": {
    "request_id": "req_xxx",
    "generated_at": "2026-02-19T20:45:09Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {},
  "warnings": [],
  "error": {
    "code": "WOLT_AUTH_REQUIRED",
    "message": "...",
    "details": {}
  }
}
```

`error` is omitted on success.

## Parsing Guidelines

- Read primary payload from `.data`.
- Always inspect `.warnings` and surface important warnings.
- On failure, present `.error.code` and `.error.message`.
- Keep `meta.request_id` for troubleshooting/log correlation.

## Common Error Codes

- `WOLT_AUTH_REQUIRED`: missing credentials or a session still rejected after any available automatic refresh
- `WOLT_FORBIDDEN`: HTTP 403; the account or operation is not allowed
- `WOLT_INVALID_ARGUMENT`: invalid flag combinations or required args missing
- `WOLT_PROFILE_ERROR`: profile load/select/write failure
- `WOLT_LOCATION_RESOLVE_ERROR`: address geocoding failure
- `WOLT_RATE_LIMITED`: upstream rate limit; retry later
- `WOLT_UPSTREAM_TEMPORARY`: network/retryable upstream failure
- `WOLT_UPSTREAM_INVALID_RESPONSE`: malformed or invalid success response
- `WOLT_UNSUPPORTED_ENDPOINT`: removed or unsupported upstream operation
- `WOLT_CLIENT_OUTDATED`: Wolt explicitly rejected the configured client version
- `WOLT_UPSTREAM_ERROR`: other upstream HTTP/API failure (`--verbose` adds details without changing the code)
- `WOLT_EMPTY_CART`: checkout/cart mutation attempted without basket items
- `WOLT_BASKET_UNRESOLVED`: basket IDs are incomplete; delete is blocked
- `WOLT_ITEM_NOT_FOUND`: item not found in selected basket/venue
- `WOLT_ITEM_AVAILABILITY_UNKNOWN`: current availability could not be verified
- `WOLT_ITEM_UNAVAILABLE`: current item is missing, disabled, or out of stock
- `WOLT_CART_ITEMS_UNAVAILABLE`: checkout basket contains unavailable items
- `WOLT_CURRENCY_UNKNOWN`: mutation currency could not be verified
- `WOLT_VENUE_UNRESOLVED`: canonical venue identity could not be resolved
- `WOLT_VENUE_CONFLICT`: resolved venue conflicts with the existing basket
- `WOLT_CHECKOUT_PAYLOAD_ERROR`: failed to build checkout preview payload
- `WOLT_DELIVERY_MODE_UNAVAILABLE`: Wolt did not confirm the requested delivery mode
- `WOLT_NOT_FOUND`: requested address/entity missing

## Discovery enrichment fields

Venue rows in `feed`, `top`, and `venues` carry additive fields on top of the legacy `tagline`/`top_offer`:

- `badges[]: { icon, variant, text }` — from upstream `badges_v2`. Empty array when upstream omits the field. The table renderer prefixes the venue cell with a single-rune glyph (`+` for Wolt+, `%` for discounts, `⚡` for fast delivery). Set `WOLT_BADGES_PLAIN=1` for bracketed-text fallback.
- `menu_highlights[]: { name, formatted_price }` — from upstream `venue_preview_items`. Empty array when upstream omits the field. The Highlights table column auto-renders when at least one row has data; pass `--show-highlights` / `--show-highlights=false` to force.
- `order_now_available`, `scheduled_order_available`, `scheduled_pickup_available`, `scheduled_only`, `delivers_to_location`, `store_open_now`, `next_opening_at`, `status_text`, `telemetry_status` — location-aware discovery signals. `null` means Wolt did not provide the signal, not `false`; `store_open_now` remains null because the discovery `online` field only proves order-now availability.

Feed sections carry `kind: "venues" | "brands"`. Brand sections carry `brands[]: { name, slug }` instead of venue items; the table renders them as a single-line summary, and `--query` matches against `brands[].name`.

## Diagnostics

Rerun with `--verbose` when debugging:

- enables HTTP trace output to stderr
- preserves machine envelope in stdout
- returns richer upstream error details

## Exit Codes

- `0`: success
- `1`: command/domain/upstream error
- `2`: unknown command
