# wolt-cli

`wolt-cli` is an unofficial community Go CLI for interacting with Wolt endpoints from a terminal.
It is not affiliated with Wolt. Use it at your own responsibility.

## What It Covers

- discovery feed and category listing
- venue and item search
- venue details, menus, and hours
- item detail and option matrix inspection
- cart commands (`show`, `count`, `add`, `remove`, `clear`)
- checkout projection (`checkout preview`, no order placement)
- profile/auth commands (`status`, `show`, addresses, payments, favorites)
- token rotation using refresh token (`--wrtoken`)

## Requirements

- Go `1.26+`

## Build and Run

```bash
go build ./...
go build -o bin/wolt ./cmd/wolt
./bin/wolt --help
```

Or without installing:

```bash
go run ./cmd/wolt --help
```

## First Command to Run

Configure a profile first:

```bash
wolt configure --profile-name default --address "<address>" --overwrite
```

Configure auth in the same profile:

```bash
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>"
```

Cookie auth is also supported:

```bash
wolt configure --profile-name default --cookie "__wtoken=<token>" --cookie "__wrtoken=<refresh-token>"
```

## Config Location

Configuration is loaded from:
- `WOLT_CONFIG_PATH` (if set)
- otherwise `~/.wolt/.wolt-config.json`

Example config: `configs/example.config.json`

## Common Flags

Global flags for all leaf commands:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

Shared location override flags for location-aware commands:
- `--lat <float>`
- `--lon <float>`

`--lat` and `--lon` must be provided together.

## Typical Flows

```bash
# Validate auth/profile
wolt profile status --verbose
wolt profile show --format json

# Menu -> item options -> cart -> checkout preview
wolt venue menu burger-king-finnoo --include-options --format json
wolt item options burger-king-finnoo <item-id> --format json
wolt cart add <venue-id> <item-id> --option "<group-id>=<value-id>" --format json
wolt cart show --details --format json
wolt checkout preview --delivery-mode standard --format json

# Profile workflows
wolt profile addresses --format json
wolt profile payments --format json
wolt profile favorites --format json
```

## Docs

- `docs/cli-overview.md`
- `docs/cli-installation.md`
- `docs/cli-auth.md`
- `docs/cli-discovery-search.md`
- `docs/cli-venue-item.md`
- `docs/cli-cart-checkout.md`
- `docs/cli-orders-profile.md`
- `docs/cli-profile-addresses.md`
- `docs/cli-output-contract.md`

## Test and Lint

```bash
go test ./...
make lint
```

If `golangci-lint` is missing:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

## Security

Profile config may contain `wtoken`, `wrefresh_token`, and cookies.
Keep config local and do not commit it.
Local config patterns are ignored by `.gitignore` (`.wolt/`, `.wolt-config.json`, `*.wolt-config.json`).
