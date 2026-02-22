# Discovery and Search Commands

Shared/global flags and shared location override flags are documented in `cli-overview`.

## `wolt discover feed`

```console
wolt discover feed [--address "<text>" | --lat <float> --lon <float>] [--query <text>] [--sort <mode>] [--limit <n>] [--offset <n> | --page <n>] [--fast] [global flags]
```

Options:
- `--query`: client-side filter by venue name/slug
- `--sort [recommended|rating|delivery_fee|delivery_time|name]`
- `--min-rating <float>`
- `--max-delivery-fee <minor-units>`
- `--promotions-only`
- `--limit`: cap returned venues across all sections
- `--offset`: skip N venues before returning rows (global across sections)
- `--page`: 1-based page number (requires `--limit`, mutually exclusive with `--offset`)
- `--fast`: skip per-venue enrichment requests (fewer campaign discounts, lower chance of `429`)
- `--wolt-plus`: include only Wolt+ venues (client-side filter on discovery payload)

Output schema:
- `DiscoveryFeed`

Notes:
- feed venue rows include `slug`, `price_range`, `price_range_scale`, `promotions[]`, and `wolt_plus`
- payload includes pagination metadata: `total`, `count`, `offset`, optional `limit`, optional `next_offset`
- location defaults to selected Wolt account address; use `--address` or `--lat/--lon` for a temporary override
- HTTP request pacing is enabled by default; override via `WOLT_HTTP_MIN_INTERVAL_MS` (set `0` to disable)

Examples:

```console
wolt discover feed --format json
wolt discover feed --profile work --format yaml
wolt discover feed --address "Kamppi, Helsinki" --limit 5 --format json
wolt discover feed --query "burger king" --sort rating --limit 10 --page 1 --format json
wolt discover feed --limit 20 --offset 20 --format json
wolt discover feed --fast --limit 20 --format json
wolt discover feed --lat <lat> --lon <lon> --limit 5 --format json
```

## `wolt discover categories`

```console
wolt discover categories [--address "<text>" | --lat <float> --lon <float>] [global flags]
```

Output schema:
- `CategoryList`

Examples:

```console
wolt discover categories --format json
wolt discover categories --profile work --format yaml
wolt discover categories --address "Kamppi, Helsinki" --format json
wolt discover categories --lat <lat> --lon <lon> --format json
```

## `wolt search venues`

```console
wolt search venues [--query <text>] [options] [global flags]
```

Options:
- `--query` optional free text query (omit to list venues near selected Wolt account address)
- `--sort [recommended|distance|rating|delivery_price|delivery_time]`
- `--type [restaurant|grocery|pharmacy|retail]`
- `--category <slug>`
- `--open-now`
- `--wolt-plus`
- `--min-rating <float>`
- `--max-delivery-fee <minor-units>`
- `--promotions-only`
- `--limit <n>`
- `--offset <n>`
- `--page <n>` (requires `--limit`, mutually exclusive with `--offset`)

Output schema:
- `VenueSearchResult`

Notes:
- venue rows include `price_range`, `price_range_scale`, and `promotions[]`
- location defaults to selected Wolt account address; use global `--address` for a temporary override

Examples:

```console
wolt search venues --format json
wolt search venues --address "Kamppi, Helsinki" --query burger --limit 20 --format json
wolt search venues --query burger --sort rating --open-now --limit 20 --format json
wolt search venues --query sushi --wolt-plus --category asian --format yaml
```

## `wolt search items`

```console
wolt search items --query <text> [options] [global flags]
```

Options:
- `--query` free text query
- `--sort [relevance|price|name]`
- `--category <slug>`
- `--min-price <minor-units>`
- `--max-price <minor-units>`
- `--hide-sold-out`
- `--discounts-only`
- `--limit <n>`
- `--offset <n>`
- `--page <n>` (requires `--limit`, mutually exclusive with `--offset`)

Output schema:
- `ItemSearchResult`

Notes:
- location defaults to selected Wolt account address; use global `--address` for a temporary override
- for large marketplace venues, prefer venue-scoped search: `wolt venue search <slug> --query <text>`

Examples:

```console
wolt search items --query whopper --limit 10 --format json
wolt search items --address "Kamppi, Helsinki" --query whopper --limit 10 --format json
wolt search items --query noodles --category lunch --format yaml
```
