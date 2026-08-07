# Commands

`wolt-cli` exposes twelve top-level commands. Every leaf command supports
the same machine output (`--format table|json|yaml`) and the same global
flags listed at the bottom of this page.

> **Driving from an AI client?** A typed tool surface is also exposed via the
> bundled `wolt-mcp` server, which speaks the Model Context Protocol
> (Claude Desktop, Claude Code, Cursor, …). See [`mcp.md`](mcp.md) for the
> tool catalog and wiring snippets. The CLI commands below stay the
> authoritative reference for CLI flags and envelopes. MCP handlers share the
> same Wolt gateway and service packages but have their own typed contracts.

| Command | Purpose |
|---|---|
| `wolt login` | Save a Wolt account locally via Chrome cookies or manual tokens. |
| `wolt logout` | Remove saved credentials. |
| `wolt status` | Probe whether the saved account is still authenticated. |
| `wolt account` | Show account profile, orders, addresses, payments, favorites. |
| `wolt feed` | Browse the home-page-style discovery feed grouped by section. |
| `wolt top` | Flatten the feed into a single top-N ranked table. |
| `wolt venues` | Browse or search nearby venues as a flat list. |
| `wolt items` | Search globally ranked items across nearby venues. |
| `wolt venue` | Inspect one venue: details, menu, hours, items, categories. |
| `wolt cart` | Read or mutate the saved basket draft. |
| `wolt checkout` | Preview the checkout payload (no order placement). |
| `wolt stats` | Download the wolt-stats dashboard bundle, sync history, and open the browser. |

---

## `wolt login`

```console
wolt login                                  # opens managed Chrome, extracts cookies
wolt login --wtoken <token> --wrtoken <rt>  # manual tokens
wolt login --cookie "__wtoken=<token>"      # cookie-style auth (repeatable)
```

Without token flags, a managed Chrome window is opened against
`http://127.0.0.1:9222` (see `scripts/start-chrome.sh`). After a successful
sign-in on `https://wolt.com/login`, Wolt cookies are extracted via the
Chrome DevTools Protocol and normalized into `wtoken` / `wrefresh_token`.
The launcher checks `CHROME_BIN`, browsers on `PATH`, and standard Chrome,
Chromium, and Edge install locations on Windows, macOS, and Linux. `--timeout`
bounds browser discovery, DevTools requests, and the complete login wait.
The config is saved with `0600` permissions on Unix; on Windows it relies on
the inherited ACL of the user's profile directory. It is written to:

- `$WOLT_CONFIG_PATH` if set, else `~/.wolt/.wolt-config.json`

`--wtoken` accepts raw JWT, `Bearer <jwt>`, JSON `accessToken` payloads,
URL-encoded payloads, and cookie blobs (`__wtoken=<jwt>`). When upstream
returns `401` later, the CLI auto-rotates the access token with the saved
refresh token. A refresh token can also bootstrap a profile whose access token
is missing. A refreshed access token is persisted only if the complete saved
credential snapshot is unchanged, so a concurrent explicit login or logout
wins. Any refresh token returned by the rotation endpoint remains
process-local; the saved bootstrap refresh token and cookies stay pinned.

## `wolt logout`

```console
wolt logout
```

Clears `wtoken`, `wrefresh_token`, `cookies[]`, and the local
`wolt_address_id` pointer from the saved config. Preserves location
preferences. Does not call any Wolt endpoint.

## `wolt status`

```console
wolt status [--verbose]
```

Calls `GET /v1/user/me`. Returns `authenticated`, `user_id`, `country`,
`session_expires_at`, `wolt_plus_subscriber`. Without credentials, returns
`authenticated=false` with a `no auth credentials provided` warning.
`--verbose` adds a token preview and the upstream HTTP trace.

## `wolt account`

```console
wolt account                                # ProfileSummary
wolt account addresses                      # list saved Wolt addresses
wolt account addresses add --address "..." --lat .. --lon ..
wolt account addresses update <id> --address "..." --lat .. --lon ..
wolt account addresses remove <id>
wolt account addresses use <id>             # set local default pointer
wolt account addresses links [id]           # Google Maps validation URLs
wolt account orders [--limit 1-50] [--page-token <t>] [--status <s>]
wolt account order <purchase-id>            # one order detail
wolt account payments [--mask-sensitive]
wolt account favorites [--limit <n>] [--offset <n> | --page <n>]
wolt account favorites add <venue|slug|url>
wolt account favorites remove <venue|slug|url>
```

