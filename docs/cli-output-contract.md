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

- IDs: string identifiers from upstream APIs (`venue_id`, `purchase_id`, `item_id`)
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
      "hint": "Run wolt auth login"
    }
  }
}
```

Error fields:
- `code` (string, stable machine key)
- `message` (human-readable)
- `details` (object, optional)

## Canonical Schema Types

### AuthStatus
Required:
- `authenticated`
- `user_id`
- `country`
- `session_expires_at`

### DiscoveryFeed
Required:
- `city`
- `sections[]`

### CategoryList
Required:
- `categories[]:{id,name,slug}`

### VenueSearchResult
Required:
- `query`
- `total`
- `items[]:{venue_id,slug,name,address,rating,delivery_estimate,delivery_fee,wolt_plus}`

### ItemSearchResult
Required:
- `query`
- `total`
- `items[]:{item_id,venue_id,venue_slug,name,base_price,currency,is_sold_out}`

### VenueDetail
Required:
- `venue_id`
- `slug`
- `name`
- `address`
- `currency`
- `rating`
- `delivery_methods`
- `order_minimum`

### VenueMenu
Required:
- `venue_id`
- `categories[]`
- `items[]:{item_id,name,base_price,option_group_ids}`

### VenueHours
Required:
- `venue_id`
- `timezone`
- `opening_windows[]`
- `delivery_windows[]`

### ItemDetail
Required:
- `item_id`
- `venue_id`
- `name`
- `description`
- `price`
- `option_groups[]`
- `upsell_items[]`

### CartState
Required:
- `basket_id`
- `venue_id`
- `currency`
- `lines[]`
- `subtotal`
- `fees`
- `total`

### CartMutationResult
Required:
- `basket_id`
- `venue_id`
- `mutation`
- `line_id`
- `total_items`
- `total`

### CheckoutPreview
Required:
- `payable_amount`
- `checkout_rows[]`
- `delivery_configs[]`
- `offers`
- `tip_config`

### DeliveryModes
Required:
- `modes[]:{label,schedule,estimate,additional_fee}`

### CheckoutQuote
Required:
- `payable_amount`
- `selected_delivery_mode`
- `selected_tip`
- `payment_breakdown`
- `purchase_validation`

### OrderList
Required:
- `next_cursor`
- `orders[]:{purchase_id,received_at,status,venue_name,total_amount,is_active}`

### OrderDetail
Required:
- `order_id`
- `order_number`
- `status`
- `items[]`
- `fees`
- `discounts`
- `payments`
- `delivery_method`

### ProfileSummary
Required:
- `user_id`
- `name`
- `email_masked`
- `phone_masked`
- `country`

### AddressList
Required:
- `addresses[]:{address_id,label,street,city,lat,lon,is_default}`

### PaymentMethodList
Required:
- `methods[]:{method_id,type,label,is_default,is_available_for_checkout}`
