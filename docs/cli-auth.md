# Auth Commands

All commands in this document support:
- `--format json`
- `--format yaml`

Global flags inherited by each command:
- `--format [table|json|yaml]`
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

## wolt auth status

Synopsis:

```console
wolt auth status [--verbose] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--verbose`: include token/session metadata fields (masked)

Output schema:
- `AuthStatus`

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
    "user_id": "usr_5f8c9a2b",
    "country": "FIN",
    "session_expires_at": "2026-02-26T10:00:00Z"
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_auth_status_001
  generated_at: "2026-02-19T21:00:00Z"
  profile: default
  locale: en-FI
data:
  authenticated: true
  user_id: usr_5f8c9a2b
  country: FIN
  session_expires_at: "2026-02-26T10:00:00Z"
warnings: []
```

## wolt auth login

Synopsis:

```console
wolt auth login [--headless=false] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--headless=false`: open interactive browser login flow

Output schema:
- `AuthStatus`

JSON example:

```json
{
  "meta": {
    "request_id": "req_auth_login_001",
    "generated_at": "2026-02-19T21:01:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "authenticated": true,
    "user_id": "usr_5f8c9a2b",
    "country": "FIN",
    "session_expires_at": "2026-02-26T10:01:00Z"
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_auth_login_001
  generated_at: "2026-02-19T21:01:00Z"
  profile: default
  locale: en-FI
data:
  authenticated: true
  user_id: usr_5f8c9a2b
  country: FIN
  session_expires_at: "2026-02-26T10:01:00Z"
warnings: []
```

## wolt auth logout

Synopsis:

```console
wolt auth logout [--all-profiles] [global flags]
```

Arguments:
- none

Options:
- `--format [json|yaml]`: machine-readable output
- `--all-profiles`: remove session data for every local profile

Output schema:
- `AuthStatus`

JSON example:

```json
{
  "meta": {
    "request_id": "req_auth_logout_001",
    "generated_at": "2026-02-19T21:02:00Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {
    "authenticated": false,
    "user_id": "",
    "country": "FIN",
    "session_expires_at": null
  },
  "warnings": []
}
```

YAML example:

```yaml
meta:
  request_id: req_auth_logout_001
  generated_at: "2026-02-19T21:02:00Z"
  profile: default
  locale: en-FI
data:
  authenticated: false
  user_id: ""
  country: FIN
  session_expires_at: null
warnings: []
```

## Compatibility Note

Current `wolt-cli` implementation does not yet have `auth` commands.
This document defines the target behavior for v1 implementation.
