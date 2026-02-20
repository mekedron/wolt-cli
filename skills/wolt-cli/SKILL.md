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
  - `profile favorites add`, `profile favorites remove`
  - `profile addresses add`, `profile addresses update`, `profile addresses remove`, `profile addresses use`
  - `configure` (writes local profile credentials)
- Never describe `checkout preview` as order placement. The CLI does not place final orders.

## Auth Workflow

Use explicit profile names to avoid ambiguity:

```bash
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>" --overwrite
wolt profile status --profile default --format json --verbose
```

Credential fallback for authenticated commands:

1. Explicit flags (`--wtoken`, `--wrtoken`, `--cookie`)
2. Selected profile auth fields
3. Default profile auth fields

When refresh credentials are available, expired/401 access tokens are refreshed automatically and persisted back to local config.

## Location Rules

Apply exactly:

- Use either `--address "<text>"` or both `--lat` + `--lon`.
- Do not combine `--address` with `--lat/--lon`.
- If no override is passed, profile location is used.
- `search venues/items` and `venue show/hours` use `--address` or profile location (no direct `--lat/--lon` flags).
- `discover`, `cart`, `checkout preview`, and `profile favorites` support `--lat/--lon`.

## Command Selection

- Explore nearby options: `discover feed`, `discover categories`, `search venues`, `search items`
- Inspect one venue deeply: `venue show`, `venue categories`, `venue search`, `venue menu`, `venue hours`
- Resolve one item/options for basket actions: `item show`, `item options`
- Basket and pricing: `cart count/show/add/remove/clear`, then `checkout preview`
- Account and history: `profile show/status/orders/payments/addresses/favorites`

For large marketplace venues, prefer:

- `venue search <slug> --query "<text>"`
- `venue menu <slug> --category <category-slug>`

instead of unrestricted full-catalog menu crawl.

## Output and Diagnostics

- `--format json|yaml` returns envelope keys: `meta`, `data`, `warnings`, optional `error`.
- On upstream failures, rerun with `--verbose` to capture request trace and detailed diagnostics.

## References

- Full command and flag matrix: `references/command-reference.md`
- Reusable high-confidence workflows: `references/workflows.md`
- Envelope/error parsing and automation notes: `references/output-and-errors.md`
