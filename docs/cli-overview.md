# CLI Overview

`wolt-cli` is an unofficial community CLI for interacting with Wolt endpoints from a terminal.
It is not affiliated with Wolt. Use it at your own responsibility.

## Start Here

First command to run:

```console
wolt configure --profile-name default --address "<address>" --overwrite
```

Then validate auth/profile:

```console
wolt profile status --verbose
wolt profile show --format json
```

Implemented command groups:
- `configure`
- `auth`
- `discover`
- `search`
- `venue`
- `item`
- `cart`
- `checkout`
- `profile`

Root interface:

```console
wolt <group> <command> [flags]
```

## Global Flags

All command leaf nodes support:
- `--format [table|json|yaml]` (default `table`)
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

Auth fallback order:
1. explicit command flags (`--wtoken`, `--wrtoken`, `--cookie`)
2. selected profile auth fields (`wtoken`, `wrefresh_token`, `cookies`)
3. default profile auth fields

When refresh credentials are available, expired/401 access tokens are rotated automatically and persisted back into the selected profile.

## Shared Location Flags

Location-aware commands support the shared override pair:
- `--lat <float>`
- `--lon <float>`

Rules:
- provide both flags together
- if both are omitted, profile location is used
- if only one is provided, command returns `WOLT_INVALID_ARGUMENT`

Used by:
- `discover feed`, `discover categories`
- `cart show`, `cart remove`, `cart clear`, `checkout preview`
- `profile favorites`, `profile favorites list`

## Safety

- `checkout preview` is projection-only and does not place orders.
- There is no order placement command.

## Quick Reference

```console
wolt venue menu burger-king-finnoo --include-options --format json
wolt item options burger-king-finnoo <item-id> --format json
wolt cart show --details --format json
wolt checkout preview --delivery-mode standard --format json
wolt profile payments --format json
wolt profile favorites --format json
```