All `account *` subcommands require a logged-in session. An expired access
token is refreshed automatically when a saved refresh credential is available.
If refresh is unavailable or the refreshed session is still rejected,
auth-gated commands return a friendly `WOLT_AUTH_REQUIRED` or refresh-specific
error instead of a raw status code. Address mutation endpoints write to
`https://restaurant-api.wolt.com/v2/delivery/info`.

## `wolt feed`

```console
wolt feed [--section-limit <n>] [--per-section <n>]
          [--query <text>]
          [--summary]
          [--show-highlights[=true|false]]
          [--address "<text>" | --lat <f> --lon <f>]
```

Renders the same section structure you see on wolt.com — "Popular near
you", "Order again", "Fastest delivery", "Top-rated", "Popular stores",
"Restaurant categories", etc. — with marketing context per row (tagline,
top discount offer, rating, ETA). One upstream call, no per-venue
enrichment, sub-3-second.

Each row in JSON carries `name`, `slug`, `tagline`, `top_offer`,
`rating`, `delivery_estimate`, `delivery_fee`, `price_range`,
`promotions`, `badges`, `menu_highlights`, `wolt_plus`, plus `venue_id`
for chaining into `cart add` or `venue menu`. Sections carry a
`kind: "venues" | "brands"` discriminant — brand carousels render as a
single-line summary instead of a per-venue table. The default table
shows the action-relevant columns truncated to fit (≤32 chars for
tagline, ≤26 for top offer).

The venue cell is prefixed with single-rune glyphs derived from the
icon-bearing `badges_v2` payload (`+ Wolt+`, `% 20% off`, `⚡ Fast`,
`◷ Schedule`, `★ New`). Set `WOLT_BADGES_PLAIN=1` to fall back to
bracketed-text labels (`[Wolt+]`, `[20% off]`) when your terminal
doesn't render the glyphs cleanly.

`--summary` prints one line per section (`Section · Kind · Count · Top
items`) instead of full per-section tables — useful for getting a quick
glance at what's on the home page right now.

`--per-section` caps the per-section rows shown in the table (default 6);
JSON keeps the full upstream slice. `--query` filters venues by name,
tagline, top-offer, or slug across all sections; in brand sections it
matches against `brands[].name`. Empty sections drop out.

`--show-highlights` defaults to **auto** — the Highlights column
(`menu_highlights[]` from upstream `venue_preview_items`) appears only
when at least one row in the table has data. Pass
`--show-highlights=false` to force-hide it.

## `wolt top`

```console
wolt top [N]                                # default 10
         [--limit <n>] [--offset <n> | --page <n>]
         [--query <text>]
         [--wolt-plus]
         [--show-highlights[=true|false]]
         [--address "<text>" | --lat <f> --lon <f>]
```

Flattens every venue section of the discovery feed into a single ranked
table, dedupes by `venue_id` while preserving upstream order, and trims
to N (default 10). Brand carousels are excluded. The fastest path from
"I'm hungry" to a shortlist — no `jq` required.

```console
wolt top 5
wolt top --query pizza
wolt top --wolt-plus --limit 8
```

Same row shape as `wolt venues`. The same badge-glyph prefix and
auto-mode Highlights column apply.

## `wolt venues`

```console
wolt venues [--query <text>]
            [--type restaurant|grocery|pharmacy|retail]
            [--category <slug>]
            [--sort recommended|distance|rating|delivery_price|delivery_time|delivery|fee]
            [--open-now] [--wolt-plus] [--promotions-only]
            [--min-rating <float>] [--max-delivery-fee <minor>]
            [--limit <n>] [--offset <n> | --page <n>]
            [--show-highlights[=true|false]]
            [--enrich]
wolt venues categories [--limit <n>] [--offset <n> | --page <n>]
```

`--query` filters by venue name or slug. Without `--query`, returns
nearby venues. Default table is 8 columns: Venue, Slug, Tagline,
Top offer, Rating, Delivery, Fee, Wolt+ — the tagline (Wolt
`short_description`) and top discount offer come from the same payload,
no extra HTTP. The venue cell is prefixed with badge glyphs (see
`wolt feed` for the icon → glyph map). JSON keeps the full payload
including `address`, `promotions`, `badges`, `menu_highlights`,
`price_range_scale`, and nullable immediate/scheduled availability signals.
The discovery feed is non-exhaustive; use an exact venue slug, ID, or URL when
the user already knows the venue.

`--sort` accepts the canonical `delivery_time`/`delivery_price` forms,
the short aliases `delivery`/`fee`, and the hyphenated
`delivery-time`/`delivery-price` forms. `delivery` means delivery time;
`fee` means delivery price.

