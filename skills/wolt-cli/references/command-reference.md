# Command Reference

## Invocation

Tool repository: https://github.com/mekedron/wolt-cli

Open the repository for setup/build details, then use the local binary:

```bash
wolt <group> <command> [flags]
```

Leaf commands share global flags unless noted:

- `--format table|json|yaml`
- `--profile <name>`
- `--address "<text>"`
- `--locale <bcp47>`
- `--no-color`
- `--wtoken <token>`
- `--wrtoken <refresh-token>`
- `--cookie <name=value>` (repeatable)
- `--verbose`

`configure` uses its own flags and writes local profile auth config.

## Root Groups

- `auth`
- `cart`
- `checkout`
- `configure`
- `discover`
- `item`
- `profile`
- `search`
- `venue`

## Configure

- `wolt configure --profile-name <name> [--wtoken ...] [--wrtoken ...] [--cookie ...] [--overwrite]`
- Default profile-name is `Default`; pass explicit `--profile-name default` for consistency.

## Auth

- `wolt auth status`
- Equivalent auth probe: `wolt profile status`

## Discover

- `wolt discover feed [--limit <n>] [--wolt-plus] [--address ... | --lat ... --lon ...]`
- `wolt discover categories [--address ... | --lat ... --lon ...]`

## Search

- `wolt search venues [--query <text>] [--sort ...] [--type ...] [--category ...] [--open-now] [--wolt-plus] [--limit <n>] [--offset <n>]`
- `wolt search items --query <text> [--sort ...] [--category ...] [--limit <n>] [--offset <n>]`

## Venue

- `wolt venue show <slug> [--include hours,tags,rating,fees] [--address ...]`
- `wolt venue categories <slug>`
- `wolt venue search <slug> --query <text> [--category <slug>] [--include-options] [--limit <n>]`
- `wolt venue menu <slug> [--category <slug>] [--full-catalog] [--include-options] [--limit <n>]`
- `wolt venue hours <slug> [--timezone <iana>] [--address ...]`

## Item

- `wolt item show <venue-slug> <item-id> [--include-upsell]`
- `wolt item options <venue-slug> <item-id>`

`item options` returns `example_option` values in `group-id=value-id` format suitable for `cart add --option`.

## Cart

- `wolt cart count`
- `wolt cart show [--venue-id <id>] [--details] [--address ... | --lat ... --lon ...]`
- `wolt cart add <venue-id> <item-id> [--count <n>] [--option <group-id=value-id[:count]> ...] [--allow-substitutions] [--name ...] [--price ...] [--currency ...] [--venue-slug <slug>] [--lat ... --lon ...]`
- `wolt cart remove <item-id> [--count <n>] [--all] [--venue-id <id>] [--address ... | --lat ... --lon ...]`
- `wolt cart clear [--venue-id <id>] [--all] [--address ... | --lat ... --lon ...]`

If multiple baskets exist and no `--venue-id` is passed, commands select the first basket.

## Checkout

- `wolt checkout preview [--delivery-mode standard|priority|schedule] [--tip <minor-units>] [--promo-code <id>] [--venue-id <id>] [--address ... | --lat ... --lon ...]`

Preview only. No final order placement.

## Profile

- `wolt profile show [--include personal,settings]`
- `wolt profile status`
- `wolt profile orders [--limit 1-50] [--page-token <token>] [--status <value>]`
- `wolt profile orders list [--limit 1-50] [--page-token <token>] [--status <value>]`
- `wolt profile orders show <purchase-id>`
- `wolt profile payments [--label <contains>] [--mask-sensitive]`
- `wolt profile addresses [--active-only]`
- `wolt profile addresses add --address ... --lat ... --lon ... [--type ...] [--label ...] [--alias ...] [--detail key=value ...] [--set-default-profile]`
- `wolt profile addresses update <address-id> --address ... --lat ... --lon ... [--type ...] [--label ...] [--alias ...] [--detail key=value ...] [--set-default-profile]`
- `wolt profile addresses remove <address-id>`
- `wolt profile addresses use <address-id>`
- `wolt profile addresses links [address-id]`
- `wolt profile favorites [--address ... | --lat ... --lon ...]`
- `wolt profile favorites list [--address ... | --lat ... --lon ...]`
- `wolt profile favorites add <venue-id-or-slug>`
- `wolt profile favorites remove <venue-id-or-slug>`

Aliases:

- `wolt profile favorites` alias: `favourites`
- `wolt profile orders` aliases: `history`, `order-history`
