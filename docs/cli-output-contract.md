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
- `wolt_plus_subscriber`

Optional:
- `token_preview` (when `--verbose`)
- `cookie_count` (when `--verbose`)

### DiscoveryFeed (`discover feed`)
Required:
- `city`
- `total`
- `count`
- `offset`
- `wolt_plus_only`
- `enrichment_mode` (`full|fast`)
- `sections[]`

Optional:
- `limit` (when `--limit` is set)
- `total_pages` (when `--limit > 0` is set)
- `next_offset` (when more venues are available after current slice)
- `page` (when `--page` is set)
- `query` (when `--query` filter is set)
- `sort`

Each `sections[].items[]` row includes:
- `venue_id`
- `slug`
- `name`
- `rating`
- `delivery_estimate`
- `delivery_fee`
- `price_range` (integer level, may be null)
- `price_range_scale` (for example `$`, `$$`, `$$$`)
- `promotions[]` (active venue promotion labels)
- `wolt_plus`

Notes:
- promotions are enriched with dynamic campaign banners (for example `40% off selected items`) when the dynamic endpoint is available.
- in `fast` enrichment mode, dynamic/static per-venue enrichment is skipped.

### CategoryList (`discover categories`)
Required:
- `categories[]:{id,name,slug}`

### VenueSearchResult (`search venues`)
Required:
- `query`
- `total`
- `items[]:{venue_id,slug,name,address,rating,delivery_estimate,delivery_fee,price_range,price_range_scale,promotions,wolt_plus}`

Optional:
- `count`
- `offset`
- `limit`
- `total_pages`
- `next_offset`
- `page`

Notes:
- venue promotions are enriched with dynamic campaign banners (for example `40% off selected items`) when the dynamic endpoint is available.

### ItemSearchResult (`search items`)
Required:
- `query`
- `total`
- `items[]:{item_id,venue_id,venue_slug,name,base_price,currency,is_sold_out}`

Optional:
- `items[].discounts`
- `items[].original_price`
- `count`
- `offset`
- `limit`
- `total_pages`
- `next_offset`
- `page`

Notes:
- `base_price.currency`/`base_price.formatted_amount` are normalized from payload venue metadata when upstream omits currency.

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

### VenueCategoryList (`venue categories`)
Required:
- `venue_id`
- `loading_strategy`
- `categories[]:{id,slug,name,parent_slug,level,leaf,item_refs_count}`

### VenueItemSearchResult (`venue search`)
Required:
- `venue_id`
- `venue_slug`
- `query`
- `total`
- `items[]:{item_id,name,category,base_price,discounts,is_sold_out}`

Optional:
- `original_price` (when upstream exposes pre-discount amount)
- `option_group_ids` (when `--include-options`)
- `count`
- `offset`
- `limit`
- `total_pages`
- `next_offset`
- `page`
- `sort`

Notes:
- `base_price.currency`/`base_price.formatted_amount` are normalized from venue metadata when upstream search payload omits currency.
- when upstream returns `original_price` without promotion labels, `discounts[]` may contain a derived label like `21% off`.

### VenueMenu (`venue menu`)
Required:
- `venue_id`
- `wolt_plus`
- `categories[]`
- `items[]:{item_id,name,base_price,discounts}`

Optional:
- `original_price` (for campaign-adjusted menu prices)
- `option_group_ids` (when `--include-options`)
- `count`
- `offset`
- `limit`
- `total_pages`
- `next_offset`
- `page`
- `sort`

Notes:
- item-level campaign discounts from dynamic venue payloads are merged into `discounts[]`.
- when a percentage campaign applies, `base_price` is adjusted to discounted value and `original_price` is included.

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

Notes:
- `price.currency`/`price.formatted_amount` are normalized from payload venue metadata when upstream omits currency.
- `upsell_items[].price` follows the same normalization.

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

### OrderHistoryList (`profile orders`, `profile orders list`)
Required:
- `orders[]:{purchase_id,received_at,status,venue_name,total_amount,is_active,items_summary,payment_time_ts,main_image,main_image_blurhash}`
- `count`

Optional:
- `next_page_token`
- `status_filter`

### OrderHistoryDetail (`profile orders show`)
Required:
- `order_id`
- `status`
- `currency`
- `venue:{id,name,address,phone,country,product_line}`
- `totals:{items,delivery,service_fee,subtotal,credits,tokens,total}` where each value is `{amount,formatted_amount}`
- `items[]:{id,name,count,price,line_total,options}`
- `payments[]:{name,amount,method_type,method_id,provider,payment_time}`
- `delivery:{alias,address,city,comment}`

Optional:
- `order_number`
- `creation_time`
- `delivery_time`
- `delivery_method`
- `discounts[]:{title,amount}`
- `surcharges[]:{title,amount}`

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
