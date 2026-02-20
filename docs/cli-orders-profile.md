# Profile Commands

Included commands:
- `wolt profile status`
- `wolt profile show`
- `wolt profile addresses`
- `wolt profile payments`
- `wolt profile favorites`

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

## `wolt profile status`

Synopsis:

```console
wolt profile status [global flags]
```

Behavior:
- Calls `GET https://restaurant-api.wolt.com/v1/user/me`
- Same auth session probe as `wolt auth status`

## `wolt profile show`

Synopsis:

```console
wolt profile show [--include personal,settings] [global flags]
```

Behavior:
- Calls `GET https://restaurant-api.wolt.com/v1/user/me`
- Returns `ProfileSummary` shape:
  - `user_id`
  - `name`
  - `email_masked`
  - `phone_masked`
  - `country`
- Optional include blocks:
  - `personal`
  - `settings`

## `wolt profile addresses`

Synopsis:

```console
wolt profile addresses [--active-only] [global flags]
```

Behavior:
- Calls `GET https://restaurant-api.wolt.com/v2/delivery/info`
- Returns Wolt saved address-book entries
- Output includes:
  - `addresses[]`
  - `profile_default_address_id`

Notes:
- `--active-only` keeps only the profile-selected default Wolt address ID.

Subcommands:

### `wolt profile addresses add`

```console
wolt profile addresses add --address "<text>" --lat <value> --lon <value> [--type <apartment|office|house|outdoor|other>] [--label <home|work|other>] [--alias <text>] [--detail key=value ...] [--set-default-profile] [global flags]
```

- Creates a new Wolt address (`POST /v2/delivery/info`)
- `--label` controls Wolt address label type
- `--alias` sets custom label text (for example with `--label other`)

### `wolt profile addresses update <address-id>`

```console
wolt profile addresses update <address-id> --address "<text>" --lat <value> --lon <value> [--type <...>] [--label <home|work|other>] [--alias <text>] [--detail key=value ...] [--set-default-profile] [global flags]
```

- Updates an existing address by posting a new version with `previous_version`
- Uses the same payload shape as Wolt web

### `wolt profile addresses remove <address-id>`

```console
wolt profile addresses remove <address-id> [global flags]
```

- Deletes an address (`DELETE /v2/delivery/info/{id}`)

### `wolt profile addresses use <address-id>`

```console
wolt profile addresses use <address-id> [global flags]
```

- Sets local profile `wolt_address_id` default pointer

### `wolt profile addresses links [address-id]`

```console
wolt profile addresses links [address-id] [global flags]
```

- Generates Google Maps validation links:
  - `address_link`
  - `entrance_link`
  - `coordinates_link`
- If `address-id` is omitted, uses profile default `wolt_address_id`

## `wolt profile payments`

Synopsis:

```console
wolt profile payments [--mask-sensitive] [--label <contains>] [global flags]
```

Behavior:
- Calls `GET https://restaurant-api.wolt.com/v3/user/me/payment_methods` (saved methods fallback)
- Calls `GET https://payment-service.wolt.com/v1/payment-methods/profile` (full payment methods as shown in Wolt web UI)
- Normalizes response into:
  - `methods[]:{method_id,type,label,is_default,is_available_for_checkout}`

`--mask-sensitive` masks payment labels in output.
`--label` filters methods by case-insensitive label match (for example `--label revolut`).

## `wolt profile favorites`

Synopsis:

```console
wolt profile favorites [--lat <value> --lon <value>] [global flags]
```

Behavior:
- Calls `GET https://consumer-api.wolt.com/v1/pages/venue-list/profile/favourites`
- Returns normalized favorite venues:
  - `favorites[]:{venue_id,slug,name,address,rating,is_favorite,url,price_range,currency,country,delivery_price_int,estimate}`
  - `count`
- Uses profile location by default; `--lat/--lon` can override.

Subcommands:

### `wolt profile favorites list`

```console
wolt profile favorites list [--lat <value> --lon <value>] [global flags]
```

- Explicit list alias for `wolt profile favorites`

### `wolt profile favorites add <venue-id-or-slug>`

```console
wolt profile favorites add <venue-id-or-slug> [global flags]
```

- Marks a venue as favorite:
  - resolves a 24-char Wolt venue ID directly
  - resolves slug from a Wolt venue URL or slug input (for example `rioni-espoo`)
- Calls `PUT https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`

### `wolt profile favorites remove <venue-id-or-slug>`

```console
wolt profile favorites remove <venue-id-or-slug> [global flags]
```

- Removes a venue from favorites:
  - resolves a 24-char Wolt venue ID directly
  - resolves slug from a Wolt venue URL or slug input
- Calls `DELETE https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`
