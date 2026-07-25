# Output contract

Stable shape for machine-readable responses (`--format json` / `--format yaml`).
Table output is shown by default and is not part of this contract.

## Envelope

```json
{
  "meta": {
    "request_id": "req_01j0zdq8q6k7y8d6w2g0y9p4m7",
    "generated_at": "2026-02-19T20:45:09Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {},
  "warnings": []
}
```

YAML uses the same fields. `warnings[]` carries non-fatal upstream issues
(fallback used, address inferred from account, etc.). `data` can be
`null` on errors.

### Errors

```json
{
  "meta": { },
  "data": null,
  "warnings": [],
  "error": {
    "code": "WOLT_AUTH_REQUIRED",
    "message": "Authentication is required. Run \"wolt login\" first.",
    "details": { }
  }
}
```

Stable error codes:

| Code | Meaning |
|---|---|
| `WOLT_AUTH_REQUIRED` | The endpoint requires a logged-in session. |
| `WOLT_INVALID_ARGUMENT` | Flag combination is invalid (e.g. `--lat` without `--lon`). |
| `WOLT_UPSTREAM_ERROR` | Upstream returned a non-success status. `--verbose` adds the URL and status code to the message. |
| `WOLT_STATS_BUNDLE_UNAVAILABLE` | `wolt stats` could not fetch a usable dashboard bundle (GitHub unreachable, missing release asset, or checksum mismatch) and no cached bundle is on disk. |
| `WOLT_STATS_ENV_ERROR` | `wolt stats` environment issue: port busy, stats dir not writable, missing user email, sync failure. |

## Conventions

- IDs are strings (`venue_id`, `item_id`, `basket_id`, `purchase_id`).
- Money: `amount` in minor units (cents) plus an optional `formatted_amount` for display.
- Time: ISO-8601 UTC for CLI-generated stamps; upstream-provided strings are passed through verbatim alongside parsed equivalents when available.
- Booleans are never serialised as strings.

## Schemas

### `wolt status` — Status

```
authenticated: bool
user_id: string
country: string
session_expires_at: string|null  (ISO-8601 UTC)
wolt_plus_subscriber: bool
token_preview?: string           (--verbose only)
cookie_count?: int               (--verbose only)
```

### `wolt account` — ProfileSummary

```
user_id: string
name: string
email_masked: string
phone_masked: string
country: string
```

### `wolt account addresses` — AddressList

```
addresses[]: { address_id, label, street, is_default }
profile_default_address_id: string
```

### `wolt account addresses links` — AddressLinks

```
address_id: string
links: { address_link, entrance_link, coordinates_link }
```

### `wolt account payments` — PaymentMethodList

```
methods[]: { method_id, type, label, is_default, is_available_for_checkout }
```

### `wolt account orders` — OrderHistoryList

```
orders[]: {
  purchase_id, received_at, status, venue_name,
  total_amount, is_active, items_summary,
  payment_time_ts, main_image, main_image_blurhash
}
count: int
next_page_token?: string
status_filter?: string
```

### `wolt account order <id>` — OrderHistoryDetail

```
order_id, status, currency
venue: { id, name, address, phone, country, product_line }
totals: { items, delivery, service_fee, subtotal, credits, tokens, total }
  (each value is { amount, formatted_amount })
items[]: { id, name, count, price, line_total, options }
payments[]: { name, amount, method_type, method_id, provider, payment_time }
delivery: { alias, address, city, comment }

# Optional
order_number, creation_time, delivery_time, delivery_method
discounts[]: { title, amount }
surcharges[]: { title, amount }
```

### `wolt venues` — VenueSearchResult

```
query?: string
total: int
items[]: {
  venue_id, slug, name, address,
  tagline, top_offer,
  rating, delivery_estimate, delivery_fee,
  price_range, price_range_scale,
  promotions[], badges[], menu_highlights[], wolt_plus
}

# Pagination
count, offset, limit, total_pages, next_offset, page
```

