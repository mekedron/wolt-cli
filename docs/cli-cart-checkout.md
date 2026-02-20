# Cart and Checkout Commands

Included commands:
- `wolt cart count`
- `wolt cart show`
- `wolt cart add <venue-id> <item-id>`
- `wolt cart remove <item-id>`
- `wolt cart clear`
- `wolt checkout preview`

All commands in this document support:
- global flags:
  - `--format [table|json|yaml]`
  - `--profile <name>`
  - `--locale <bcp47>`
  - `--no-color`
  - `--output <path>`
  - `--verbose`
  - `--wrtoken <token>`
- auth via one of:
  - `--wtoken <token>`
  - `--wrtoken <token>`
  - `--cookie <name=value>`
  - `profile.wtoken`, `profile.wrefresh_token`, or `profile.cookies` from selected/default local profile

## `wolt cart count`

Synopsis:

```console
wolt cart count [global flags]
```

Behavior:
- Calls `GET https://consumer-api.wolt.com/order-xp/v1/baskets/count`

Output:
- `count` (int)

## `wolt cart show`

Synopsis:

```console
wolt cart show [--venue-id <id>] [--lat <value> --lon <value>] [global flags]
```

Behavior:
- Calls `GET https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets?lat=...&lon=...`
- When multiple baskets exist and `--venue-id` is omitted, selects the first basket and returns a warning in machine formats.
- `--details` expands table output with selected option/value details for each line item (for example what is included in a combo/set).
  - when upstream does not expose option names, IDs are shown instead.
- Normalizes response into `CartState` shape with:
  - `basket_id`
  - `venue_id`
  - `venue_name`
  - `venue_slug`
  - `selection` (`selection_mode`, `basket_count`, selected basket details)
  - `currency` (best-effort from formatted totals)
  - `lines[]`
  - `subtotal`
  - `fees` (empty list in current implementation)
  - `total`

## `wolt cart add <venue-id> <item-id>`

Synopsis:

```console
wolt cart add <venue-id> <item-id> [--count <n>] [--option <group-id=value-id[:count]>...] [--allow-substitutions] [global flags]
```

Options:
- `--count` default `1`
- `--option` repeatable `group-id=value-id` or `group-id=value-id:count` (group/value can be IDs or exact names)
- `--allow-substitutions`
- `--name` optional item name override
- `--price` optional item price override in minor units
- `--currency` optional basket currency override

Behavior:
- Fetches item detail from `GET https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/<venue_id>/item/<item_id>` to infer name/price/options
- Sends add request to `POST https://consumer-api.wolt.com/order-xp/v1/baskets`
- Refreshes totals from basket/count endpoints

Output:
- `basket_id`
- `venue_id`
- `mutation` (`add`)
- `line_id`
- `total_items`
- `total`

## `wolt checkout preview`

Synopsis:

```console
wolt checkout preview [--delivery-mode <standard|priority|schedule>] [--tip <minor-units>] [--promo-code <id>] [--venue-id <id>] [global flags]
```

Behavior:
- Reads current basket (`/order-xp/web/v1/pages/baskets`)
- Uses the same basket selection rules as `cart show` (`--venue-id` preferred, otherwise first available).
- Builds a `purchase_plan` payload
- Calls `POST https://consumer-api.wolt.com/order-xp/web/v2/pages/checkout`
- Returns projected totals/rows without placing an order

Output schema:
- `basket_id`
- `venue_id`
- `venue_name`
- `venue_slug`
- `selection`
- `payable_amount`
- `checkout_rows`
- `delivery_configs`
- `offers`
- `tip_config`

## `wolt cart remove <item-id>`

Synopsis:

```console
wolt cart remove <item-id> [--count <n>] [--all] [--venue-id <id>] [--lat <value> --lon <value>] [global flags]
```

Behavior:
- Loads baskets from `GET https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets?lat=...&lon=...`
- Resolves selected basket the same way as `cart show` (`--venue-id` preferred, otherwise first basket)
- For quantity decrement (`count` remains above zero), sends mutation to `POST https://consumer-api.wolt.com/order-xp/v1/baskets`
- For full basket clear fallback (single-line basket), sends `POST https://consumer-api.wolt.com/order-xp/v1/baskets/bulk/delete`

Output:
- `basket_id`
- `venue_id`
- `mutation` (`remove` or `clear`)
- `line_id`
- `removed_count`
- `total_items`
- `total`

Notes:
- Removing an entire line from multi-line baskets is not supported in this command yet.
- Use `wolt cart clear` to clear the full selected basket.

## `wolt cart clear`

Synopsis:

```console
wolt cart clear [--venue-id <id>] [--all] [--lat <value> --lon <value>] [global flags]
```

Behavior:
- Loads baskets from `GET https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets?lat=...&lon=...`
- Clears selected basket (or all baskets with `--all`) using `POST https://consumer-api.wolt.com/order-xp/v1/baskets/bulk/delete`

Output:
- `mutation` (`clear`)
- `basket_ids`
- `cleared_baskets`
- `total_items`
- `total`
