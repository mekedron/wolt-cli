# Output and Errors

## Machine-Readable Envelope

Use `--format json` for automation. Every response follows:

```json
{
  "meta": {
    "request_id": "req_xxx",
    "generated_at": "2026-02-19T20:45:09Z",
    "profile": "default",
    "locale": "en-FI"
  },
  "data": {},
  "warnings": [],
  "error": {
    "code": "WOLT_AUTH_REQUIRED",
    "message": "...",
    "details": {}
  }
}
```

`error` is omitted on success.

## Parsing Guidelines

- Read primary payload from `.data`.
- Always inspect `.warnings` and surface important warnings.
- On failure, present `.error.code` and `.error.message`.
- Keep `meta.request_id` for troubleshooting/log correlation.

## Common Error Codes

- `WOLT_AUTH_REQUIRED`: missing credentials
- `WOLT_INVALID_ARGUMENT`: invalid flag combinations or required args missing
- `WOLT_PROFILE_ERROR`: profile load/select/write failure
- `WOLT_LOCATION_RESOLVE_ERROR`: address geocoding failure
- `WOLT_UPSTREAM_ERROR`: upstream HTTP/API failure (details with `--verbose`)
- `WOLT_EMPTY_CART`: checkout/cart mutation attempted without basket items
- `WOLT_ITEM_NOT_FOUND`: item not found in selected basket/venue
- `WOLT_REMOVE_UNSUPPORTED`: remove operation cannot be mapped safely
- `WOLT_CHECKOUT_PAYLOAD_ERROR`: failed to build checkout preview payload
- `WOLT_NOT_FOUND`: requested address/entity missing

## Diagnostics

Rerun with `--verbose` when debugging:

- enables HTTP trace output to stderr
- preserves machine envelope in stdout
- returns richer upstream error details

## Exit Codes

- `0`: success
- `1`: command/domain/upstream error
- `2`: unknown command