`--show-highlights` defaults to **auto** — the column appears only
when at least one row carries `menu_highlights[]` data. Force-show
with `--show-highlights`; force-hide with `--show-highlights=false`.

**Speed**: by default `venues` does not hit per-venue promotion or
Wolt+ endpoints — one upstream call, sub-second response. Pass
`--enrich` to fetch dynamic campaign banners and resolve Wolt+ for
venues whose flag is missing from the feed payload (slower; bounded by
internal budgets). `--promotions-only` implies `--enrich`.

## `wolt items`

```console
wolt items --query <text> [--limit 1-200] [--available-only]
```

Calls Wolt's item-targeted global search and preserves the upstream relevance
order. The default limit is 20. Machine output contains the complete returned
sample without duplicating product objects: `items[]` is the flat ranked list,
while `venue_groups[]` references those rows by `item_ranks[]` and provides
structured arguments for expanding the same query with
`wolt venue menu <venue> --query <text>`.

The table groups matches by venue and shows up to three products from each
venue; JSON and YAML retain every returned row. `--available-only` removes only
rows explicitly marked unavailable by Wolt. An absent availability signal stays
unknown.

The current upstream endpoint exposes neither a continuation token nor an exact
total and caps a single request at 200. Consequently the result always reports
`completeness: unknown`; `limit_reached` and `upstream_cap_reached` describe the
sample boundary but never claim that every matching Wolt item was returned.

## `wolt venue`

```console
wolt venue <venue> [--include hours,tags,rating,fees]
wolt venue menu <venue> [--query <text>] [--category <slug>]
                        [--include-options] [--full-catalog]
                        [--sort recommended|price|name]
                        [--min-price <minor>] [--max-price <minor>]
                        [--hide-sold-out] [--discounts-only]
                        [--limit <n>] [--offset <n> | --page <n>]
wolt venue categories <venue> [--limit <n>] [--offset <n> | --page <n>]
wolt venue hours <venue> [--timezone <iana>]
wolt venue item <venue> <item-id|url>
wolt venue item <wolt-item-url>              # one-arg: venue read from URL
```

`venue hours` reads the supported static venue-page payload directly and does
not call the legacy `/v3/venues` restaurant endpoint. Output uses the same
`[{day, open, close}]` shape for each upstream-known opening interval. A day
without a returned row has no opening window in the upstream payload; split
shifts produce multiple rows for the same day. `--timezone` is an expected
timezone or fallback label when upstream omits one; it does not convert these
undated venue-local windows.

`<venue>` accepts a slug, a 24-char Mongo ObjectID, or a Wolt URL
(e.g. `https://wolt.com/en/fin/helsinki/venue/<slug>`). The CLI extracts
the slug and looks up the venue id when needed. Detail, menu, item, and search
JSON surface canonical identity fields when the corresponding Wolt payload
provides them; category-list output retains its documented `venue_id` shape.
When choosing between branches, match Wolt's `venue_id` and address rather than
relying on a display slug alone.

`venue menu` without `--query` returns the full menu (`VenueMenu`). With
`--query`, it returns a venue-scoped item search (`VenueItemSearchResult`)
— preferred for large marketplace catalogs.

Some large grocery venues expose a partial root assortment: category metadata
is available, but items must be loaded through category or search endpoints.
Use `venue categories` to inspect leaf slugs, then
`venue menu <venue> --category <slug>`, or use `--query`. A partial or
unavailable backend is reported explicitly rather than represented as a
successful complete menu with zero categories and zero items.

`venue item <venue> <item-id>` shows item detail; `--include-options`
on `venue menu` exposes option-group IDs you can pass to `cart add --option`.

## `wolt cart`

```console
wolt cart [--venue-id <id>] [--details]
wolt cart count
wolt cart add <venue> <item-id|url>
              [--count <n>]
              [--option <group=value[:count]>...]
              [--allow-substitutions]
              [--name <text>] [--price <minor>]
              [--currency <code>]
              [--venue-slug <slug>]
              [--query "<item name>"] [--cheapest]
wolt cart add <wolt-item-url>                # one-arg: venue read from URL
wolt cart add <venue> --query "<item name>"  # resolves to item id via menu search
wolt cart add <venue> --query "burger" --cheapest  # cheapest in-stock match
wolt cart add <venue> --cheapest             # cheapest in-stock item in the venue
wolt cart remove <item-id|url> [--count <n>] [--all] [--venue-id <id>]
wolt cart clear [--venue-id <id>] [--all]
```

