# Discovery and Search Commands

Shared/global flags and shared location override flags are documented in `cli-overview`.

## `wolt discover feed`

```console
wolt discover feed [--address "<text>" | --lat <float> --lon <float>] [--limit <n>] [global flags]
```

Options:
- `--limit`: cap returned feed sections/items
- `--wolt-plus`: include only Wolt+ venues (client-side filter on discovery payload)

Output schema:
- `DiscoveryFeed`

Notes:
- feed venue rows include `price_range`, `price_range_scale`, `promotions[]`, and `wolt_plus`
- location defaults to profile location; use `--address` or `--lat/--lon` for a temporary override

Examples:

```console
wolt discover feed --format json
wolt discover feed --profile work --format yaml
wolt discover feed --address "Kamppi, Helsinki" --limit 5 --format json
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
- `--query` optional free text query (omit to list venues near profile location)
- `--sort [recommended|distance|rating|delivery_price|delivery_time]`
- `--type [restaurant|grocery|pharmacy|retail]`
- `--category <slug>`
- `--open-now`
- `--wolt-plus`
- `--limit <n>`
- `--offset <n>`

Output schema:
- `VenueSearchResult`

Notes:
- venue rows include `price_range`, `price_range_scale`, and `promotions[]`
- location defaults to profile location; use global `--address` for a temporary override

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
- `--limit <n>`
- `--offset <n>`

Output schema:
- `ItemSearchResult`

Notes:
- location defaults to profile location; use global `--address` for a temporary override

Examples:

```console
wolt search items --query whopper --limit 10 --format json
wolt search items --address "Kamppi, Helsinki" --query whopper --limit 10 --format json
wolt search items --query noodles --category lunch --format yaml
```
