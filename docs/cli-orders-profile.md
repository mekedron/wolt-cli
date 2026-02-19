# Orders and Profile Commands

All commands in this document support:
- `--format json`
- `--format yaml`

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

## wolt orders list

Synopsis:

```console
wolt orders list [--limit <n>] [--cursor <token>] [--status <value>] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--limit`: max number of orders returned
- `--cursor`: pagination cursor
- `--status`: filter by status (`delivered`, `cancelled`, `rejected`, etc.)

Output schema:
- `OrderList`

Examples:

```console
wolt orders list --limit 20 --format json
wolt orders list --status delivered --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_orders_list_001",
    "generated_at": "2026-02-19T21:40:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "next_cursor": "2025-12-03T14:40:50.585Z",
    "orders": [
      {
        "purchase_id": "69917e931d9361b2ffcb241c",
        "received_at": "2026-02-15T10:06:00Z",
        "status": "delivered",
        "venue_name": "Burger King Iso Omena",
        "total_amount": "€15.38",
        "is_active": false
      }
    ]
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_orders_list_001
  generated_at: "2026-02-19T21:40:00Z"
  profile: default
  locale: en-FI
data:
  next_cursor: "2025-12-03T14:40:50.585Z"
  orders:
    - purchase_id: "69917e931d9361b2ffcb241c"
      received_at: "2026-02-15T10:06:00Z"
      status: delivered
      venue_name: Burger King Iso Omena
      total_amount: "€15.38"
      is_active: false
warnings: []
```

## `wolt orders show <purchase-id>`

Synopsis:

```console
wolt orders show <purchase-id> [--include payments,fees,items] [global flags]
```

Arguments:
- `<purchase-id>`: order purchase identifier

Options:
- `--format [json|yaml]`: machine-readable output
- `--include`: comma-separated optional detail blocks

Output schema:
- `OrderDetail`

Examples:

```console
wolt orders show 69917e931d9361b2ffcb241c --include payments,fees,items --format json
wolt orders show 69917e931d9361b2ffcb241c --format yaml
```

## wolt profile show

Synopsis:

```console
wolt profile show [--include personal,settings] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--include`: include additional profile blocks

Output schema:
- `ProfileSummary`

Examples:

```console
wolt profile show --include personal,settings --format json
wolt profile show --format yaml
```

## wolt profile addresses

Synopsis:

```console
wolt profile addresses [--active-only] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--active-only`: filter to currently active/usable addresses

Output schema:
- `AddressList`

Examples:

```console
wolt profile addresses --active-only --format json
wolt profile addresses --format yaml
```

## wolt profile payments

Synopsis:

```console
wolt profile payments [--mask-sensitive] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--mask-sensitive`: hide full labels for sensitive payment metadata

Output schema:
- `PaymentMethodList`

Examples:

```console
wolt profile payments --mask-sensitive --format json
wolt profile payments --format yaml
```

JSON example:

```json
{
  "meta": {
    "request_id": "req_profile_payments_001",
    "generated_at": "2026-02-19T21:45:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "methods": [
      {
        "method_id": "pm_card_01",
        "type": "card",
        "label": "Visa •••• 4242",
        "is_default": true,
        "is_available_for_checkout": true
      }
    ]
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_profile_payments_001
  generated_at: "2026-02-19T21:45:00Z"
  profile: default
  locale: en-FI
data:
  methods:
    - method_id: pm_card_01
      type: card
      label: Visa •••• 4242
      is_default: true
      is_available_for_checkout: true
warnings: []
```

## Integration Notes (Current Implementation)

Current `wolt-cli configure` stores local profiles with address and geocoded
location in `~/.wolt-cli/.wolt-cli-config.json`. New `wolt profile` commands
must coexist with this local profile concept.
