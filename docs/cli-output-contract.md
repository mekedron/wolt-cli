# CLI Output Contract

This document defines the stable output shape for machine-readable responses used by
all `wolt` v1 commands.

## Formats

Supported output formats:
- `table` (default, human-readable)
- `json`
- `yaml`

Every command must support:
- `--format json`
- `--format yaml`

## Envelope

For `json` and `yaml`, the response envelope is mandatory:

```json
{
  "meta": {
    "request_id": "req_01j0zdq8q6k7y8d6w2g0y9p4m7",
    "generated_at": "2026-02-19T20:45:09Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {},
  "warnings": []
}
```

YAML equivalent:

```yaml
meta:
  request_id: req_01j0zdq8q6k7y8d6w2g0y9p4m7
  generated_at: "2026-02-19T20:45:09Z"
  profile: default
  locale: en-FI
data: {}
warnings: []
```

## Field Conventions

- IDs: string identifiers from upstream APIs (`venue_id`, `item_id`, `basket_id`)
- Money:
  - `amount` in minor units (for example cents)
  - optional `formatted_amount` string for display
- Time:
  - use ISO-8601 UTC by default (`generated_at`, timestamps)
  - if upstream only provides localized strings, include both when possible
- Booleans should never be encoded as strings

## Error Object

When a command fails, `data` may be null and `error` must be present:

```json
{
  "meta": {
    "request_id": "req_01j0ze0cbm78jwry8x4v6x0g8t",
    "generated_at": "2026-02-19T20:46:02Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": null,
  "warnings": [],
  "error": {
    "code": "WOLT_AUTH_REQUIRED",
    "message": "Authentication is required for this command.",
    "details": {
      "hint": "Provide --wtoken/--cookie or configure profile credentials"
    }
  }
}
```

Error fields:
- `code` (string, stable machine key)
- `message` (human-readable)
- `details` (object, optional)

## Canonical Schema Types (Implemented Commands)

### AuthStatus (`auth status`, `profile status`)
Required:
- `authenticated`
- `user_id`
- `country`
- `session_expires_at`

Optional:
- `token_preview` (when `--verbose`)
- `cookie_count` (when `--verbose`)

### DiscoveryFeed (`discover feed`)
Required:
- `city`
- `sections[]`

### CategoryList (`discover categories`)
Required:
- `categories[]:{id,name,slug}`

### VenueSearchResult (`search venues`)
Required:
- `query`
- `total`
- `items[]:{venue_id,slug,name,address,rating,delivery_estimate,delivery_fee,wolt_plus}`

### ItemSearchResult (`search items`)
Required:
- `query`
- `total`
- `items[]:{item_id,venue_id,venue_slug,name,base_price,currency,is_sold_out}`

### VenueDetail (`venue show`)
Required:
- `venue_id`
- `slug`
- `name`
- `address`
- `currency`
- `rating`
- `delivery_methods`
- `order_minimum`

### VenueMenu (`venue menu`)
Required:
- `venue_id`
- `categories[]`
- `items[]:{item_id,name,base_price}`

Optional:
- `option_group_ids` (when `--include-options`)

### VenueHours (`venue hours`)
Required:
- `venue_id`
- `timezone`
- `opening_windows[]`

### ItemDetail (`item show`)
Required:
- `item_id`
- `venue_id`
- `name`
- `description`
- `price`
- `option_groups[]`
- `upsell_items[]`

### ItemOptions (`item options`)
Required:
- `venue_id`
- `item_id`
- `currency`
- `group_count`
- `option_groups[]`

Each `option_groups[]` entry contains:
- `group_id`
- `name`
- `required`
- `min`
- `max`
- `values[]:{value_id,name,price,example_option}`

### CartState (`cart show`)
Required:
- `basket_id`
- `venue_id`
- `venue_name`
- `venue_slug`
- `selection`
- `currency`
- `total_items`
- `lines[]`
- `subtotal`
- `fees`
- `total`

Each line includes:
- `line_id`
- `item_id`
- `name`
- `count`
- `options[]`
- `price`
- `line_total`

### CartMutationResult (`cart add`, `cart remove`, `cart clear`)
Required:
- `mutation`
- `total_items`
- `total`

Conditional by mutation:
- `add`: `basket_id`, `venue_id`, `line_id`
- `remove`: `basket_id`, `venue_id`, `line_id`, `removed_count`
- `clear`: `basket_ids[]`, `cleared_baskets`

### CheckoutPreview (`checkout preview`)
Required:
- `basket_id`
- `venue_id`
- `venue_name`
- `venue_slug`
- `selection`
- `payable_amount`
- `checkout_rows[]`
- `delivery_configs[]`
- `offers`
- `tip_config`

### ProfileSummary (`profile show`)
Required:
- `user_id`
- `name`
- `email_masked`
- `phone_masked`
- `country`

### AddressList (`profile addresses`)
Required:
- `addresses[]:{address_id,label,street,is_default}`
- `profile_default_address_id`

Optional:
- none

### AddressLinks (`profile addresses links`)
Required:
- `address_id`
- `links:{address_link,entrance_link,coordinates_link}`

### PaymentMethodList (`profile payments`)
Required:
- `methods[]:{method_id,type,label,is_default,is_available_for_checkout}`
