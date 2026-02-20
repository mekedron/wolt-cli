# Auth Commands

Included commands:
- `wolt auth status`
- `wolt profile status` (alias)

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`
- `--verbose`
- `--wtoken <token>`
- `--wrtoken <token>`
- `--cookie <name=value>` (repeatable)

## First Step: Configure a Profile

Before using authenticated commands, configure a profile first.

Recommended first command:

```console
wolt configure --profile-name default --address "<address>" --wtoken "<token>" --wrtoken "<refresh-token>"
```

If the profile already exists and you only want to rotate credentials:

```console
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>"
```

Cookie-based setup is also supported:

```console
wolt configure --profile-name default --cookie "__wtoken=<token>" --cookie "__wrtoken=<refresh-token>"
```

After configure, typical checks are:
1. `wolt profile status --verbose`
2. `wolt profile show --format json`
3. `wolt profile addresses --format json`
4. `wolt profile payments --format json`

## Profile-Based Auth and Profile Functionality

Profiles are the default place to keep reusable auth and location settings.
When `--profile` is not passed, the CLI uses the default profile.

Stored profile fields used by auth-enabled commands:
- `wtoken`
- `wrefresh_token`
- `cookies[]`
- location/address fields for location-dependent requests

Security:
- profile config can include sensitive auth values; keep it local only
- do not commit local profiles or config snapshots to git

## Auth Inputs

Authenticated commands accept:
- `--wtoken <token>`: bearer token sent as `Authorization: Bearer <token>`
- `--wrtoken <token>`: refresh token used for automatic bearer token rotation
- `--cookie <name=value>`: repeatable cookie forwarding

If `--wtoken` is omitted and a `--cookie __wtoken=<token>` cookie is provided,
the token is also reused as bearer auth.
If both are omitted, the CLI also tries profile auth fields from the selected
profile (`--profile`, or default profile when omitted):
- `profile.wtoken`
- `profile.wrefresh_token`
- `profile.cookies` (and token extraction from `__wtoken`/`accessToken` cookie values)

`--wtoken` parsing is tolerant of common copy-paste formats:
- Raw JWT: `eyJ...<snip>...`
- Bearer value: `Bearer eyJ...<snip>...`
- JSON payload: `{"accessToken":"eyJ...","expirationTime":1771540095000}`
- URL-encoded JSON payload:
  - `%7B%22accessToken%22%3A%22eyJ...%22%2C%22expirationTime%22%3A1771540095000%7D`
  - `{%22accessToken%22:%22eyJ...%22%2C%22expirationTime%22:1771540095000}`
- Query-style payload: `accessToken=eyJ...&expirationTime=1771540095000`

Cookie fallback extraction also supports:
- `--cookie "__wtoken=<jwt>"`
- `--cookie "foo=1; __wtoken=<jwt>; bar=2"`
- `--cookie "__wtoken={%22accessToken%22:%22<jwt>%22...}"`
- `--cookie "__wrtoken=<refresh-token>"`
- `--cookie "foo=1; __wrtoken=<refresh-token>; bar=2"`

## Automatic Token Rotation

For authenticated commands, if the access token is expired or upstream returns
`401`, the CLI will:
1. call `POST https://authentication.wolt.com/v1/wauth2/access_token` with
   `grant_type=refresh_token`
2. retry the original request once with the rotated access token
3. persist `wtoken` and `wrefresh_token` to the selected profile in local config

Refresh token discovery order:
- `--wrtoken`
- refresh token embedded in `--wtoken` payload
- `__wrtoken` in `--cookie` values
- `profile.wrefresh_token`
- refresh token embedded in `profile.wtoken` payload
- `__wrtoken` in `profile.cookies`

Most reliable source from Chrome:
- `Application -> Cookies -> __wrtoken`
- or response payload field `refresh_token` from
  `POST https://authentication.wolt.com/v1/wauth2/access_token`

## Profile Command Guide

Profile command group coverage:
- `wolt profile status`: auth/session status check (same probe as `auth status`)
- `wolt profile show`: normalized account summary (`user_id`, masked contact fields, country)
- `wolt profile addresses`: Wolt address-book list and address CRUD (`add`, `update`, `remove`, `use`, `links`)
- `wolt profile payments`: payment method list for checkout context
- `wolt profile favorites`: favorite venues listing and add/remove operations

## `wolt auth status`

Synopsis:

```console
wolt auth status [global flags]
```

Behavior:
- With credentials: calls `GET https://restaurant-api.wolt.com/v1/user/me`
- Without credentials: returns `authenticated=false` with a warning
- With `--verbose`: includes token preview/cookie count and detailed upstream error diagnostics

`wolt profile status` is an alias with the same behavior and output schema.

Output schema:
- `authenticated` (bool)
- `user_id` (string)
- `country` (string)
- `session_expires_at` (nullable ISO timestamp, inferred from JWT `exp` when token is provided)

JSON example:

```json
{
  "meta": {
    "request_id": "req_auth_status_001",
    "generated_at": "2026-02-19T21:00:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "authenticated": true,
    "user_id": "624586e0ac0d99adf9947d65",
    "country": "FIN",
    "session_expires_at": "2026-02-19T22:45:37Z"
  },
  "warnings": []
}
```
