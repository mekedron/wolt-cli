# CLI Overview

`wolt-cli` is an unofficial community CLI for interacting with Wolt endpoints from a terminal.
It is not affiliated with Wolt. Use it at your own responsibility.

## Start Here

Install via Homebrew tap:

```console
brew tap mekedron/tap
brew install wolt-cli
```

or:

```console
brew install mekedron/tap/wolt-cli
```

See `cli-installation` for build-from-source instructions.

First command to run:

```console
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>" --overwrite
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
- `--address <text>` (temporary location override; geocoded to coordinates)
- `--locale <bcp47>`
- `--no-color`
- `--verbose` (prints upstream HTTP request trace and detailed error diagnostics)
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

Auth fallback order:
1. explicit command flags (`--wtoken`, `--wrtoken`, `--cookie`)
2. selected profile auth fields (`wtoken`, `wrefresh_token`, `cookies`)
3. default profile auth fields

When refresh credentials are available, expired/401 access tokens are rotated automatically and persisted back into the selected profile.

## Shared Location Inputs

Location-aware commands support:
- `--address <text>` (temporary address override)
- `--lat <float>`
- `--lon <float>`

Rules:
- provide both `--lat` and `--lon` together
- do not combine `--address` with `--lat/--lon`
- if all overrides are omitted, location is resolved from the selected Wolt account address
- with only one coordinate flag, command returns `WOLT_INVALID_ARGUMENT`

Used by:
- `discover feed`, `discover categories`
- `cart show`, `cart remove`, `cart clear`, `checkout preview`
- `profile favorites`, `profile favorites list`
- `search venues`, `search items` (address/account address only)
- `venue show`, `venue hours` (address/account address only)

## Safety

- `checkout preview` is projection-only and does not place orders.
- any `--address` / `--lat` / `--lon` override only affects preview/read endpoints.
- final order placement in Wolt uses the delivery address selected in your Wolt account.
- There is no order placement command.

For large marketplace-style venues, prefer `wolt venue search <slug> --query "<text>"` to find items quickly instead of forcing full menu traversal.

## Quick Reference

```console
wolt venue categories burger-king-finnoo --format json
wolt venue search wolt-market-niittari --query "milk" --format json
wolt venue menu burger-king-finnoo --include-options --format json
wolt item options burger-king-finnoo <item-id> --format json
wolt cart show --details --format json
wolt checkout preview --delivery-mode standard --format json
wolt profile orders --limit 20 --format json
wolt profile orders show <purchase-id> --format json
wolt profile payments --format json
wolt profile favorites --format json
```
