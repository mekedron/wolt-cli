# Command Reference

## Invocation

Tool repository: https://github.com/mekedron/wolt-cli

Open the repository for setup/build details, then use the local binary:

```bash
wolt <group> <command> [flags]
```

> If you are an agent running inside an MCP host (Claude Desktop, Claude
> Code, Cursor, …), prefer the typed tools served by the bundled `wolt-mcp`
> binary over shelling to the CLI. It shares the saved auth state, Wolt gateway,
> and service packages, with MCP-specific typed handlers instead of CLI
> envelopes or tables.
> Tool surface: `wolt_feed`, `wolt_top`,
> `wolt_search_venues`, `wolt_venue_categories`, `wolt_resolve_address`,
> `wolt_resolve_venue`, `wolt_venue_detail`, `wolt_venue_menu`, `wolt_venue_hours`,
> `wolt_venue_item`, `wolt_venue_search_items`, `wolt_account_*`,
> `wolt_favorites_*`, `wolt_cart_*`, `wolt_checkout_preview`. Full catalog
> and per-client wiring in [`docs/mcp.md`](../../../docs/mcp.md).

Leaf commands share global flags unless noted:

- `--format table|json|yaml`
- `--address "<text>"`
- `--locale <bcp47>`
- `--no-color`
- `--verbose`

`login` is the only credential setup command. It can open managed Chrome or accept manual token flags.

## Root Groups

- `login`
- `logout`
- `status`
- `account`
- `feed`
- `top`
- `venues`
- `venue`
- `cart`
- `checkout`
- `stats`

## Login

- `wolt login`
- `wolt login [--wtoken ...] [--wrtoken ...] [--cookie ...]`
- `wolt logout`
- `wolt status`

## Feed

- `wolt feed [--section-limit <n>] [--per-section <n>] [--query <text>] [--summary] [--show-highlights[=bool]] [--address ... | --lat ... --lon ...]`

Mirrors the wolt.com home page: section-grouped venues with tagline + top discount offer per row. One upstream call, sub-3-second. Sections carry a `kind: "venues" | "brands"` discriminant — brand carousels (Popular stores, Restaurant categories, …) render as a single-line summary. Use `--summary` to collapse the whole feed into one line per section. `--show-highlights` defaults to auto (render iff at least one row has `menu_highlights[]`). `--query` matches against brand names too.

## Top

- `wolt top [N] [--limit <n>] [--offset <n> | --page <n>] [--query <text>] [--wolt-plus] [--show-highlights[=bool]] [--address ... | --lat ... --lon ...]`

Flattens every `kind=venues` section of the discovery feed into a single ranked table, dedupes by `venue_id` preserving upstream order, and trims to N (default 10). The "what should I order right now" shortcut. Same row shape as `wolt venues`.

## Venues

- `wolt venues [--query <text>] [--sort ...] [--type ...] [--category ...] [--open-now] [--wolt-plus] [--promotions-only] [--min-rating <float>] [--max-delivery-fee <minor>] [--enrich] [--show-highlights[=bool]] [--limit <n>] [--offset <n> | --page <n>] [--address ... | --lat ... --lon ...]`

By default `venues` skips per-venue promotion/Wolt+ enrichment (single upstream call, sub-second). Add `--enrich` to fetch dynamic campaign banners and resolve missing Wolt+ flags (slower; capped by internal budget). `--promotions-only` implies `--enrich`. `--sort` accepts canonical `delivery_time`/`delivery_price`, short `delivery`/`fee`, and hyphenated `delivery-time`/`delivery-price` forms.

Discovery is ranked and non-exhaustive. In MCP workflows, use
`wolt_resolve_venue` for an exact name, slug, venue ID, or URL, especially for
closed or scheduled-order venues.

- `wolt venues categories [--limit <n>] [--offset <n> | --page <n>] [--address ... | --lat ... --lon ...]`

## Venue

`<venue>` accepts slug, 24-char Mongo ObjectID, or a Wolt URL.

- `wolt venue <venue> [--include hours,tags,rating,fees] [--address ...]`
- `wolt venue categories <venue> [--limit <n>] [--offset <n> | --page <n>]`
- `wolt venue menu <venue> [--query <text>] [--category <slug>] [--full-catalog] [--include-options] [--sort recommended|price|name] [--min-price <minor>] [--max-price <minor>] [--hide-sold-out] [--discounts-only] [--limit <n>] [--offset <n> | --page <n>]`
- `wolt venue hours <venue> [--timezone <iana>]` — reads venue-local opening windows directly from the supported static venue-page payload without calling the legacy `/v3/venues` endpoint.
- `wolt venue item <venue> <item-id|url>` (or `wolt venue item <wolt-item-url>` for the single-arg form)

