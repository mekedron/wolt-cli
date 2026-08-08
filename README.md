# wolt-cli

<img src="assets/logo.png" alt="wolt-cli logo" width="160" />

`wolt-cli` is an unofficial community Go CLI for interacting with Wolt endpoints from a terminal.
It is not affiliated with Wolt. Use it at your own responsibility.

<p align="center">
  <a href="#local-stats-dashboard">
    <img src="docs-site/static/img/stats/dashboard-overview.png" alt="wolt stats — local order-history dashboard" width="900" />
  </a>
  <br />
  <sub>Run <code>wolt stats</code> to explore your own order history locally — see the <a href="#local-stats-dashboard">Local Stats Dashboard</a> section.</sub>
</p>

## What It Covers

- discovery feed grouped by section (`wolt feed`), with `--summary` for a one-line-per-section overview
- top-N flattened picks across the feed (`wolt top`) — the "what should I eat right now" shortcut
- venue browsing, filtering, sorting, and category listing (`wolt venues`)
- globally ranked item search across nearby venues (`wolt items`)
- venue details, menus (with `--query` / `--category`), hours, and item drilldown
- option matrix inspection and option resolution by name (`--option "Drink=Cola"`)
- cart commands (`cart`, `cart count`, `cart add`, `cart remove`, `cart clear`)
- checkout projection (`checkout`, no order placement)
- single-account commands (`login`, `logout`, `status`, `account`)
- local stats dashboard (`wolt stats`) — fetches a pre-built bundle, syncs your order history into SQLite, serves the dashboard at `http://127.0.0.1:5173`, and opens the browser. No Node.js required at runtime.
- discovery enrichments: `menu_highlights[]` from `venue_preview_items`, badge glyphs in the venue cell from `badges_v2`, brand carousels ("Popular stores", "Restaurant categories") as one-line summaries
- token rotation using the refresh token (`--wrtoken`)

## Requirements

- Go `1.26+`

## Development Setup

**Required for all contributors.** Right after `git clone`, install the
repo-tracked git hooks so commits and pushes are gated by the same checks
the remote CI runs. Without this step, pushes tend to fail CI on the
"trivial" gates (gofmt drift, missing lint fixes, race-detector regressions).

```bash
make install-hooks
# or, equivalently:
./scripts/install-git-hooks.sh
```

The installer wires `git config core.hooksPath -> scripts/git-hooks/`, so
the hooks live in-tree (`scripts/git-hooks/`) and update automatically with
every `git pull`. It is idempotent — safe to re-run.

What the hooks do:

- **`pre-commit`** (fast) — runs `gofmt -l` on staged Go files and
  `go vet ./...`. Blocks commits with formatting drift or vet errors.
- **`pre-push`** (full CI parity, only when pushing `main` or a `v*` tag) —
  runs `go mod download`, `go build ./...`, the versioned `wolt --version`
  round-trip, `golangci-lint run`, `go test ./...`, and `go test -race ./...`.
  Mirrors `.github/workflows/go-ci.yml` exactly.

If `golangci-lint` is missing the installer prints a hint; you'll also need:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
```

Emergency bypass (avoid unless something is on fire): `git commit --no-verify`,
`git push --no-verify`.

## Recommended Install (Homebrew Tap)

Use the dedicated tap at [mekedron/tap](https://github.com/mekedron/tap):

```bash
brew tap mekedron/tap
brew install wolt-cli
```

Or as a one-liner without adding the tap first:

```bash
brew install mekedron/tap/wolt-cli
```

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

Log in first:

```bash
wolt login
```

Manual token login:

```bash
wolt login --wtoken "<token>" --wrtoken "<refresh-token>"
```

Cookie auth is also supported:

```bash
wolt login --cookie "__wtoken=<token>" --cookie "__wrtoken=<refresh-token>"
```

## Config Location

Configuration is loaded from:
- `WOLT_CONFIG_PATH` (if set)
- otherwise `~/.wolt/.wolt-config.json`

Example config: `configs/example.config.json`

## Common Flags

Global flags for all leaf commands:
- `--format [table|json|yaml]`
- `--address <text>` (temporary location override; geocoded to coordinates)
- `--locale <bcp47>`
- `--no-color`
- `--verbose` (prints upstream HTTP request trace and detailed error diagnostics)

Shared location override flags for location-aware commands:
- `--lat <float>`
- `--lon <float>`

Rules:
- `--lat` and `--lon` must be provided together
- `--address` cannot be combined with `--lat/--lon`
- location overrides are preview inputs only; final order placement in Wolt uses the delivery address selected in your Wolt account

## Quick Tour

The fastest path from "I'm hungry" to a planned cart:

```bash
# What's good near me right now? (single ranked table; no jq needed)
wolt top 10

# Glance the whole home page in one screen (one line per section)
wolt feed --summary

# Drill into a venue
wolt venue noodle-story-kamppi
wolt venue menu noodle-story-kamppi --query "udon"

# Add by name (resolves to item id via the venue search)
wolt cart add noodle-story-kamppi --query "Teriyaki Udon"

# Preview totals without placing an order
wolt checkout
```

## Example: Find a Venue, Inspect Options, Build a Cart

```bash
# 0) Validate account (friendly hint when the session expires)
wolt status --verbose
wolt account --format json

# 1) Find a venue
wolt venues --query "burger king" --limit 10
# Or search products across all nearby venues while keeping Wolt relevance:
wolt items --query "whopper" --limit 20
# Want a single ranked list across all curated sections? Use top:
wolt top 10 --query burger

