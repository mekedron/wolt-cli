# Cart and Checkout Commands

Included commands:
- `wolt cart count`
- `wolt cart show`
- `wolt cart add <venue-id> <item-id>`
- `wolt cart remove <item-id>`
- `wolt cart clear`
- `wolt checkout preview`

Shared/global flags and shared location override flags are documented in `cli-overview`.

## `wolt cart count`

```console
wolt cart count [global flags]
```

Behavior:
- calls `GET https://consumer-api.wolt.com/order-xp/v1/baskets/count`

Output:
- `count`

## `wolt cart show`

```console
wolt cart show [--venue-id <id>] [--address "<text>" | --lat <value> --lon <value>] [--details] [global flags]
```

Behavior:
- calls `GET https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets?lat=...&lon=...`
- if multiple baskets exist and `--venue-id` is omitted, first basket is selected by default
- `--details` expands line options in table output (names when available, otherwise IDs)
- location defaults to selected Wolt account address; use `--address` or `--lat/--lon` for a temporary override

Output schema:
- `CartState`

## `wolt cart add <venue-id> <item-id>`

```console
wolt cart add <venue-id> <item-id> [--count <n>] [--option <group-id=value-id[:count]>...] [--allow-substitutions] [--venue-slug <slug>] [global flags]
```

Options:
- `--count` (default `1`)
- `--option` repeatable `group-id=value-id` or `group-id=value-id:count` (IDs or exact names)
- `--allow-substitutions`
- `--name` optional item name override
- `--price` optional item price override in minor units
- `--currency` optional basket currency override
- `--venue-slug` optional slug for assortment/venue-content metadata enrichment

Behavior:
- tries item endpoint for name/price/options
- falls back to assortment metadata when item endpoint is missing or incomplete
- if assortment is empty/partial upstream, falls back to venue-content metadata for item resolution
- venue-content fallback uses auth from profile/global flags when available
- sends add request to `POST https://consumer-api.wolt.com/order-xp/v1/baskets`
- refreshes totals from basket/count endpoints

Output:
- `basket_id`
- `venue_id`
- `mutation` (`add`)
- `line_id`
- `total_items`
- `total`

## `wolt cart remove <item-id>`

```console
wolt cart remove <item-id> [--count <n>] [--all] [--venue-id <id>] [--address "<text>" | --lat <value> --lon <value>] [global flags]
```

Behavior:
- loads baskets
- selects basket by `--venue-id` or first available basket
- decrements quantity via `POST /order-xp/v1/baskets`
- clears full basket via `POST /order-xp/v1/baskets/bulk/delete` when needed
- location defaults to selected Wolt account address; use `--address` or `--lat/--lon` for a temporary override

Output:
- `basket_id`
- `venue_id`
- `mutation` (`remove` or `clear`)
- `line_id`
- `removed_count`
- `total_items`
- `total`

## `wolt cart clear`

```console
wolt cart clear [--venue-id <id>] [--all] [--address "<text>" | --lat <value> --lon <value>] [global flags]
```

Behavior:
- loads baskets
- clears selected basket (or all baskets with `--all`) using `POST /order-xp/v1/baskets/bulk/delete`
- location defaults to selected Wolt account address; use `--address` or `--lat/--lon` for a temporary override

Output:
- `mutation` (`clear`)
- `basket_ids`
- `cleared_baskets`
- `total_items`
- `total`

## `wolt checkout preview`

```console
wolt checkout preview [--delivery-mode <standard|priority|schedule>] [--tip <minor-units>] [--promo-code <id>] [--venue-id <id>] [--address "<text>" | --lat <value> --lon <value>] [global flags]
```

Behavior:
- reads current baskets
- selects basket by `--venue-id` or first available basket
- builds `purchase_plan` payload with assortment/item fallback data for category/options
- calls `POST https://consumer-api.wolt.com/order-xp/web/v2/pages/checkout`
- returns projected totals without placing an order
- location overrides (`--address` / `--lat` / `--lon`) affect preview only
- actual order placement in Wolt uses the delivery address selected in your Wolt account

Output schema:
- `CheckoutPreview`
