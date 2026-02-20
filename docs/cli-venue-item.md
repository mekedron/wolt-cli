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

## `wolt venue menu <slug>`

```console
wolt venue menu <slug> [--category <slug>] [--include-options] [--limit <n>] [global flags]
```

Options:
- `--category`: restrict to one category
- `--include-options`: include option-group IDs per item
- `--limit`: cap number of returned items

Behavior:
- loads venue metadata from static venue page endpoint
- loads menu/option topology from assortment endpoint
- does not require discovery catalog lookup

Output schema:
- `VenueMenu`

Notes:
- venue payload includes `wolt_plus` participation flag
- each menu item includes `discounts[]` when promotion metadata is present upstream

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
- returns an error if the provided item is not found in the venue menu

Output schema:
- `ItemDetail`

## `wolt item options <venue-slug> <item-id>`

```console
wolt item options <venue-slug> <item-id> [global flags]
```

Behavior:
- resolves option groups from item payload and assortment payload
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
wolt venue menu <slug> --include-options --format json
wolt item options <slug> <item-id> --format json
wolt cart add <venue-id> <item-id> --option "<group-id>=<value-id>" --format json
```
