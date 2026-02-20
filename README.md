# wolt (Go)

`wolt` is a production-oriented Go CLI for browsing Wolt discovery data,
searching venues/items, managing carts, previewing checkout totals, and
inspecting profile/auth state.

## Binaries

- Release artifact binary: `wolt` (`./cmd/wolt`)

## Features

- Discovery feed and category listing
- Venue and item search with filters and fallback behavior
- Venue details, menu, opening hours
- Item details with option groups and optional upsell section
- Item option matrix command for cart-ready option/value selections
- Cart commands: show/count/add/remove/clear
- Checkout projection command: `checkout preview` (no order placement)
- Auth/profile commands with automatic access-token rotation via refresh token
- Favorite venues commands: list/add/remove for authenticated profiles
- Profile bootstrap command: `configure`
- JSON/YAML machine output envelope with warnings and structured errors

## Requirements

- Go `1.26+`

## Quick Start

```bash
go build ./...
go run ./cmd/wolt --help
```

## Build

```bash
go build ./...
go build -o bin/wolt ./cmd/wolt
```

Or via Make:

```bash
make build
```

## Run

```bash
go run ./cmd/wolt --help
go run ./cmd/wolt discover feed --format json
go run ./cmd/wolt search venues --query burger --format json
```

## Configuration

Configuration is loaded from:

- `WOLT_CONFIG_PATH` (if set)
- Otherwise: `~/.wolt/.wolt-config.json`

Example config is provided at `configs/example.config.json`.

Create/update config from CLI:

```bash
wolt configure --profile-name default --address "Krakow" --overwrite
```

Optional auth storage in profile:

```bash
wolt configure --profile-name default --address "Krakow" --wtoken "<token>" --overwrite
wolt configure --profile-name default --address "Krakow" --wrtoken "<refresh-token>" --overwrite
wolt configure --profile-name default --address "Krakow" --cookie "__wtoken=<token>" --cookie "foo=bar" --overwrite
wolt configure --profile-name default --address "Krakow" --cookie "__wrtoken=<refresh-token>" --overwrite
# update auth only on an existing profile (keeps address/location unchanged)
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>"
```

Security:
- profile config may contain `wtoken`, `wrefresh_token`, and `cookies`; keep it local and never commit it.
- local config patterns are ignored in `.gitignore` (`.wolt/`, `.wolt-config.json`, `*.wolt-config.json`).

## CLI Examples

```bash
# Discovery
wolt discover feed --format json
wolt discover categories --format yaml

# Search
wolt search venues --format json
wolt search venues --query burger --sort rating --limit 10 --format json
wolt search items --query whopper --format json

# Venue
wolt venue show burger-king-finnoo --include tags,hours --format json
wolt venue menu burger-king-finnoo --include-options --format yaml
wolt venue hours burger-king-finnoo --format json

# Item
wolt item show burger-king-finnoo 676939cb70769df4cec6cc6f --include-upsell --format json
wolt item options burger-king-finnoo 676939cb70769df4cec6cc6f --format json

# Cart / Checkout (safe preview only)
wolt cart show --details --wtoken "<token>" --format json
wolt cart add <venue-id> <item-id> --count 1 --option "<group-id>=<value-id>" --wtoken "<token>" --format json
wolt cart remove <item-id> --count 1 --wtoken "<token>" --format json
wolt cart clear --wtoken "<token>" --format json
wolt checkout preview --delivery-mode standard --wtoken "<token>" --format json

# Auth/Profile
wolt auth status --wtoken "<token>" --format json
wolt auth status --wtoken "<token>" --wrtoken "<refresh-token>" --format json
wolt profile status --wtoken "<token>" --format json
wolt profile payments --wtoken "<token>" --format json
wolt profile favorites --wtoken "<token>" --format json
wolt profile favorites add rioni-espoo --wtoken "<token>" --format json
wolt profile favorites remove 5a8426f188b5de000b8857bb --wtoken "<token>" --format json

# Config bootstrap
wolt configure --profile-name default --address "Krakow" --overwrite
wolt configure --profile-name default --address "Krakow" --wtoken "<token>" --overwrite
wolt configure --profile-name default --address "Krakow" --wrtoken "<refresh-token>" --overwrite
wolt configure --profile-name default --address "Krakow" --cookie "__wtoken=<token>" --overwrite
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>"
```

## Output Contract

See `/docs/cli-output-contract.md` for the canonical machine-output schema.

## Testing

```bash
go test ./...
go test -race ./...
go test ./test/e2e ./test/integration
```

Includes:

- Unit tests (`internal/service/...`)
- Integration tests (`test/integration`)
- E2E-style CLI behavior tests (`test/e2e`)

## Lint

```bash
make lint
```

If `golangci-lint` is not installed locally:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

## Docker

```bash
docker build -t wolt:local .
docker run --rm wolt:local --help
```

Or:

```bash
docker compose up --build
```