# 2) Inspect a venue's menu (one of two paths)
wolt venue menu burger-king-finnoo --include-options              # full menu
wolt venue menu burger-king-finnoo --query "whopper" --include-options  # search

# For big marketplace catalogs prefer the category-first flow:
wolt venue categories wolt-market-niittari --limit 30
wolt venue menu wolt-market-niittari --category <category-slug> --include-options

# 3) Inspect a single item (URL form works too)
wolt venue item burger-king-finnoo <item-id>
wolt venue item "https://wolt.com/en/fin/espoo/venue/burger-king-finnoo/itemid-<id>"

# 4a) Add by item id with explicit option ids (most precise)
wolt cart add burger-king-finnoo <item-id> \
  --option "<drink-group-id>=<drink-value-id>" \
  --option "<side-group-id>=<side-value-id>" \
  --count 1

# 4b) Add by name with option values resolved by name (most ergonomic)
wolt cart add burger-king-finnoo --query "WHOPPER Meal" \
  --option "Drink=Coca-Cola Zero" \
  --option "Side=Fries L" \
  --count 1

# 5) Verify cart and preview checkout (no order placement)
wolt cart --details --venue-id <venue-id>
wolt checkout --delivery-mode standard --venue-id <venue-id>
# Final order placement still happens in the official Wolt app/website.

# Optional cleanup
wolt cart clear --venue-id <venue-id>
```

## Other Common Flows

```bash
wolt account addresses
wolt account orders --limit 20
wolt account order <purchase-id>
wolt account payments
wolt account favorites --limit 20
```

## Local Stats Dashboard

`wolt stats` is the one-line shortcut for exploring your order history in a
visual dashboard. It downloads a pre-built bundle of
[`wolt-stats`](https://github.com/mekedron/wolt-stats) from GitHub Releases,
syncs your orders into a local SQLite file, starts a small embedded HTTP
server, and opens your browser. There is no Node.js install step — the
dashboard is shipped as a static HTML/JS/WASM bundle, and the sync runs
inside `wolt-cli` itself.

```bash
# All-in-one. First run downloads the bundle (~1.5 MB) and the full order
# history; subsequent runs are incremental.
wolt stats

# Force a full re-sync (use after a long absence or to repair data).
wolt stats --resync

# Skip the sync (open the dashboard against the database that's already on disk).
wolt stats --no-sync

# Just print the dashboard URL — useful in scripts and CI.
wolt stats --no-open --format json

# Pin to a specific dashboard release.
wolt stats --bundle-version v0.1.0
```

Everything lives under `~/.wolt/stats/` (override with `--stats-dir` or
`$WOLT_STATS_DIR`). The SQLite file is at
`~/.wolt/stats/db/wolt-history.sqlite`. The server binds to `127.0.0.1` only.

The sync is resilient by design: catalog and detail phases share an
inter-call pacer (1.1 s/call baseline) that auto-adjusts when Wolt returns
`HTTP 429`, individual retries honor `Retry-After` with exponential backoff,
and incremental mode picks up exactly where a previous run stopped.

Full reference (sync model, flags, schema, privacy): [`docs/stats.md`](docs/stats.md).

## Rendering Notes

- Tables are column-aligned via the standard library's `tabwriter`. The
  `Highlights` column auto-renders when at least one row has data (force
  with `--show-highlights`, hide with `--show-highlights=false`).
- Badge glyphs are prefixed to the venue cell (`+ Wolt+`, `% 20% off`,
  `⚡ Fast`). If your terminal renders them as boxes, set
  `WOLT_BADGES_PLAIN=1` for a bracketed-text fallback (`[Wolt+]`).
- Brand carousels ("Popular stores", "Restaurant categories", …) appear
  in `wolt feed` as a single-line summary; `wolt feed --query <text>`
  matches against brand names as well as venues.

## MCP server for AI clients

`wolt-cli` ships a second binary, `wolt-mcp`, that exposes every read-only
discovery and account flow (plus cart and checkout-preview) over the
[Model Context Protocol](https://modelcontextprotocol.io). Wire it into
Claude Desktop, Claude Code, Cursor, or any other MCP host and the AI gets a
typed, schema-described tool surface for Wolt instead of having to shell out.

```json
{
  "mcpServers": {
    "wolt": { "command": "wolt-mcp" }
  }
}
```

Auth is shared with the CLI — log in once via `wolt login` and `wolt-mcp`
inherits the same session. Full tool catalog and per-client setup steps:
[`docs/mcp.md`](docs/mcp.md).

## Documentation

- [`docs/commands.md`](docs/commands.md) — full command reference with flags and behavior
- [`docs/stats.md`](docs/stats.md) — local stats dashboard: sync model, flags, schema, privacy
- [`docs/mcp.md`](docs/mcp.md) — MCP server: tool catalog, client setup, troubleshooting
- [`docs/output-contract.md`](docs/output-contract.md) — JSON/YAML envelope and per-command schemas
- [`docs/discovery-enrichment.md`](docs/discovery-enrichment.md) — design notes on `venue_preview_items`, `badges_v2`, and brand carousels
- [`docs/roadmap.md`](docs/roadmap.md) — upcoming ergonomics

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

Profile config may contain `wtoken`, `wrtoken`, and cookies.
Keep config local and do not commit it.
Local config patterns are ignored by `.gitignore` (`.wolt/`, `.wolt-config.json`, `*.wolt-config.json`).

## Copyright

Copyright (c) 2026 Nikita Rabykin.
