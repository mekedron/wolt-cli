# Cart and Checkout Commands

All commands in this document support:
- `--format json`
- `--format yaml`

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

Safety boundary:
- v1 checkout commands do **not** place orders.
- `preview`, `delivery-modes`, and `quote` only return pricing/projection data.

## wolt cart show

Synopsis:

```console
wolt cart show [--venue-id <id>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--venue-id`: show basket scoped to one venue

Output schema:
- `CartState`

Examples:

```console
wolt cart show --format json
wolt cart show --venue-id 629f1f18480882d6f02c25f0 --format yaml
```

## `wolt cart add <venue-id> <item-id>`

Synopsis:

```console
wolt cart add <venue-id> <item-id> [--count <n>] [--option <group=value>...] [--allow-substitutions] [global flags]
```

Arguments:
- `<venue-id>`: venue identifier
- `<item-id>`: menu item identifier

Options:
- `--format [json|yaml]`: machine-readable output
- `--count`: quantity, default `1`
- `--option <group=value>`: repeatable option selector
- `--allow-substitutions`: allow substitutions for unavailable items

Output schema:
- `CartMutationResult`

Examples:

```console
wolt cart add 629f1f18480882d6f02c25f0 676939cb70769df4cec6cc6f --count 1 --option drink=coke --format json
wolt cart add 629f1f18480882d6f02c25f0 676939cb70769df4cec6cc6f --allow-substitutions --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_cart_add_001",
    "generated_at": "2026-02-19T21:25:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "basket_id": "69974cfd285af11962f0f8ab",
    "venue_id": "629f1f18480882d6f02c25f0",
    "mutation": "add",
    "line_id": "line_001",
    "total_items": 1,
    "total": {
      "amount": 1595,
      "formatted_amount": "€15.95"
    }
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_cart_add_001
  generated_at: "2026-02-19T21:25:00Z"
  profile: default
  locale: en-FI
data:
  basket_id: "69974cfd285af11962f0f8ab"
  venue_id: "629f1f18480882d6f02c25f0"
  mutation: add
  line_id: line_001
  total_items: 1
  total:
    amount: 1595
    formatted_amount: "€15.95"
warnings: []
```

## `wolt cart update <line-id>`

Synopsis:

```console
wolt cart update <line-id> [--count <n>] [--option <group=value>...] [global flags]
```

Arguments:
- `<line-id>`: basket line identifier

Options:
- `--format [json|yaml]`: machine-readable output
- `--count`: new quantity
- `--option <group=value>`: replace or amend line selections

Output schema:
- `CartMutationResult`

Examples:

```console
wolt cart update line_001 --count 2 --format json
wolt cart update line_001 --option side=fries --format yaml
```

## `wolt cart remove <line-id>`

Synopsis:

```console
wolt cart remove <line-id> [--yes] [global flags]
```

Arguments:
- `<line-id>`: basket line identifier

Options:
- `--format [json|yaml]`: machine-readable output
- `--yes`: skip confirmation prompt

Output schema:
- `CartMutationResult`

Examples:

```console
wolt cart remove line_001 --yes --format json
wolt cart remove line_001 --format yaml
```

## wolt cart clear

Synopsis:

```console
wolt cart clear [--yes] [--venue-id <id>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--yes`: skip confirmation prompt
- `--venue-id`: clear specific venue basket only

Output schema:
- `CartMutationResult`

Examples:

```console
wolt cart clear --yes --format json
wolt cart clear --venue-id 629f1f18480882d6f02c25f0 --format yaml
```

## wolt checkout preview

Synopsis:

```console
wolt checkout preview [--delivery-mode <standard|priority|schedule>] [--address-id <id>] [--tip <minor-units>] [--promo-code <code>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--delivery-mode`: selected checkout mode
- `--address-id`: delivery address identifier
- `--tip`: tip amount in minor units
- `--promo-code`: promo code token

Output schema:
- `CheckoutPreview`

Examples:

```console
wolt checkout preview --delivery-mode standard --address-id 6916f0f4cbcd388b5e76b8d7 --format json
wolt checkout preview --delivery-mode priority --tip 200 --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_checkout_preview_001",
    "generated_at": "2026-02-19T21:30:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "payable_amount": {
      "amount": 1707,
      "formatted_amount": "€17.07"
    },
    "checkout_rows": [
      {
        "label": "Item subtotal",
        "amount": {
          "amount": 1595,
          "formatted_amount": "€15.95"
        }
      }
    ],
    "delivery_configs": [
      {
        "label": "Standard",
        "schedule": "standard",
        "estimate": "25-35 min",
        "additional_fee": {
          "amount": 0,
          "formatted_amount": "€0.00"
        }
      }
    ],
    "offers": {
      "selectable": [],
      "applied": []
    },
    "tip_config": {
      "min_amount": 50,
      "max_amount": 2000,
      "tip_options": [
        {
          "amount": 0,
          "amount_label": "€0"
        }
      ]
    }
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_checkout_preview_001
  generated_at: "2026-02-19T21:30:00Z"
  profile: default
  locale: en-FI
data:
  payable_amount:
    amount: 1707
    formatted_amount: "€17.07"
  checkout_rows:
    - label: Item subtotal
      amount:
        amount: 1595
        formatted_amount: "€15.95"
  delivery_configs:
    - label: Standard
      schedule: standard
      estimate: 25-35 min
      additional_fee:
        amount: 0
        formatted_amount: "€0.00"
  offers:
    selectable: []
    applied: []
  tip_config:
    min_amount: 50
    max_amount: 2000
    tip_options:
      - amount: 0
        amount_label: "€0"
warnings: []
```

## wolt checkout delivery-modes

Synopsis:

```console
wolt checkout delivery-modes [--address-id <id>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--address-id`: delivery address identifier

Output schema:
- `DeliveryModes`

Examples:

```console
wolt checkout delivery-modes --address-id 6916f0f4cbcd388b5e76b8d7 --format json
wolt checkout delivery-modes --format yaml
```

## wolt checkout quote

Synopsis:

```console
wolt checkout quote [--delivery-mode <standard|priority|schedule>] [--address-id <id>] [--tip <minor-units>] [--promo-code <code>] [--payment-method-id <id>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--delivery-mode`: selected checkout mode
- `--address-id`: delivery address identifier
- `--tip`: tip in minor units
- `--promo-code`: promo code
- `--payment-method-id`: payment method identifier

Output schema:
- `CheckoutQuote`

Examples:

```console
wolt checkout quote --delivery-mode standard --address-id 6916f0f4cbcd388b5e76b8d7 --payment-method-id pm_card_01 --format json
wolt checkout quote --delivery-mode priority --tip 100 --format yaml
```

## Integration Notes (Current Implementation)

Observed basket/checkout payload families:
- `POST .../order-xp/v1/baskets`
- `POST .../order-xp/web/v2/pages/checkout`

Current repository has no cart/checkout command implementation yet; this file is the normative contract for upcoming work.
