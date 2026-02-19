# Discovery and Search Commands

All commands in this document support:
- `--format json`
- `--format yaml`

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

## wolt discover feed

Synopsis:

```console
wolt discover feed --lat <float> --lon <float> [--limit <n>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--lat`: latitude
- `--lon`: longitude
- `--limit`: cap returned feed sections/items

Output schema:
- `DiscoveryFeed`

Examples:

```console
wolt discover feed --lat 60.1484 --lon 24.6913 --limit 5 --format json
wolt discover feed --lat 60.1484 --lon 24.6913 --format yaml
```

## wolt discover categories

Synopsis:

```console
wolt discover categories --lat <float> --lon <float> [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--lat`: latitude
- `--lon`: longitude

Output schema:
- `CategoryList`

Examples:

```console
wolt discover categories --lat 60.1484 --lon 24.6913 --format json
wolt discover categories --lat 60.1484 --lon 24.6913 --format yaml
```

## wolt search venues

Synopsis:

```console
wolt search venues --query <text> [options] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--query`: free text query
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

## wolt search items

Synopsis:

```console
wolt search items --query <text> [options] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
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

Current `wolt-cli ls` already implements a subset of this surface:
- profile-scoped location
- query/tag filtering
- sorting and limiting

Future `wolt search venues` should preserve that behavior while adding explicit Wolt filters.