`venue menu` without `--query` returns the full menu; with `--query` it returns a venue-scoped item search (preferred for large marketplace catalogs). Partial grocery roots expose category metadata instead of pretending an empty menu is complete; select a leaf category or use item search. Search in the language selected in the user's Wolt profile. Wolt's search, menu, and item endpoints can return different upstream-provided languages; the client never invents missing translations. `venue item` includes option metadata so option group/value names can be passed straight to `cart add --option`. Unit, weight-step, and `purchasable_balance` semantics are documented in [`output-contract.md`](../../../docs/output-contract.md).

## Cart

- `wolt cart count`
- `wolt cart [--venue-id <id>] [--details] [--address ... | --lat ... --lon ...]`
- `wolt cart add <venue> <item-id|url> [--count <n>] [--option <group=value[:count]> ...] [--allow-substitutions] [--name ...] [--price ...] [--currency ...] [--venue-slug <slug>] [--lat ... --lon ...]`
- `wolt cart add <wolt-item-url>` (single-arg: venue slug read from the URL)
- `wolt cart add <venue> --query "<item name>"` (resolves a unique item by name via the venue menu search; errors on ambiguous matches)
- `wolt cart remove <item-id|url> [--count <n>] [--all] [--venue-id <id>] [--address ... | --lat ... --lon ...]`
- `wolt cart clear [--venue-id <id>] [--all] [--address ... | --lat ... --lon ...]`

`<venue>` accepts slug, hex ID, or Wolt URL (same as `venue`). `<item-id>` on `cart add`/`cart remove` and `venue item` accepts a 24-char Mongo ObjectID or a Wolt item URL (`.../venue/<slug>/itemid-<id>`, `menuitem-<id>`, or `?itemid=<id>`). `--option` accepts both IDs and case-insensitive names (e.g. `--option "Drink=Cola"`). With complete option metadata, unknown or ambiguous names are rejected; use the displayed ID to disambiguate. If multiple baskets exist and no `--venue-id` is passed, commands select the first basket.

## Checkout

- `wolt checkout [--delivery-mode standard|priority] [--tip <minor-units>] [--promo-code <id>] [--venue-id <id>] [--address ... | --lat ... --lon ...]`

Preview only. No final order placement. Priority sets the Wolt purchase-plan
flag. Offered modes come from Wolt's `delivery_configs` (keyed on each entry's
`schedule` slug, not its localized label). The CLI returns requested, applied,
and available modes plus the config for the applied mode, and reports
`WOLT_DELIVERY_MODE_UNAVAILABLE` only when the requested mode was not offered,
a different one was named, or two were named at once. Scheduled ordering is venue
availability, not a checkout delivery mode supported by the current endpoint.

## Stats

- `wolt stats [--resync] [--no-sync] [--no-open] [--port <n>] [--bundle-version <tag>] [--no-check-updates] [--stats-dir <path>]`

Downloads or reuses the `wolt-stats` dashboard bundle, optionally syncs order
history into a local SQLite database, and serves the dashboard on localhost.
See [`docs/stats.md`](../../../docs/stats.md) for the sync, storage, privacy,
and lifecycle contracts.

## Account

- `wolt account [--include personal,settings]`
- `wolt account orders [--limit 1-50] [--page-token <token>] [--status <value>]`
- `wolt account order <purchase-id>`
- `wolt account payments [--label <contains>] [--mask-sensitive]`
- `wolt account addresses [--active-only]`
- `wolt account addresses add --address ... --lat ... --lon ... [--type ...] [--label ...] [--alias ...] [--detail key=value ...] [--set-default-profile]`
- `wolt account addresses update <address-id> --address ... --lat ... --lon ... [--type ...] [--label ...] [--alias ...] [--detail key=value ...] [--set-default-profile]`
- `wolt account addresses remove <address-id>`
- `wolt account addresses use <address-id>`
- `wolt account addresses links [address-id]`
- `wolt account favorites [--limit <n>] [--offset <n> | --page <n>] [--address ... | --lat ... --lon ...]`
- `wolt account favorites add <venue-id-or-slug>`
- `wolt account favorites remove <venue-id-or-slug>`
