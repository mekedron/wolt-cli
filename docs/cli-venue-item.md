# Venue and Item Commands

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

## `wolt venue show <slug>`

Synopsis:

```console
wolt venue show <slug> [--include hours,tags,rating,fees] [global flags]
```

Arguments:
- `<slug>`: venue slug (for example `burger-king-finnoo`)

Options:
- `--include`: comma-separated optional sections

Output schema:
- `VenueDetail`

Examples:

```console
wolt venue show burger-king-finnoo --include hours,tags,rating,fees --format json
wolt venue show burger-king-finnoo --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_venue_show_001",
    "generated_at": "2026-02-19T21:15:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "venue_id": "629f1f18480882d6f02c25f0",
    "slug": "burger-king-finnoo",
    "name": "Burger King Finnoo",
    "address": "Finnoonristi 1",
    "currency": "EUR",
    "rating": 8.6,
    "delivery_methods": [
      "homedelivery"
    ],
    "order_minimum": {
      "amount": 1000,
      "formatted_amount": "€10.00"
    }
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_venue_show_001
  generated_at: "2026-02-19T21:15:00Z"
  profile: default
  locale: en-FI
data:
  venue_id: "629f1f18480882d6f02c25f0"
  slug: burger-king-finnoo
  name: Burger King Finnoo
  address: Finnoonristi 1
  currency: EUR
  rating: 8.6
  delivery_methods:
    - homedelivery
  order_minimum:
    amount: 1000
    formatted_amount: "€10.00"
warnings: []
```

## `wolt venue menu <slug>`

Synopsis:

```console
wolt venue menu <slug> [--category <slug>] [--include-options] [--limit <n>] [global flags]
```

Arguments:
- `<slug>`: venue slug

Options:
- `--category`: restrict to one category
- `--include-options`: include option-group IDs per item
- `--limit`: cap number of menu items returned

Output schema:
- `VenueMenu`

Examples:

```console
wolt venue menu burger-king-finnoo --include-options --limit 30 --format json
wolt venue menu burger-king-finnoo --category meals --format yaml
```

## `wolt venue hours <slug>`

Synopsis:

```console
wolt venue hours <slug> [--timezone <iana>] [global flags]
```

Arguments:
- `<slug>`: venue slug

Options:
- `--timezone`: output timezone (for example `Europe/Helsinki`)

Output schema:
- `VenueHours`

Examples:

```console
wolt venue hours burger-king-finnoo --timezone Europe/Helsinki --format json
wolt venue hours burger-king-finnoo --format yaml
```

## `wolt item show <venue-slug> <item-id>`

Synopsis:

```console
wolt item show <venue-slug> <item-id> [--include-upsell] [global flags]
```

Arguments:
- `<venue-slug>`: venue slug
- `<item-id>`: item identifier

Options:
- `--include-upsell`: include frequently bought together/upsell items

Output schema:
- `ItemDetail`

Examples:

```console
wolt item show burger-king-finnoo 676939cb70769df4cec6cc6f --include-upsell --format json
wolt item show burger-king-finnoo 676939cb70769df4cec6cc6f --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_item_show_001",
    "generated_at": "2026-02-19T21:18:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "item_id": "676939cb70769df4cec6cc6f",
    "venue_id": "629f1f18480882d6f02c25f0",
    "name": "WHOPPER® big meal",
    "description": "Burger meal with drink and side",
    "price": {
      "amount": 1595,
      "formatted_amount": "€15.95"
    },
    "option_groups": [
      {
        "group_id": "6995b94045f708d8b1ad1c0a",
        "name": "Select Drink",
        "required": true,
        "min": 1,
        "max": 1
      }
    ],
    "upsell_items": [
      {
        "item_id": "676939cb70769df4cec6ccaf",
        "name": "Chicken Nuggets 9 pcs + dip",
        "price": {
          "amount": 745,
          "formatted_amount": "€7.45"
        }
      }
    ]
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_item_show_001
  generated_at: "2026-02-19T21:18:00Z"
  profile: default
  locale: en-FI
data:
  item_id: "676939cb70769df4cec6cc6f"
  venue_id: "629f1f18480882d6f02c25f0"
  name: WHOPPER® big meal
  description: Burger meal with drink and side
  price:
    amount: 1595
    formatted_amount: "€15.95"
  option_groups:
    - group_id: "6995b94045f708d8b1ad1c0a"
      name: Select Drink
      required: true
      min: 1
      max: 1
  upsell_items:
    - item_id: "676939cb70769df4cec6ccaf"
      name: Chicken Nuggets 9 pcs + dip
      price:
        amount: 745
        formatted_amount: "€7.45"
warnings: []
```

## `wolt item options <venue-slug> <item-id>`

Synopsis:

```console
wolt item options <venue-slug> <item-id> [global flags]
```

Behavior:
- Fetches item payload from `GET https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/<venue_id>/item/<item_id>`
- Falls back to venue dynamic payload when item endpoint is unavailable.
- Returns normalized option groups with selectable values and ready-to-use `--option` examples for `cart add`.

Output schema:
- `item_id`
- `venue_id`
- `currency`
- `group_count`
- `option_groups[]`:
  - `group_id`
  - `name`
  - `required`
  - `min`
  - `max`
  - `values[]` (`value_id`, `name`, `price`, `example_option`)

## Integration Notes (Current Implementation)

Current `wolt venue show` resolves venue details from
`/v3/venues/<venue_id>` and keeps those details available with
machine-readable output support.