`<item-id>` on `cart add` and `cart remove` accepts either a 24-char
Mongo ObjectID or a Wolt item URL of the form
`https://wolt.com/<locale>/<country>/<city>/venue/<slug>/itemid-<id>`
(`menuitem-<id>` and the same URL with `?itemid=<id>` also work). The
slug embedded in the URL is reused for venue resolution when you
haven't passed one explicitly — `cart add` accepts the URL as a single
argument (no separate `<venue>`) since it carries both pieces.

`cart add --query "<text>"` is a one-shot path that calls the same
assortment item search `venue menu --query` uses, requires a single
match, and errors with a "did you mean…" list when more than one item
matches. Exact-name matches always beat substring hits.

`--cheapest` swaps that strict resolution for a deterministic one: it
takes the cheapest in-stock item, skipping sold-out and unpriced rows
rather than erroring on ambiguity. Narrow it with `--query` (cheapest
item whose name contains the text) or use it alone to add the venue's
cheapest in-stock item. Handy for scripting where any orderable item
will do without pinning a volatile item ID.

The basket lives in your Wolt account (same draft you see in the Wolt
sidebar). Mutations call `POST /order-xp/v1/baskets` and the bulk-delete
endpoint; no payment or delivery is dispatched from this CLI.

Wolt's basket mutation replaces the complete item array. The CLI therefore
validates every existing basket line, option, count, and price before posting
and fails without changing the basket when the snapshot is incomplete. Avoid
running cart mutations concurrently from separate CLI processes or the
official Wolt clients: the upstream endpoint exposes no basket revision or
conditional-write token, so independent writers cannot prevent a last-write
win. One MCP server serializes its own cart mutations, but it cannot coordinate
with other processes or apps.

`cart add` first resolves whatever venue you pass (slug, id, or URL) to
its real 24-character venue id before posting — the basket POST only
persists when keyed by that id, not a slug. If the venue can't be
resolved (an unknown slug, or one Wolt no longer serves) `cart add`
fails with `WOLT_VENUE_UNRESOLVED` instead of reporting a success that
never reaches your cart.

It then refreshes the exact current item. A non-null `disabled_info`,
non-positive numeric `purchasable_balance`, or a missing current item blocks
the mutation even when `--price`, `--currency`, and `--name` are supplied.
A null balance means Wolt supplied no numeric balance constraint, not unlimited
stock. Currency is resolved from current item, basket, or venue metadata; the
command fails instead of silently defaulting to EUR. See
[`output-contract.md`](./output-contract.md) for unit, weight-step, and balance
semantics.

`--option` accepts both IDs and case-insensitive names:

```console
wolt cart add huuva-food-court-niittykumpu 689efcc0dbe125482d2fecb2 \
  --option "Drink=Cola" --option "Side=Fries" --count 1
```

When complete option metadata is available, unknown or ambiguous names fail
closed; use the displayed group/value ID to disambiguate. Required and
minimum/maximum selection counts are validated before the mutation, and
explicitly free option values remain distinct from values whose price metadata
is missing.

If `--count` is omitted, the API treats the call as "add one of this
line." `--all` on remove/clear targets every basket the user holds.

## `wolt checkout`

```console
wolt checkout [--delivery-mode standard|priority]
              [--tip <minor>] [--promo-code <id>]
              [--venue-id <id>]
              [--address "<text>" | --lat <f> --lon <f>]
```

Preview-only. Calls `POST /order-xp/web/v2/pages/checkout` and returns
the projected payable amount, checkout rows, delivery configs, and tip
config. Location overrides affect the preview only — real orders use
the Wolt-saved default delivery address.

Priority sets Wolt's `is_priority_delivery` purchase-plan flag. The CLI reports
the requested, applied, and available modes plus the config Wolt advertised for
the applied mode.

Wolt's checkout response lists one `delivery_configs` entry per offered mode,
identified by its `schedule` slug, and marks none of them as selected — the mode
follows from the posted purchase plan. A requested mode is therefore treated as
applied unless the response contradicts it: the CLI returns
`WOLT_DELIVERY_MODE_UNAVAILABLE` (MCP: a structured `DELIVERY_MODE_UNAVAILABLE`
result listing `available_delivery_modes`) only when the response never
advertised that mode, names a different one, or names two at once. Scheduled
ordering is venue availability, not a checkout delivery mode supported by this
endpoint.

There is no order-placement command. To place an order, finish in the
Wolt app or web UI.

---

## `wolt stats`

