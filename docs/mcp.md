# MCP server (`wolt-mcp`)

`wolt-mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server
that gives AI clients (Claude Desktop, Claude Code, Cursor, Continue, Zed, …)
a typed tool surface for Wolt. The same business logic the `wolt` CLI calls is
exposed in-process — no shell-out, no flag-guessing by the model.

## Why an MCP server instead of "just use the CLI"

- The model gets JSON-Schema typed inputs for every tool, so it stops fabricating
  flag combinations.
- Errors are returned in-band (e.g. *"Wolt session expired or missing. Run
  'wolt login'…"*) — the model self-corrects instead of failing silently.
- One stdio process per host session, instead of one subprocess per CLI call.

## Install

`wolt-mcp` is built and released alongside the `wolt` binary. Once `wolt` is on
your `$PATH`, `wolt-mcp` is too.

```bash
brew install mekedron/tap/wolt-cli   # installs both wolt and wolt-mcp
```

From source:

```bash
go install github.com/mekedron/wolt-cli/cmd/wolt-mcp@latest
```

Verify:

```bash
wolt-mcp --version
```

## Wire it into your MCP client

The same one-line config works in every client that speaks MCP over stdio.

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)
or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "wolt": { "command": "wolt-mcp" }
  }
}
```

Restart Claude Desktop. The `wolt` server should appear in the tool inventory.

### Claude Code

Add to `.mcp.json` at the project root (or `~/.claude.json` globally):

```json
{
  "mcpServers": {
    "wolt": { "command": "wolt-mcp" }
  }
}
```

### Cursor

`~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "wolt": { "command": "wolt-mcp" }
  }
}
```

### Other clients

Anything that supports stdio MCP servers accepts the same `command` /`args`
shape. If your client needs an absolute path, find one with `which wolt-mcp`.

## Authentication

`wolt-mcp` shares `~/.wolt/.wolt-config.json` (or `$WOLT_CONFIG_PATH`) with the
CLI. Log in once:

```bash
wolt login                 # interactive browser flow
# or
wolt login --wtoken "<token>" --wrtoken "<refresh-token>"
```

Both binaries read and rotate the same tokens. Refresh-token rotation is handled
inside the MCP server too: on a `401` response the server hits `RefreshAccessToken`,
persists the rotated tokens, and retries the original call once.

## Tool catalog

24 tools in v1. ✓ = read-only, ⚠ = mutates user state.

### Discovery (no auth required)

| Tool | What it does |
|---|---|
| `wolt_feed` ✓ | Discovery home page grouped by section ("Popular", "Order again", …) |
| `wolt_top` ✓ | Top N venues for a location, with sort/filter |
| `wolt_search_venues` ✓ | Search venues with category, open-now, Wolt+, rating, fee filters |
| `wolt_venue_categories` ✓ | Available category slugs at a location |
| `wolt_resolve_address` ✓ | Geocode a free-form address to lat/lon |

### Venue (no auth required)

| Tool | What it does |
|---|---|
| `wolt_venue_detail` ✓ | Address, rating, delivery methods, opening windows, tags |
| `wolt_venue_menu` ✓ | Browse a venue's menu with image URLs and current availability (paginated, filterable) |
| `wolt_venue_hours` ✓ | Opening windows in the venue's timezone |
| `wolt_venue_item` ✓ | Current item payload (price, options, image URLs, availability) |
| `wolt_venue_search_items` ✓ | Free-text item search within a venue |

### Account (auth required)

| Tool | What it does |
|---|---|
| `wolt_account_status` ✓ | Current user profile |
| `wolt_account_orders` ✓ | Paginated order history |
| `wolt_account_order` ✓ | Full detail for one order |
| `wolt_account_addresses` ✓ | Saved delivery addresses |
| `wolt_account_payments` ✓ | Saved payment methods |

### Favorites (auth required)

| Tool | What it does |
|---|---|
| `wolt_favorites_list` ✓ | Favorited venues |
| `wolt_favorites_add` ⚠ | Add a venue to favorites |
| `wolt_favorites_remove` ⚠ | Remove a venue from favorites |

### Cart + checkout (auth required)

| Tool | What it does |
|---|---|
| `wolt_cart_show` ✓ | All baskets with item lines and totals |
| `wolt_cart_count` ✓ | Total items across all baskets |
| `wolt_cart_add` ⚠ | Add an item to a basket (merges with existing items) |
| `wolt_cart_remove` ⚠ | Remove an item line from a basket |
| `wolt_cart_clear` ⚠ | Delete every basket the user has |
| `wolt_checkout_preview` ✓ | Preview totals without placing an order |

