# Discovery and Search Commands

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

## `wolt discover feed`

Synopsis:

```console
wolt discover feed [--lat <float> --lon <float>] [--limit <n>] [global flags]
```

Arguments:
- none

Options:
- `--lat`: latitude override (optional when profile location is configured)
- `--lon`: longitude override (optional when profile location is configured)
- `--limit`: cap returned feed sections/items

Location resolution:
- if both `--lat` and `--lon` are omitted, use the selected profile location
- if `--profile` is omitted, use the default profile
- if only one of `--lat` or `--lon` is provided, return `WOLT_INVALID_ARGUMENT`

Output schema:
- `DiscoveryFeed`

Examples:

```console
wolt discover feed --format json
wolt discover feed --profile work --format yaml
wolt discover feed --lat 60.1484 --lon 24.6913 --limit 5 --format json
wolt discover feed --lat 60.1484 --lon 24.6913 --format yaml
```

## `wolt discover categories`

Synopsis:

```console
wolt discover categories [--lat <float> --lon <float>] [global flags]
```

Arguments:
- none

Options:
- `--lat`: latitude override (optional when profile location is configured)
- `--lon`: longitude override (optional when profile location is configured)

Location resolution:
- if both `--lat` and `--lon` are omitted, use the selected profile location
- if `--profile` is omitted, use the default profile
- if only one of `--lat` or `--lon` is provided, return `WOLT_INVALID_ARGUMENT`

Output schema:
- `CategoryList`

Examples:

```console
wolt discover categories --format json
wolt discover categories --profile work --format yaml
wolt discover categories --lat 60.1484 --lon 24.6913 --format json
wolt discover categories --lat 60.1484 --lon 24.6913 --format yaml
```

## `wolt search venues`

Synopsis:

```console
wolt search venues [--query <text>] [options] [global flags]
```

Arguments:
- none

Options:
- `--query`: free text query (optional; omit to list venues near profile location)
- `--sort [recommended|distance|rating|delivery_price|delivery_time]`
- `--type [restaurant|grocery|pharmacy|retail]`
- `--category <slug>`
- `--open-now`
- `--wolt-plus`
- `--limit <n>`
- `--offset <n>`

Output schema:
- `VenueSearchResult`

Examples:

```console
wolt search venues --format json
wolt search venues --query burger --sort rating --open-now --limit 20 --format json
wolt search venues --query sushi --wolt-plus --category asian --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_search_venues_001",
    "generated_at": "2026-02-19T21:10:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "query": "burger",
    "total": 4,
    "items": [
      {
        "venue_id": "629f1f18480882d6f02c25f0",
        "slug": "burger-king-finnoo",
        "name": "Burger King Finnoo",
        "address": "Finnoonristi 1",
        "rating": 8.6,
        "delivery_estimate": "25-35 min",
        "delivery_fee": {
          "amount": 449,
          "formatted_amount": "€4.49"
        },
        "wolt_plus": true
      }
    ]
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_search_venues_001
  generated_at: "2026-02-19T21:10:00Z"
  profile: default
  locale: en-FI
data:
  query: burger
  total: 4
  items:
    - venue_id: "629f1f18480882d6f02c25f0"
      slug: burger-king-finnoo
      name: Burger King Finnoo
      address: Finnoonristi 1
      rating: 8.6
      delivery_estimate: 25-35 min
      delivery_fee:
        amount: 449
        formatted_amount: "€4.49"
      wolt_plus: true
warnings: []
```

## `wolt search items`

Synopsis:

```console
wolt search items --query <text> [options] [global flags]
```

Arguments:
- none

Options:
- `--query`: free text query
- `--sort [relevance|price|name]`
- `--category <slug>`
- `--limit <n>`
- `--offset <n>`

Output schema:
- `ItemSearchResult`

Examples:

```console
wolt search items --query whopper --limit 10 --format json
wolt search items --query noodles --category lunch --format yaml
```

## Integration Notes (Current Implementation)

Observed search request shape from web flow:

```json
{
  "q": "burger",
  "target": null,
  "lat": 60.148411511559424,
  "lon": 24.691323861479756
}
```

Current `wolt discover feed`, `wolt discover categories`, `wolt search venues`,
and `wolt search items` all follow the same profile-scoped location behavior,
with optional `--lat/--lon` overrides where supported.