```console
wolt stats                                  # fetch bundle, sync, serve, open browser
wolt stats --resync                         # force a full history rescan
wolt stats --no-sync                        # skip sync; just open whatever DB is on disk
wolt stats --no-open                        # serve without launching the browser
wolt stats --port 5180                      # use a different localhost port
wolt stats --bundle-version v0.1.0          # pin a specific wolt-stats release
wolt stats --no-check-updates               # offline: do not query GitHub for newer bundle
wolt stats --stats-dir /custom/path         # override default ~/.wolt/stats install dir
```

The single-step flow:

1. **Resolve install dir.** Defaults to `~/.wolt/stats`. Override with
   `--stats-dir` or `$WOLT_STATS_DIR`. Created with mode `0700`.
2. **Ensure bundle.** Queries
   `https://api.github.com/repos/mekedron/wolt-stats/releases/latest`
   (ETag-aware, throttled to once per hour via `state.json`). If a newer
   tarball exists, downloads it, verifies the published SHA256, and
   extracts into `<stats-dir>/bundles/<version>/`. Older versions are kept
   for rollback.
3. **Sync history** (skipped with `--no-sync`). Calls the Wolt API directly
   (no Node, no subprocess) and writes to
   `<stats-dir>/db/wolt-history.sqlite` using the same schema the Node
   `sync-wolt-history.mjs` script writes — the resulting file is
   interchangeable between the two implementations.
4. **Serve.** A `net/http` server binds to `127.0.0.1:<port>` (default
   `5173`; auto-bumps `+1` up to `+4` if busy). The bundle is served at
   `/`; the SQLite file is served at `/data/wolt-history.sqlite` —
   localhost only, never the LAN.
5. **Open browser** unless `--no-open`. Blocks until Ctrl-C, then
   gracefully shuts the server down (exit code `130`).

On-disk layout:

```
~/.wolt/stats/
  state.json                    cached release metadata: active_version, etag, last_checked_at
  bundles/<version>/            extracted dashboard bundle (index.html, _app/, manifest.json, ...)
  db/wolt-history.sqlite        synced order history
```

Failure modes:

- Not logged in → `WOLT_AUTH_REQUIRED` (reuse the same code as the other
  authenticated commands).
- GitHub unreachable AND no cached bundle → `WOLT_STATS_BUNDLE_UNAVAILABLE`.
- Port `5173..5177` all busy → `WOLT_STATS_ENV_ERROR` ("no free port…").
- `user.email` missing from `UserMe` → `WOLT_STATS_ENV_ERROR` with a
  "Run \"wolt login\" again" hint.

Progress output (table mode only):

- Every phase is announced as a `[i/N] Title` banner on stderr.
- The bundle download draws an in-place progress bar with byte counts and
  percentage, falling back to indeterminate when the server omits
  `Content-Length`.
- Sync emits one line per catalog page and a checkpoint every ~5 % of the
  detail queue. Pass `--verbose` to log every individual order id.
- `--format json` / `--format yaml` suppresses all progress — only the
  final envelope lands on stdout, stderr stays clean.

## Global flags

Available on every leaf command:

- `--format table|json|yaml` (default `table`)
- `--address <text>` — temporary address override; cannot combine with `--lat`/`--lon`
- `--locale <bcp-47>` (default `en-FI`)
- `--no-color`
- `--verbose` — prints the upstream HTTP request trace and detailed error envelopes

## On-disk caches

- `~/.wolt/.wolt-config.json` — the saved account.
- `~/.wolt/.wolt-slug-cache.json` — venue slug → id + static-page payload cache, 24 h TTL. Eliminates the ~200–500 ms static-page lookup on repeated `cart add`, `venue menu`, `venue item`, and `checkout` flows against the same venue. Wiped automatically by `wolt logout`. Override the path with `WOLT_SLUG_CACHE_PATH`.

Both files use `0600` permissions on Unix. Windows uses inherited
user-profile/directory ACLs instead of POSIX mode bits. If
`WOLT_CONFIG_PATH` or `WOLT_SLUG_CACHE_PATH` is overridden, point it at a
directory that is private to the current user.

Location-aware commands additionally accept `--lat <float>` and `--lon <float>`
(must be supplied together). If no override is given, the address attached
to the logged-in Wolt account is used.

## Output contract

JSON / YAML responses are wrapped in a stable envelope
(`meta`, `data`, `warnings`, optional `error`). See
[`output-contract.md`](./output-contract.md) for the full schema reference.

## Roadmap

Planned ergonomics improvements (item name / URL resolution, etc.) are
tracked in [`roadmap.md`](./roadmap.md).
