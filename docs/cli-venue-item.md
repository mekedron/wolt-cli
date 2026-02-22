# Venue and Item Commands

Shared/global flags are documented in `cli-overview`.

## `wolt venue show <slug>`

```console
wolt venue show <slug> [--include hours,tags,rating,fees] [--address "<text>"] [global flags]
```

Options:
- `--include`: comma-separated optional sections
- `--address`: temporary location override for slug lookup

Output schema:
- `VenueDetail`

Notes:
- if the restaurant detail endpoint is unavailable for a venue, CLI falls back to static venue payload and returns basic venue fields with warnings.

## `wolt venue categories <slug>`

```console
wolt venue categories <slug> [global flags]
```

Behavior:
- loads venue assortment topology in one request
- returns available category tree rows (`slug`, `name`, `parent_slug`, `level`, `leaf`, `item_refs_count`)
- intended as the first step for partial-market assortments (for example many Wolt Market venues)

Output schema:
- `VenueCategoryList`

## `wolt venue search <slug>`

```console
wolt venue search <slug> --query <text> [--category <slug>] [--include-options] [--sort <mode>] [--min-price <n>] [--max-price <n>] [--hide-sold-out] [--discounts-only] [--limit <n>] [--offset <n> | --page <n>] [global flags]
```

Options:
- `--query`: item search query (required)
- `--category`: optional category filter over matched items
- `--include-options`: include option-group IDs per item
- `--sort [recommended|price|name]`
- `--min-price` / `--max-price`: base price filter in minor units
- `--hide-sold-out`: exclude sold-out items
- `--discounts-only`: include only discounted items
- `--limit`: cap number of returned rows
- `--offset`: skip N matched rows
- `--page`: 1-based page number (requires `--limit`, cannot be combined with `--offset`)

Behavior:
- calls venue-scoped assortment item search endpoint
- returns matched items only for the provided venue slug
- recommended for large marketplace-style venues with very large catalogs

Output schema:
- `VenueItemSearchResult`

## `wolt venue menu <slug>`

```console
wolt venue menu <slug> [--category <slug>] [--full-catalog] [--include-options] [--sort <mode>] [--min-price <n>] [--max-price <n>] [--hide-sold-out] [--discounts-only] [--limit <n>] [--offset <n> | --page <n>] [global flags]
```

Options:
- `--category`: restrict to one category
- `--full-catalog`: force cross-category crawl for partial assortments (can be slow)
- `--include-options`: include option-group IDs per item
- `--sort [recommended|price|name]`
- `--min-price` / `--max-price`: base price filter in minor units
- `--hide-sold-out`: exclude sold-out items
- `--discounts-only`: include only discounted items
- `--limit`: cap number of returned items
- `--offset`: skip N items
- `--page`: 1-based page number (requires `--limit`, cannot be combined with `--offset`)

Behavior:
- loads venue metadata from static venue page endpoint
- loads dynamic venue page payload to resolve campaign-level item discounts
- loads menu/option topology from assortment endpoint
- when `--category` is provided, fetches only that category payload and hydrates its items
- for partial assortments without `--category`, returns `WOLT_INVALID_ARGUMENT` and guidance to use `venue categories` + `--category`, or `venue search`
- `--full-catalog` keeps legacy full cross-category crawl for partial assortments
- when assortment is empty for non-partial venues, falls back to venue-content endpoint
- does not require discovery catalog lookup
- when auth tokens/cookies are available in profile or flags, they are forwarded to improve venue-content coverage

Output schema:
- `VenueMenu`

Notes:
- venue payload includes `wolt_plus` participation flag
- menu/search items include `discounts[]` from upstream promotion metadata and dynamic campaign payloads when available
- when dynamic item campaigns include percentage discounts, `base_price` is adjusted to discounted value and `original_price` is populated
- for marketplace payloads that expose `original_price` without promo labels, CLI derives a synthetic discount label (for example `21% off`)

## `wolt venue hours <slug>`

```console
wolt venue hours <slug> [--timezone <iana>] [--address "<text>"] [global flags]
```

Options:
- `--timezone`: output timezone (for example `Europe/Helsinki`)
- `--address`: temporary location override for slug lookup

Output schema:
- `VenueHours`

Notes:
- if the restaurant detail endpoint is unavailable, CLI returns fallback hours payload with empty opening windows and a warning.

## `wolt item show <venue-slug> <item-id>`

```console
wolt item show <venue-slug> <item-id> [--include-upsell] [global flags]
```

Options:
- `--include-upsell`: include upsell items when available

Behavior:
- resolves venue by slug
- loads item payload from item endpoint
- merges assortment fallback when item endpoint payload is incomplete
- falls back to venue-content payload when assortment does not expose item-level data
- returns an error if the provided item is not found in the venue menu

Output schema:
- `ItemDetail`

## `wolt item options <venue-slug> <item-id>`

```console
wolt item options <venue-slug> <item-id> [global flags]
```

Behavior:
- resolves option groups from item payload, assortment payload, and venue-content fallback payloads
- returns ready-to-use `--option group-id=value-id` examples for `wolt cart add`
- returns an error if item does not belong to the venue

Output fields:
- `item_id`
- `venue_id`
- `currency`
- `group_count`
- `option_groups[]`

`option_groups[]` fields:
- `group_id`
- `name`
- `required`
- `min`
- `max`
- `values[]` with `value_id`, `name`, `price`, `example_option`

## Recommended Flow

```console
wolt venue categories <slug> --format json
wolt venue search <slug> --query "<text>" --format json
wolt venue menu <slug> --category <category-slug> --include-options --format json
wolt item options <slug> <item-id> --format json
wolt cart add <venue-id> <item-id> --option "<group-id>=<value-id>" --format json
```
