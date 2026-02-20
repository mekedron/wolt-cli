# Profile Address Book Commands

This page focuses on Wolt address-book management and map validation helpers.

## Shared flags

All commands below support global flags:

- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

Auth is required for all address-book calls.

## `wolt profile addresses`

```console
wolt profile addresses [--active-only] [global flags]
```

Lists saved addresses from:

- `GET https://restaurant-api.wolt.com/v2/delivery/info`

Returns:

- `addresses[]:{address_id,label,street,is_default}`
- `profile_default_address_id`

## `wolt profile addresses add`

```console
wolt profile addresses add \
  --address "<formatted>" \
  --lat <value> \
  --lon <value> \
  [--type <apartment|office|house|outdoor|other>] \
  [--label <home|work|other>] \
  [--alias <text>] \
  [--detail key=value ...] \
  [--set-default-profile] \
  [global flags]
```

Creates a new address with:

- `POST https://restaurant-api.wolt.com/v2/delivery/info`

Notes:

- `--label home` or `--label work` maps to Wolt label type.
- `--alias` is useful with `--label other` for custom labels.

## `wolt profile addresses update <address-id>`

```console
wolt profile addresses update <address-id> \
  --address "<formatted>" \
  --lat <value> \
  --lon <value> \
  [--type <apartment|office|house|outdoor|other>] \
  [--label <home|work|other>] \
  [--alias <text>] \
  [--detail key=value ...] \
  [--set-default-profile] \
  [global flags]
```

Updates by posting a new version with `previous_version`, matching web behavior.

## `wolt profile addresses remove <address-id>`

```console
wolt profile addresses remove <address-id> [global flags]
```

Deletes one address:

- `DELETE https://restaurant-api.wolt.com/v2/delivery/info/{address-id}`

## `wolt profile addresses use <address-id>`

```console
wolt profile addresses use <address-id> [global flags]
```

Sets local profile default pointer (`wolt_address_id`) in config.

## `wolt profile addresses links [address-id]`

```console
wolt profile addresses links [address-id] [global flags]
```

Generates Google Maps validation URLs:

- `address_link`
- `entrance_link`
- `coordinates_link`

If `address-id` is omitted, command uses profile default `wolt_address_id`.

Example:

```console
wolt profile addresses links --profile Default
```
