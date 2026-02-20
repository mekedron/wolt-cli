# Profile Commands

Included commands:
- `wolt profile status`
- `wolt profile show`
- `wolt profile orders`
- `wolt profile addresses`
- `wolt profile payments`
- `wolt profile favorites`

Shared/global flags are documented in `cli-overview`.

## `wolt profile status`

```console
wolt profile status [global flags]
```

Behavior:
- calls `GET https://restaurant-api.wolt.com/v1/user/me`
- same auth probe as `wolt auth status`
- includes `wolt_plus_subscriber` when available from user profile payload

## `wolt profile show`

```console
wolt profile show [--include personal,settings] [global flags]
```

Behavior:
- calls `GET https://restaurant-api.wolt.com/v1/user/me`
- returns `ProfileSummary` fields (`user_id`, `name`, masked contact, country)

## `wolt profile orders`

```console
wolt profile orders [--limit <1-50>] [--page-token <token>] [--status <value>] [global flags]
```

Aliases:
- `history`
- `order-history`

Behavior:
- calls `GET https://consumer-api.wolt.com/order-tracking-api/v1/order_history/?limit=<n>`
- forwards `page_token` when `--page-token` is provided
- supports local status filter (`--status`) after upstream payload is read
- returns normalized list plus `count` and optional `next_page_token`

Subcommands:

### `wolt profile orders list`

```console
wolt profile orders list [--limit <1-50>] [--page-token <token>] [--status <value>] [global flags]
```

### `wolt profile orders show <purchase-id>`

```console
wolt profile orders show <purchase-id> [global flags]
```

Behavior:
- calls `GET https://consumer-api.wolt.com/order-tracking-api/v1/order_history/purchase/{purchase_id}?tips_use_percentage=true`
- returns order totals in minor units and formatted currency values

## `wolt profile addresses`

```console
wolt profile addresses [--active-only] [global flags]
```

Behavior:
- calls `GET https://restaurant-api.wolt.com/v2/delivery/info`
- returns Wolt address-book entries and `profile_default_address_id`

Subcommands:

### `wolt profile addresses add`

```console
wolt profile addresses add --address "<text>" --lat <value> --lon <value> [--type <apartment|office|house|outdoor|other>] [--label <home|work|other>] [--alias <text>] [--detail key=value ...] [--set-default-profile] [global flags]
```

### `wolt profile addresses update <address-id>`

```console
wolt profile addresses update <address-id> --address "<text>" --lat <value> --lon <value> [--type <apartment|office|house|outdoor|other>] [--label <home|work|other>] [--alias <text>] [--detail key=value ...] [--set-default-profile] [global flags]
```

### `wolt profile addresses remove <address-id>`

```console
wolt profile addresses remove <address-id> [global flags]
```

### `wolt profile addresses use <address-id>`

```console
wolt profile addresses use <address-id> [global flags]
```

### `wolt profile addresses links [address-id]`

```console
wolt profile addresses links [address-id] [global flags]
```

## `wolt profile payments`

```console
wolt profile payments [--mask-sensitive] [--label <contains>] [global flags]
```

Behavior:
- calls `GET https://restaurant-api.wolt.com/v3/user/me/payment_methods` (fallback list)
- calls `GET https://payment-service.wolt.com/v1/payment-methods/profile` (full web-style list)
- normalizes methods to `method_id`, `type`, `label`, `is_default`, `is_available_for_checkout`

## `wolt profile favorites`

```console
wolt profile favorites [--address "<text>" | --lat <value> --lon <value>] [global flags]
```

Behavior:
- calls `GET https://consumer-api.wolt.com/v1/pages/venue-list/profile/favourites`
- returns normalized favorite venues list with `count`
- supports shared location overrides from `cli-overview` (`--address` or `--lat` + `--lon`)

Subcommands:

### `wolt profile favorites list`

```console
wolt profile favorites list [global flags]
```

### `wolt profile favorites add <venue-id-or-slug>`

```console
wolt profile favorites add <venue-id-or-slug> [global flags]
```

Behavior:
- resolves venue id directly or from slug/url
- when slug lookup fallback is needed, location comes from profile by default or global `--address`
- calls `PUT https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`

### `wolt profile favorites remove <venue-id-or-slug>`

```console
wolt profile favorites remove <venue-id-or-slug> [global flags]
```

Behavior:
- resolves venue id directly or from slug/url
- when slug lookup fallback is needed, location comes from profile by default or global `--address`
- calls `DELETE https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`