`tagline` is the venue's marketing one-liner (`short_description` /
`short_description_v2.value` from upstream). `top_offer` is the most
prominent promo text — preference order: discount-variant promos, then
any promo (excluding Wolt+ membership labels). Both come from the same
front-page payload, no extra HTTP. With `--enrich`, `promotions` is
backfilled with dynamic campaign banners.

`badges[]: { icon, variant, text }` is sourced from upstream
`badges_v2` and surfaces newer icon-bearing badges (`wolt-plus`,
`coupon-fill`, `bike`, …). Empty array when upstream omits the field.
The table renderer prefixes the venue cell with single-rune glyphs
derived from `icon`; set `WOLT_BADGES_PLAIN=1` to fall back to
bracketed text (e.g. `[Wolt+]`).

`menu_highlights[]: { name, formatted_price }` is sourced from
upstream `venue_preview_items` and lists flagship dishes for sponsored
/ featured rows. Empty array when upstream omits the field. The
`venues` table hides this column by default — pass `--show-highlights`
to surface it.

### `wolt feed` — DiscoveryFeed

```
city: string
wolt_plus_only: bool
sections[]: {
  name: string             # internal name, e.g. "popular-restaurants"
  title: string            # display title, e.g. "Popular near you"
  kind: "venues" | "brands"
  items[]: {                                          # kind = "venues"
    venue_id, slug, name,
    tagline, top_offer,
    rating, delivery_estimate, delivery_fee,
    price_range, price_range_scale,
    promotions[], badges[], menu_highlights[], wolt_plus
  }
  brands[]: { name, slug }                            # kind = "brands"
}
```

Venue sections have the same row shape as `venues`, including the
additive `badges[]` and `menu_highlights[]` fields described above.
Sections preserve the upstream ordering you see on wolt.com. The
`feed` table renders the Highlights column by default (pass
`--show-highlights=false` to hide).

`kind = "brands"` covers carousels whose entries lack a venue block —
brand-curated lists ("Popular stores"), restaurant-category tiles, or
hero banners. `items[]` is always present and empty for that kind;
`brands[].slug` is the upstream link target (often a curated list ID
like `woltmarket-popular-brands:helsinki`). The table renders these as
a single one-line summary; `--query` matches against `brands[].name`
as well as venue rows.

### `wolt top` — TopVenues

```
venues[]: {
  venue_id, slug, name,
  tagline, top_offer,
  rating, delivery_estimate, delivery_fee,
  price_range, price_range_scale,
  promotions[], badges[], menu_highlights[], wolt_plus
}

# Pagination
count, offset, limit, total_pages, next_offset, page
```

Same row shape as `wolt venues`. The flattened slice is bounded by the
combined `feed` payload and the `--limit` (default 10) before
deduplication by `venue_id`. Brand carousels are excluded.

### `wolt venues categories` — CategoryList

```
categories[]: { id, name, slug }

# Pagination
count, total, offset, limit, total_pages, next_offset, page
```

### `wolt venue <slug>` — VenueDetail

```
venue_id, slug, name, address, currency
rating, delivery_methods, order_minimum
```

If the restaurant detail endpoint is unavailable, the CLI falls back to
the static venue payload and adds a warning.

### `wolt venue categories <slug>` — VenueCategoryList

```
venue_id: string
loading_strategy: string
categories[]: { id, slug, name, parent_slug, level, leaf, item_refs_count }

# Pagination
count, total, offset, limit, total_pages, next_offset, page
```

### `wolt venue menu <slug>` (without `--query`) — VenueMenu

```
venue_id, wolt_plus
categories[]
items[]: {
  item_id, name, base_price, discounts,
  is_available, unavailable_reason, purchasable_balance,
  image_url, image_urls
}

# Optional
items[].original_price          (campaign-adjusted)
items[].option_group_ids        (--include-options)
count, offset, limit, total_pages, next_offset, page, sort
```

