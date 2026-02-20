# Auth Commands

Included commands:
- `wolt auth status`
- `wolt profile status` (alias)

Shared/global flags are documented in `cli-overview`.

## First Step: Configure a Profile

Before using authenticated commands, configure a profile first.

```console
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>" --overwrite
```

If the profile already exists and you only want to rotate credentials:

```console
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>"
```

Cookie-based setup is also supported:

```console
wolt configure --profile-name default --cookie "__wtoken=<token>" --cookie "__wrtoken=<refresh-token>"
```

## Profile-Based Auth

Profiles are the default place to keep reusable auth settings.
When `--profile` is not passed, the CLI uses the default profile.

Stored profile fields used by auth-enabled commands:
- `wtoken`
- `wrefresh_token`
- `cookies[]`

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

`--wtoken` parsing accepts common copy-paste formats:
- raw JWT
- `Bearer <jwt>`
- JSON payload containing `accessToken`
- URL-encoded JSON payload containing `accessToken`
- query-style payload containing `accessToken`

Cookie fallback extraction also supports:
- `__wtoken=<jwt>`
- cookie headers containing `__wtoken=<jwt>`
- cookie headers containing `__wrtoken=<refresh-token>`

## Automatic Token Rotation

For authenticated commands, if the access token is expired or upstream returns `401`, the CLI:
1. calls `POST https://authentication.wolt.com/v1/wauth2/access_token` with `grant_type=refresh_token`
2. retries the original request once with the rotated access token
3. persists `wtoken` and `wrefresh_token` to the selected profile in local config

Refresh token discovery order:
1. `--wrtoken`
2. refresh token embedded in `--wtoken` payload
3. `__wrtoken` in `--cookie` values
4. `profile.wrefresh_token`
5. refresh token embedded in `profile.wtoken` payload
6. `__wrtoken` in `profile.cookies`

Most reliable source from Chrome:
- `Application -> Cookies -> __wrtoken`
- or `refresh_token` from `POST https://authentication.wolt.com/v1/wauth2/access_token`

## `wolt auth status`

```console
wolt auth status [global flags]
```

Behavior:
- with credentials: calls `GET https://restaurant-api.wolt.com/v1/user/me`
- includes `wolt_plus_subscriber` flag when account membership signal is present
- without credentials: returns `authenticated=false` with a warning
- with `--verbose`: includes token preview/cookie count, upstream HTTP request trace, and detailed upstream error diagnostics

`wolt profile status` is an alias with the same behavior and output schema.
