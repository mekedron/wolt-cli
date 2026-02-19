# Wolt CLI v1 Documentation (Design)

## Scope

This documentation defines the **v1 command interface** for a new `wolt` CLI surface.
Implementation is intentionally deferred. The goal is a complete command contract that can
be implemented incrementally.

Status on February 19, 2026:
- Current shipped binary: `wolt-cli`
- Current shipped commands: `configure`, `discover`, `search`, `venue`, `item`
- This spec introduces a future primary binary: `wolt`
- Compatibility requirement: `wolt-cli` remains an alias for the same command tree

## Current Implementation Snapshot

Current code in `/Users/nikita/Projects/wolt-cli/cmd/wolt-cli/main.go` supports:
- `wolt-cli configure`: profile and location configuration
- `wolt-cli discover`: discovery feed and categories
- `wolt-cli search`: venue and item search
- `wolt-cli venue`: venue details, menu, and hours
- `wolt-cli item`: item details by venue slug and item id

Current docs remain valid for the shipped behavior:
- `/Users/nikita/Projects/wolt-cli/docs/cli-discovery-search.md`
- `/Users/nikita/Projects/wolt-cli/docs/cli-venue-item.md`

## v1 Command Taxonomy

Root interface:

```console
wolt <group> <command> [flags]
```

Alias:

```console
wolt-cli <group> <command> [flags]
```

Command groups in v1:
- `auth`
- `discover`
- `search`
- `venue`
- `item`
- `cart`
- `checkout`
- `orders`
- `profile`

## Global Flags (Inherited By Every Command)

All commands inherit these global flags:
- `--format [table|json|yaml]` (default: `table`)
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

Machine-readable formats are mandatory for every command:
- `--format json`
- `--format yaml`

## Safety Model

v1 safety guarantees:
- `checkout` commands are **read/projection operations only** (`preview`, `delivery-modes`, `quote`)
- No `order place` command in this phase
- Any command that mutates baskets must require explicit target parameters and return mutation summaries

## Relationship To Existing CLI

Integration strategy with current implementation:
- Keep existing shipped commands operational during transition
- Add compatibility mapping docs so users can migrate gradually
- Preserve profile-based location behavior from current config model

Compatibility mapping (planned):
- `wolt-cli configure` -> `wolt profile setup` (future, not in v1 command set yet)
- `wolt-cli discover` -> `wolt discover *`
- `wolt-cli search` -> `wolt search *`

## Observed Wolt Endpoint Families (Design Input)

Observed on February 19, 2026 from authenticated web flows:
- Search: `POST https://restaurant-api.wolt.com/v1/pages/search`
- Venue static/dynamic: `.../order-xp/web/v1/pages/venue/slug/<slug>/static`, `.../dynamic`
- Menu/content: `.../consumer-assortment/...`, `.../venue-content-api/...`
- Item details: `.../order-xp/web/v1/pages/venue/<venue_id>/item/<item_id>`
- Basket: `.../order-xp/web/v1/pages/baskets`, `POST .../order-xp/v1/baskets`
- Checkout projection: `POST .../order-xp/web/v2/pages/checkout`
- Order history: `GET .../order-tracking-api/v1/order_history`, `.../purchase/<id>`
- Payment methods context: `POST https://payment-service.wolt.com/v1/payment-methods/checkout`

These endpoints are implementation guidance only and may change without notice.

## Quick Start Examples (Design Target)

```console
wolt search venues --query burger --format table
wolt search venues --query burger --format json
wolt venue menu burger-king-finnoo --include-options --format yaml
wolt cart add 629f1f18480882d6f02c25f0 676939cb70769df4cec6cc6f --count 1 --option size=large --format json
wolt checkout quote --delivery-mode standard --address-id 6916f0f4cbcd388b5e76b8d7 --format yaml
wolt orders list --limit 20 --format json
```
