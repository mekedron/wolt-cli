# MCP server (`wolt-mcp`)

`wolt-mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server
that gives AI clients (Claude Desktop, Claude Code, Cursor, Continue, Zed, …)
a typed tool surface for Wolt. Its in-process handlers share the CLI's Wolt
gateway and service packages — no shell-out and no flag-guessing by the model.

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
shape. If your client needs an absolute path, use `command -v wolt-mcp` on
macOS/Linux, `Get-Command wolt-mcp` in PowerShell, or `where.exe wolt-mcp` in
Command Prompt. Windows source builds normally use `wolt-mcp.exe`.

## Authentication

`wolt-mcp` shares `~/.wolt/.wolt-config.json` (or `$WOLT_CONFIG_PATH`) with the
CLI. Log in once:

```bash
wolt login                 # interactive browser flow
# or
wolt login --wtoken "<token>" --wrtoken "<refresh-token>"
```

Both binaries read the same explicit login state. On an expired token or a
`401` response, the server calls `RefreshAccessToken`, caches the refreshed
access token and any rotated refresh token for later tool calls, and retries
once. It safely persists only the refreshed access token when the complete
saved credential snapshot is unchanged. The saved bootstrap refresh token and
cookies stay pinned, while any rotated refresh token remains process-local.
This compare-and-swap rule prevents automatic refresh from overwriting a
concurrent explicit login or logout.

## Tool catalog

25 tools in v1. ✓ = read-only, ⚠ = mutates user state.

### Discovery (no auth required)

| Tool | What it does |
|---|---|
| `wolt_feed` ✓ | Discovery home page grouped by section ("Popular", "Order again", …) |
| `wolt_top` ✓ | Top N venues for a location, with sort/filter |
| `wolt_search_venues` ✓ | Filter the non-exhaustive discovery feed by text, category, availability, Wolt+, rating, and fee |
| `wolt_venue_categories` ✓ | Available category slugs at a location |
| `wolt_resolve_address` ✓ | Geocode a free-form address to lat/lon |

### Venue (no auth required)

| Tool | What it does |
|---|---|
| `wolt_resolve_venue` ✓ | Resolve an exact name, slug, venue ID, or Wolt URL, including closed and scheduled-order venues |
| `wolt_venue_detail` ✓ | Address, rating, delivery methods, opening windows, tags |
| `wolt_venue_menu` ✓ | Browse a venue's menu with image URLs and current availability; exposes partial grocery catalogs and loads a selected category |
| `wolt_venue_hours` ✓ | Opening windows in the venue's timezone |
| `wolt_venue_item` ✓ | Current item payload (price, options, image URLs, availability) |
| `wolt_venue_search_items` ✓ | Free-text item search within a venue |

`wolt_venue_detail` and `wolt_venue_hours` use Wolt's supported venue-page
payloads. Detail results combine static metadata with location-aware dynamic
availability when a location is available; warnings identify optional signals
that could not be loaded.

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
| `wolt_cart_show` ✓ | All baskets with item lines and totals; best-effort current order availability when venue enrichment succeeds |
| `wolt_cart_count` ✓ | Total items across all baskets |
| `wolt_cart_add` ⚠ | Add an item to a basket (merges with existing items) |
| `wolt_cart_remove` ⚠ | Remove an item line from a basket |
| `wolt_cart_clear` ⚠ | Delete every basket the user has |
| `wolt_checkout_preview` ✓ | Preview totals without placing an order |

> **Note:** No tool places an actual order. Final checkout still happens in the
> official Wolt app or web UI. `wolt_checkout_preview` is a pricing preview only.

`wolt_cart_add` always revalidates the exact item before mutating the basket;
caller-supplied display metadata cannot bypass an unavailable item. The tool
does not currently accept option selections, so it fails closed when an item
requires one instead of posting an invalid empty selection.
`wolt_checkout_preview` revalidates every basket item before sending the
preview request. Both operations fail closed if availability or venue
currency cannot be verified.

Basket writes replace the complete upstream item array. One MCP server
serializes its cart mutation tools and validates the full existing snapshot
before writing. Wolt exposes no basket revision or conditional-write token,
so mutations from another MCP process, the CLI, or an official Wolt client can
still race; avoid issuing cart writes concurrently across those clients.

`wolt_checkout_preview.delivery_mode` accepts `standard` and `priority`. Offered
modes come from Wolt's `delivery_configs`, keyed on each entry's stable
`schedule` slug rather than its localized label. Wolt marks no config as
selected — the mode follows from the posted purchase plan — so the requested mode
is reported as applied unless the response never advertised it, names a different
one, or names two at once; those cases return a structured
`DELIVERY_MODE_UNAVAILABLE` result carrying `available_delivery_modes`.
Scheduled ordering is venue availability, not a checkout delivery mode exposed by
the current upstream endpoint.

## Exact venue resolution and discovery

`wolt_search_venues`, `wolt_top`, and `wolt_feed` operate on Wolt's discovery
feed. That feed is ranked and non-exhaustive: a closed, temporarily
unavailable, or less-prominent venue may be absent even when its Wolt page and
catalog still exist.

Use `wolt_resolve_venue` when the user supplies an exact name, slug, 24-character
venue ID, or Wolt URL. It uses Wolt's supported search and venue-page APIs
first, then an exact-name/identity match from discovery when Wolt exposes a
venue only there. It is therefore not limited to discovery and still resolves
closed or scheduled-order venues found by the dedicated search endpoint. The
result returns canonical identity plus separate availability fields for
ordering now, scheduled ordering, closed state, next opening time, and delivery
to the selected location when Wolt supplies those signals.
`wolt_venue_detail` and `wolt_venue_hours` accept the same slug, ID, and URL
forms without requiring a saved profile or location. A location is still needed
to resolve an exact display name and enables location-aware availability.

Rows returned by `wolt_feed`, `wolt_top`, and `wolt_search_venues` also expose
`order_now_available`, `scheduled_order_available`,
`scheduled_pickup_available`, `scheduled_only`, `delivers_to_location`,
`next_opening_at`, `status_text`, and `telemetry_status` when their discovery
row supplies those signals. Missing signals are `null`, not `false`.
`store_open_now` stays `null` in discovery because Wolt's `online` field means
order-now availability, not necessarily that a physical store is open.

When selecting among branches, use the canonical `venue_id` together with
Wolt's returned address. Keep the actual slug and canonical URL for navigation,
but do not treat a display slug as the only source of branch location.

The venue sort accepted by both `wolt_top` and `wolt_search_venues` is
`recommended`, `distance`, `rating`, `delivery_price`, or `delivery_time`.
The short aliases `fee` → `delivery_price` and `delivery` →
`delivery_time`, plus hyphenated `delivery-price` and `delivery-time`, are
also supported.

## Large grocery catalogs

Some grocery backends return a root assortment with category metadata but no
materialized items. For those venues, `wolt_venue_menu` returns
`data.catalog.status = "partial"`, the available category tree, and an explicit
warning instead of reporting a misleading empty complete menu. Pass a leaf
category slug as `category` to load and hydrate that category in bounded item
batches, or use `wolt_venue_search_items` for a query. The catalog object also
states:

```text
status: "complete" | "partial" | "unavailable"
complete: bool
loading_strategy: string
selection: "full" | "metadata_only" | "category" | "search"
selection_complete: bool
requested_category: string | null
available_categories[]: {
  id, slug, name, parent_slug, level, leaf, item_refs_count
}
loaded_category_slugs[]: string
items_returned: int
```

`complete` describes the entire root catalog. `selection_complete` describes
only the requested category/search selection; a completely hydrated category
can therefore coexist with `complete = false` for a partial grocery root.

## Item, fee, and promotion metadata

Normalized menu, item-search, and item-detail rows retain venue ID, venue slug,
canonical venue URL, description, category, price
amount/currency/formatted amount, primary image, and unit/weight metadata
whenever Wolt or the resolved venue context provides them. The exact semantics
of `unit_info`, `unit_price`,
`sell_by_weight_config`, and `purchasable_balance` (including `0` and `null`)
are documented in [the output contract](./output-contract.md).

Venue detail exposes delivery fee and order-minimum fields with source-aware
nulls. Fees that only exist at checkout, such as service fee or minimum-order
surcharge, remain null at venue level and name `checkout_preview` as their
source. Promotion entries preserve Wolt's text plus raw structured
`conditions` and `effects`; `conditions_available` is false when Wolt supplies
only display copy. Checkout preview keeps Wolt's structured checkout rows,
delivery configurations, offers, and tip configuration.

## MCP result contract

Successful tool calls keep the complete typed result in
`structuredContent`; by default, `content` contains only the short summary so
the full JSON is not duplicated.

Clients that read only `content` can opt into the SDK's serialized-JSON
compatibility duplicate two ways:

- **Per server** — start `wolt-mcp` with `WOLT_MCP_DUPLICATE_CONTENT=1`. Use this
  when the client cannot be configured to send request metadata.
- **Per request** — set `_meta["wolt/duplicate_content"]`. An explicit `true` or
  `false` overrides the server default in either direction.

Duplication roughly doubles response size, so leave it off unless a client needs
it. Ordinary tool errors keep
`structuredContent` unset so they cannot violate a tool's success schema, retain
the short message in `content`, and expose the following stable shape in
`_meta.wolt_error`:

```json
{
  "code": "RATE_LIMITED",
  "message": "wolt is rate-limiting requests; retry after 2s",
  "retryable": true,
  "retry_after_ms": 2000
}
```

Authentication expiry, refresh failure, rate limiting, temporary upstream
failure, unsupported endpoints, and outdated-client responses have distinct
codes. Upstream response bodies and request URLs are not copied into normal MCP
errors.

Stable `_meta.wolt_error.code` values include `AUTH_REQUIRED`,
`AUTH_EXPIRED`, `CONFIG_INVALID`, `CONFIG_UNAVAILABLE`,
`SESSION_REFRESH_FAILED`, `FORBIDDEN`, `NOT_FOUND`, `RATE_LIMITED`,
`UPSTREAM_NETWORK_ERROR`, `UPSTREAM_TEMPORARY`,
`UPSTREAM_INVALID_RESPONSE`, `UNSUPPORTED_ENDPOINT`, `CLIENT_OUTDATED`,
`UPSTREAM_REJECTED`, `UPSTREAM_ERROR`, and `TOOL_ERROR`.

`wolt_cart_show` accepts an optional `venue` filter (slug, ID, or URL) while
preserving its default all-baskets response. Venue availability enrichment is
best-effort: a basket includes `order_availability` when its venue can be
resolved and loaded, and otherwise remains in the response with a warning.
When available, `scheduled_only` identifies venues where Wolt currently allows
scheduled delivery but not immediate ordering. Checkout blockers retain the
basket and return `unavailable_items[]` with item ID, name, and reason.

Checkout preview returns the stable control fields below in addition to Wolt's
raw structured preview in `data`:

```text
status: "ready" | "blocked" | "delivery_mode_unavailable"
requested_delivery_mode: "standard" | "priority"
applied_delivery_mode?: "standard" | "priority"
available_delivery_modes[]: ("standard" | "priority")
selected_delivery_config?: object
basket?: object
unavailable_items[]?: { item_id, name, reason }
error?: { code, message, retryable, retry_after_ms? }
```

`selected_delivery_config` is a concrete Wolt delivery-config object only when
the response marks one selected; it never duplicates the full preview.

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

The MCP server requests one [BCP-47](https://www.rfc-editor.org/rfc/rfc5646)
locale per session. For the best item-search results, set it to the language
selected in the user's Wolt profile and search using that language.

Locale is a request preference, not a guarantee that every Wolt endpoint
returns the same translation. Discovery, assortment search, menu, and item-page
backends can choose different venue or catalog languages. Consequently,
`wolt_venue_search_items`, `wolt_venue_menu`, and `wolt_venue_item` may return
different upstream-provided names for the same product. The MCP server does not
invent or machine-translate missing variants; fields such as `translations`,
`original_name`, or `name_<language>` are included only when that endpoint
actually provides them.

The locale flows to two places in each upstream Wolt request:

- the `app-language` HTTP header
- the `language` query parameter where the endpoint supports it (the
  assortment language for `wolt_venue_search_items` is derived from the part
  before the `-`)

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

Run `wolt login` in any terminal, then retry the tool. The running server reads
the shared profile on every authenticated call, so a restart is not required
after login. Restart the MCP client only when changing its server configuration
or when the client itself does not reconnect to a failed process.

### Client doesn't see the server

- Confirm `wolt-mcp` is on the path with `command -v wolt-mcp` on macOS/Linux,
  `Get-Command wolt-mcp` in PowerShell, or `where.exe wolt-mcp` in Command
  Prompt. If your MCP client does not inherit the shell path, use an absolute
  command such as `/opt/homebrew/bin/wolt-mcp` or
  `C:\Users\you\go\bin\wolt-mcp.exe`.
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

- Built on `github.com/modelcontextprotocol/go-sdk` v1.6.1 (official MCP Go SDK).
- Stdio transport only in v1 (covers every desktop client).
- The server is in-process — `cmd/wolt-mcp/main.go` wires the shared gateway and
  service packages into MCP-specific typed handlers in `internal/mcpserver/`.
- Source: [`cmd/wolt-mcp/`](https://github.com/mekedron/wolt-cli/tree/main/cmd/wolt-mcp) and [`internal/mcpserver/`](https://github.com/mekedron/wolt-cli/tree/main/internal/mcpserver).
