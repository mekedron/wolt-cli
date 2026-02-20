# Profile Commands

Included commands:
- `wolt profile status`
- `wolt profile show`
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

## `wolt profile show`

```console
wolt profile show [--include personal,settings] [global flags]
```

Behavior:
- calls `GET https://restaurant-api.wolt.com/v1/user/me`
- returns `ProfileSummary` fields (`user_id`, `name`, masked contact, country)

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
wolt profile favorites [--lat <value> --lon <value>] [global flags]
```

Behavior:
- calls `GET https://consumer-api.wolt.com/v1/pages/venue-list/profile/favourites`
- returns normalized favorite venues list with `count`
- supports shared location overrides (`--lat` + `--lon`) from `cli-overview`

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
- calls `PUT https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`

### `wolt profile favorites remove <venue-id-or-slug>`

```console
wolt profile favorites remove <venue-id-or-slug> [global flags]
```

Behavior:
- resolves venue id directly or from slug/url
- calls `DELETE https://restaurant-api.wolt.com/v3/venues/favourites/{venue_id}`
