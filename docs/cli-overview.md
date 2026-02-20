# Documentation Overview

Current scope:
- Binary: `wolt`
- Implemented command groups: `auth`, `discover`, `search`, `venue`, `item`, `cart`, `checkout`, `profile`, `configure`

## Root Interface

```console
wolt <group> <command> [flags]
```

## Global Flags

All command leaf nodes support:
- `--format [table|json|yaml]` (default: `table`)
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>` for authenticated upstream calls (raw JWT, `Bearer ...`, or payload containing `accessToken`)
- `--wrtoken <token>` for automatic bearer token rotation (or payload containing `refreshToken`)
- `--cookie <name=value>` repeatable cookie forwarding for authenticated upstream calls

When `--wtoken` and `--cookie` are not provided, authenticated commands also
attempt to use `wtoken`/`wrefresh_token`/`cookies` from the selected profile in local config.
When refresh credentials are available, expired/401 access tokens are rotated
automatically and persisted back into the selected profile.

## Implemented Authenticated Flows

Implemented against observed web endpoints:
- `GET /v1/user/me` (`auth status`, `profile status`, `profile show`)
- `GET /v3/user/me/payment_methods` (`profile payments`)
- `GET https://payment-service.wolt.com/v1/payment-methods/profile` (`profile payments` full web-style methods)
- `GET /v2/delivery/info` (`profile addresses`, `profile addresses links`)
- `POST /v2/delivery/info` (`profile addresses add`, `profile addresses update`)
- `DELETE /v2/delivery/info/{id}` (`profile addresses remove`)
- `GET /v1/pages/venue-list/profile/favourites` (`profile favorites`, `profile favorites list`)
- `PUT /v3/venues/favourites/{venue_id}` (`profile favorites add`)
- `DELETE /v3/venues/favourites/{venue_id}` (`profile favorites remove`)
- `GET /v1/consumer-api/address-fields` (still available for country/location metadata lookups)
- `GET /order-xp/v1/baskets/count` (`cart count`)
- `GET /order-xp/web/v1/pages/baskets` (`cart show`, cart totals refresh)
- `POST /order-xp/v1/baskets` (`cart add`)
- `POST /order-xp/v1/baskets/bulk/delete` (`cart clear`, single-basket clear fallback in `cart remove`)
- `POST /order-xp/web/v2/pages/checkout` (`checkout preview`)
- `POST https://authentication.wolt.com/v1/wauth2/access_token` (automatic access-token refresh on expiry/401)
- `GET /order-xp/web/v1/pages/venue/<venue_id>/item/<item_id>` (`item show`, `item options`, option inference in cart/checkout flows)

## Safety

- `checkout preview` is projection-only and does not place orders.
- There is no order placement command in this phase.

## Quick Start

```console
wolt auth status --wtoken <token> --format json
wolt profile status --wtoken <token> --format json
wolt item options burger-king-finnoo <item-id> --format json
wolt cart show --wtoken <token> --format json
wolt cart add <venue-id> <item-id> --count 1 --wtoken <token> --format json
wolt cart remove <item-id> --count 1 --wtoken <token> --format json
wolt cart clear --wtoken <token> --format json
wolt checkout preview --wtoken <token> --delivery-mode standard --format json
wolt profile payments --wtoken <token> --format json
wolt profile payments --wtoken <token> --label revolut --format json
wolt profile favorites --wtoken <token> --format json
wolt profile favorites add rioni-espoo --wtoken <token> --format json
wolt profile favorites remove 5a8426f188b5de000b8857bb --wtoken <token> --format json
```
