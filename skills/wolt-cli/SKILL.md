---
name: wolt-cli
description: Use Nikita's local Wolt CLI to browse venues, inspect menus/items/options, and run profile, cart, and checkout-preview actions for wolt.com from terminal. Trigger when asked to find food on Wolt, inspect venue catalogs, resolve item/option IDs, automate basket or profile tasks, or debug Wolt auth/location/output behavior.
---

# Wolt CLI

Tool repository: https://github.com/mekedron/wolt-cli

Open the repository for setup/build details, then use the local `wolt` binary:

```bash
wolt <group> <command> [flags]
```

## Session Startup

1. Inspect command tree once per session:
```bash
wolt --help
```
2. Prefer machine output for agent work:
```bash
... --format json
```
3. Parse `.data` from the envelope and surface `.warnings`/`.error` to the user.

## Safety Rules

- Start read-only by default.
- Request explicit confirmation before mutating commands:
  - `cart add`, `cart remove`, `cart clear`
  - `account favorites add`, `account favorites remove`
  - `account addresses add`, `account addresses update`, `account addresses remove`, `account addresses use`
  - `login` (opens browser or saves manual credentials)
- Never describe `checkout preview` as order placement. The CLI does not place final orders.

## Auth Workflow

Use the single saved account:

```bash
wolt login                                      # browser-driven (managed Chrome at 127.0.0.1:9222)
wolt login --wtoken "<jwt>" --wrtoken "<rt>"    # manual tokens
wolt status --format json --verbose
```

`wolt login` (no flags) opens the Wolt login page in managed Chrome and polls
the Chrome DevTools cookie store every ~1.5s. It returns only after the user
has actually signed in (a real `__wtoken` cookie is set); telemetry cookies
do not satisfy the polling guard. Default timeout is 2 minutes.

When refresh credentials are available, missing, expired, or rejected access
tokens are refreshed automatically. The refreshed access token is saved only
when the complete persisted credential snapshot is unchanged, so a concurrent
explicit login or logout wins. Any refresh token returned by the rotation
endpoint remains process-local; the saved bootstrap refresh token and cookies
stay pinned.

## Location Rules

Apply exactly:

- Use either `--address "<text>"` or both `--lat` + `--lon`.
- Do not combine `--address` with `--lat/--lon`.
- If no override is passed, current account location is used.
- `venues` and `venue` use `--address` or current account location (no direct `--lat/--lon` flags).
- `venue hours` reads venue-local static data and does not require a location.
- `venues`, `cart`, `checkout`, and `account favorites` support `--lat/--lon`.

## Command Selection

- "What should I eat right now?" — `top [N]` returns a single ranked
  table flattened across all curated venue sections.
- "What's on the home page?" — `feed --summary` prints one line per
  section (`title · kind · count · top items`). Use full `feed` only
  when you need per-section detail.
- Discover with context (home-page style, grouped sections,
  sub-3-second): `feed`. Sections classify as `kind: "venues"` or
  `kind: "brands"`; brand carousels render as a single one-line summary.
- Flat list / filtered search across all nearby venues: `venues`,
  `venues categories` (paginated).
- Inspect one venue deeply: `venue`, `venue categories` (paginated),
  `venue menu`, `venue hours`.
- Resolve one item/options for basket actions: `venue item`.
- Basket and pricing: `cart count/add/remove/clear`, then `checkout`.
- Account and history: `account`, `status`, `account
  orders/payments/addresses/favorites` (favorites is paginated).

Prefer `top` over manually parsing `feed` for "I'm hungry"-style
queries. Prefer `feed --summary` when the user wants an overview rather
than a long list. `venues` is the right tool when the user already has
a search term or filter in mind. Each venue row in `feed`/`top`/`venues`
carries `tagline`, `top_offer`, `badges` (icon-bearing badges from
upstream `badges_v2`), and `menu_highlights` (flagship dishes from
upstream `venue_preview_items`) — all sourced from the same upstream
call.

For large marketplace venues, prefer:

- `venue menu <slug> --query "<text>"`
- `venue menu <slug> --category <category-slug>`

instead of unrestricted full-catalog menu crawl.

## Output and Diagnostics

- `--format json|yaml` returns envelope keys: `meta`, `data`, `warnings`, optional `error`.
- On upstream failures, rerun with `--verbose` to capture request trace and detailed diagnostics.

## Driving Wolt via MCP

If you're an AI agent running inside a host that speaks the Model Context
Protocol (Claude Desktop, Claude Code, Cursor, …), prefer the `wolt-mcp`
server tools (`wolt_feed`, `wolt_top`, `wolt_cart_show`, `wolt_cart_add`,
`wolt_checkout_preview`, …) over shelling out to the CLI. They share the same
auth state (`~/.wolt/.wolt-config.json`) and return typed, schema-described
JSON instead of human-formatted tables. See `../../docs/mcp.md` for the full
catalog and client setup.

## References

- Full command and flag matrix: `references/command-reference.md`
- Reusable high-confidence workflows: `references/workflows.md`
- Envelope/error parsing and automation notes: `references/output-and-errors.md`
- MCP server tool catalog and setup: `../../docs/mcp.md`
