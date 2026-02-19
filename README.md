# wolt (Go)

`wolt` is a production-oriented Go CLI for browsing Wolt discovery data,
searching venues/items, and inspecting venue/item details.

This repository has been migrated from Python to Go with an idiomatic project
layout, dependency-injected architecture, and CI-ready test/lint tooling.

## Binaries

- Release artifact binary: `wolt` (`./cmd/wolt`)

## Features

- Discovery feed and category listing
- Venue and item search with filters and fallback behavior
- Venue details, menu, opening hours
- Item details with option groups and optional upsell section
- Profile bootstrap command: `configure`
- JSON/YAML machine output envelope with warnings and structured errors

## Requirements

- Go `1.26+`

## Quick Start

```bash
go build ./...
go run ./cmd/wolt --help
```

## Recommended Workflow

1. Sync and branch.
2. Implement in `internal/...` and wire commands in `internal/cli/...`.
3. Run fast local checks while iterating.
4. Run full quality gate before push.
5. Push and let CI validate build, lint, tests, and race detector.

Suggested command flow:

```bash
# 1) build + smoke run
go build ./...
go run ./cmd/wolt --help

# 2) focused test while iterating
go test ./test/e2e ./test/integration

# 3) full gate before push
go test ./...
go test -race ./...
go vet ./...
make lint
```

## Project Layout

```text
cmd/
  wolt/main.go
internal/
  cli/                 # command tree and output/error handling
  config/              # config file loading/saving
  domain/              # domain models and format helpers
  gateway/
    wolt/              # Wolt HTTP client
    location/          # address -> lat/lon resolver
  service/
    observability/     # discovery/search/detail transformations
    profile/           # profile resolution
    output/            # envelope and rendering
configs/
api/
migrations/
scripts/
test/
  e2e/
  integration/
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

# Config bootstrap
wolt configure --profile-name default --address "Krakow" --overwrite
```

## Output Contract

Machine formats (`json`, `yaml`) use this envelope:

```json
{
  "meta": {
    "request_id": "req_...",
    "generated_at": "2026-02-19T00:00:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {},
  "warnings": [],
  "error": {
    "code": "WOLT_UPSTREAM_ERROR",
    "message": "..."
  }
}
```

For successful responses, `error` is omitted.

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

## Local QA Gate

```bash
go test ./...
go test -race ./...
go vet ./...
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

## CI

GitHub Actions workflow (`.github/workflows/go-ci.yml`) runs:

- Build
- golangci-lint
- `go test ./...`
- `go test -race ./...`