> **Note:** No tool places an actual order. Final checkout still happens in the
> official Wolt app or web UI. `wolt_checkout_preview` is a pricing preview only.

`wolt_cart_add` always revalidates the exact item before mutating the basket;
caller-supplied display metadata cannot bypass an unavailable item.
`wolt_checkout_preview` revalidates every basket item before sending the
preview request. Both operations fail closed if availability or venue
currency cannot be verified.

## Location precedence

Tools that need a location resolve it in this order:

1. Explicit `lat` + `lon` arguments (both must be set)
2. `address` argument → geocoded via OSM Nominatim
3. `profile.Location` from the persisted CLI config
4. Live `DeliveryInfoList` lookup, if the user is logged in

A tool will return a tool error if none of these can supply coordinates — the
model is expected to either pass `lat`/`lon`, pass `address`, or instruct the
user to `wolt login`.

## Locale

The MCP server speaks one [BCP-47](https://www.rfc-editor.org/rfc/rfc5646)
locale per session. It controls the language of item names, menu copy, and
other translated content returned by every tool — Wolt's API uses it to pick
the venue's localized assortment.

The locale flows to two places in each upstream Wolt request:

- the `app-language` HTTP header
- the `language` query parameter (and the assortment language for
  `wolt_venue_search_items`, derived from the part before the `-`)

**Default:** `en-FI` — English copy, Finland market.

### Setting it

Three equivalent forms, in precedence order:

1. CLI flag (position-independent): `--locale fi-FI` or `--locale=fi-FI`
2. Environment variable: `WOLT_LOCALE=fi-FI`
3. Fallback to `en-FI`

Pinned via `args` in your client config:

```json
{
  "mcpServers": {
    "wolt": {
      "command": "wolt-mcp",
      "args": ["--locale", "fi-FI"]
    }
  }
}
```

Or via `env`, which is friendlier when the same setting should apply to other
processes:

```json
{
  "mcpServers": {
    "wolt": {
      "command": "wolt-mcp",
      "env": { "WOLT_LOCALE": "fi-FI" }
    }
  }
}
```

### Common values

| Locale  | Use when                                                   |
|---------|------------------------------------------------------------|
| `en-FI` | English UI, Finland market (default)                       |
| `fi-FI` | Finnish menu names and copy                                |
| `sv-FI` | Swedish-language menus in Finland                          |
| `et-EE` | Estonian, Estonia market                                   |
| `de-DE` | German menus, Germany market                               |

Any BCP-47 tag Wolt supports for the target market works — the server passes
it through unchanged.

## Troubleshooting

### "Not logged in" errors

The auth-gated tools return:

> *Not logged in. Run 'wolt login' in a terminal to sign in, then retry.*

Run `wolt login` in any terminal. Restart the MCP server (the client usually
reconnects automatically — for Claude Desktop, restart the app).

### Client doesn't see the server

- Confirm `wolt-mcp` is on the path: `which wolt-mcp`. If your MCP client runs
  in a desktop sandbox, it may not inherit your shell `$PATH` — use an absolute
  path in the config (e.g. `/opt/homebrew/bin/wolt-mcp`).
- Check the client's MCP log. Claude Desktop logs are at
  `~/Library/Logs/Claude/mcp*.log` on macOS.

### Stdout pollution

`wolt-mcp` deliberately routes every log line to stderr, because stdout is the
JSON-RPC transport. If a future change adds a stray `fmt.Println(...)`, framing
breaks and the client disconnects. The `test/e2e/mcp_subprocess_test.go` test
guards against this — keep it passing.

### Rate limits

Wolt's API rate-limits aggressive callers. The MCP server inherits the CLI's
`WOLT_HTTP_MIN_INTERVAL_MS` knob (default 220ms between requests). Set it
higher on slow networks:

```json
{
  "mcpServers": {
    "wolt": {
      "command": "wolt-mcp",
      "env": { "WOLT_HTTP_MIN_INTERVAL_MS": "500" }
    }
  }
}
```

## Implementation notes

- Built on `github.com/modelcontextprotocol/go-sdk` v1.6.0 (official Anthropic SDK).
- Stdio transport only in v1 (covers every desktop client).
- The server is in-process — `cmd/wolt-mcp/main.go` builds the same gateway and
  service objects the CLI uses, then calls them directly from typed handlers in
  `internal/mcpserver/`.
- Source: [`cmd/wolt-mcp/`](https://github.com/mekedron/wolt-cli/tree/main/cmd/wolt-mcp) and [`internal/mcpserver/`](https://github.com/mekedron/wolt-cli/tree/main/internal/mcpserver).