### `wolt venue menu <slug> --query <text>` — VenueItemSearchResult

```
venue_id, venue_slug, query, total
items[]: {
  item_id, name, category, base_price, discounts,
  is_available, unavailable_reason, purchasable_balance,
  image_url, image_urls
}

# Optional
items[].original_price          (upstream pre-discount amount)
items[].option_group_ids        (--include-options)
count, offset, limit, total_pages, next_offset, page, sort
```

When upstream omits the currency, it is normalized from venue metadata.
When `original_price` is present without a promo label, the CLI derives
a synthetic discount (e.g. `21% off`).

Availability is derived from Wolt's current `disabled_info` and
`purchasable_balance` fields. `is_sold_out` remains as a deprecated,
derived compatibility alias (`!is_available`); raw `is_sold_out` and
`sold_out` fields are not expected from Wolt.

### `wolt venue hours <slug>` — VenueHours

```
venue_id: string
timezone: string
opening_windows[]
```

### `wolt venue item <venue> <item-id>` — ItemDetail

```
item_id, venue_id, name, description, price
is_available, unavailable_reason, purchasable_balance
image_url, image_urls, image_blurhash
option_groups[]
upsell_items[]
```

`price.formatted_amount` is normalized from venue metadata when upstream
omits the currency. `upsell_items[].price` follows the same rule.
The exact current-item response is merged with the item page so current image
and availability metadata are preserved alongside option groups.

### `wolt cart` — CartState

```
basket_id, venue_id, venue_name, venue_slug
selection, currency, total_items
lines[]: {
  line_id, item_id, name, count,
  options[], price, line_total
}
subtotal, fees, total
```

### `wolt cart add|remove|clear` — CartMutationResult

```
mutation: "add" | "remove" | "clear"
total_items: int
total: { amount, formatted_amount }

# Per mutation
add:    basket_id, venue_id, line_id, item_name, item_price, item_currency
remove: basket_id, venue_id, line_id, removed_count
clear:  basket_ids[], cleared_baskets
```

Before `cart add`, the CLI always fetches the exact current item and refuses
the mutation when it is disabled, has zero purchasable balance, is missing
from the current assortment, or cannot be validated. Explicit `--price`,
`--currency`, and `--name` values do not bypass this check.

`total.formatted_amount` is `null` for `clear`, where the emptied basket has no
venue currency left to format against. It is never formatted with a guessed
currency.

### `wolt checkout` — CheckoutPreview

```
basket_id, venue_id, venue_name, venue_slug
selection
payable_amount
checkout_rows[]
delivery_configs[]
offers
tip_config
```

Preview-only. The CLI never calls the order placement endpoint.
Every basket item is revalidated through the exact current-item endpoint
before preview. Currency comes from structured item, basket, or venue metadata
(including GEL); there is no implicit EUR fallback.

### `wolt stats` — StatsLaunch

The envelope is emitted on startup, **before** the command blocks on the
HTTP server. JSON / YAML consumers should expect a single envelope followed
by the process continuing to run; the server stays up until SIGINT (exit
code `130`).

```
stats_dir: string                        absolute path of the install dir
bundle: {
  version: string                         tag name, e.g. "v0.1.0"
  source: "github-release" | ""           where the bundle came from
  downloaded: bool                        true if this run pulled it fresh
  active_path: string                     absolute path to the extracted bundle
}
db_path: string                          absolute path to wolt-history.sqlite
email: string                            (omitted when --no-sync)
sync: {                                  (omitted when --no-sync)
  performed: bool
  mode: "full" | "incremental"
  pages_fetched, orders_scanned, details_fetched: int
  inserted_orders, updated_orders: int
  catalog_count, detail_count: int
  reached_history_end: bool
  duration_ms: int64
  stop_reason: "known_purchase" | "checkpoint_reached"   (incremental only, omitted on full scan)
}
server: {
  url: "http://127.0.0.1:5173"
  port: int
  pid: int
}
```
