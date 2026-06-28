#!/usr/bin/env bash
# live-smoke.sh — exercise wolt-cli commands against the live upstream.
# Detects Wolt-side contract drift before users do.
#
# Invoked daily by .github/workflows/live-smoke.yml. Also runnable
# locally: just run it, it'll use whatever ~/.wolt/.wolt-config.json
# you're already logged into.
#
# In CI, set:
#   WOLT_SMOKE_CONFIG_JSON  contents of ~/.wolt/.wolt-config.json
# When set, the script writes it to ~/.wolt/.wolt-config.json verbatim,
# overwriting any existing local login. Seed it with:
#     gh secret set WOLT_SMOKE_CONFIG_JSON < ~/.wolt/.wolt-config.json
# That carries the full set of session cookies Wolt requires
# (telemetryDeviceId, activeLocation, etc.) — synthesising only
# __wtoken/__wrtoken returns 401 "session expired".
# Skip the env-var path when running locally to keep your real session intact.
#
# The read-only surface (status/account/feed/venue/cart-show) always runs
# and never mutates. The cart round-trip (add → checkout preview → remove)
# is opt-in via WOLT_SMOKE_CART=1 — CI sets it; local runs leave it unset
# so the script never touches your real cart. Even when enabled it only
# previews checkout (no order is ever placed) and a trap tears the basket
# down on exit. Never add login/logout or real order placement here.

set -euo pipefail

readonly WOLT_BIN="${WOLT_BIN:-./bin/wolt}"
# Central Helsinki — Rautatientori. Hardcoded so the smoke surface is
# stable across runs; the venue catalogue and feed shape vary by city.
readonly HEL_LAT="60.1699"
readonly HEL_LON="24.9384"
readonly KNOWN_VENUE="${WOLT_SMOKE_VENUE:-burger-king-finnoo}"
readonly SMOKE_DIR="${SMOKE_DIR:-${TMPDIR:-/tmp}/wolt-smoke}"

# Cart round-trip (add → checkout preview → remove). Opt-in: only runs
# when WOLT_SMOKE_CART=1 (CI sets it) so local runs stay read-only and
# never touch your real cart. McDonald's Kamppi is a stable, always-open
# central-Helsinki venue; the item is resolved from its live menu at run
# time via `cart add --cheapest` (cheapest in-stock match) so we never pin
# a volatile item id. When the name query matches nothing orderable — e.g.
# the Quarter Pounders are sold out during breakfast hours, which flaked
# issue #25 — we fall back to the venue's cheapest in-stock item so the
# round-trip still exercises add/preview/remove instead of hard-failing.
readonly RUN_CART_SMOKE="${WOLT_SMOKE_CART:-0}"
readonly MCD_VENUE="${WOLT_SMOKE_CART_VENUE:-mcdonalds-kamppi-1}"
MCD_ITEM_QUERY="$(printf '%s' "${WOLT_SMOKE_CART_ITEM_QUERY:-with cheese}" | tr '[:upper:]' '[:lower:]')"
readonly MCD_ITEM_QUERY

# wolt-mcp powers the MCP checkout-preview smoke — the code path PR #23 fixed
# (the MCP handler used to POST a flat body Wolt rejected for a missing
# purchase_plan). Built on demand if the workflow didn't pre-build it, so local
# runs work the same. Only exercised inside the opt-in cart round-trip below.
# Exported so the smoke client (and the wolt-mcp it spawns) inherit it; readonly
# so nothing mutates it — but that's why we must NOT re-assign it as a
# command-prefix env below (assigning to a readonly var aborts the script).
export WOLT_MCP_BIN="${WOLT_MCP_BIN:-./bin/wolt-mcp}"
readonly WOLT_MCP_BIN

mkdir -p "${SMOKE_DIR}"

pass=0
fail=0
declare -a failures=()

# redact — strip anything resembling a user identifier from a stream so
# stderr can be safely printed to the public Actions log. We err on the
# side of redacting too much; debugging always has the local file with
# the unredacted body available outside the Actions log.
redact() {
  sed -E \
    -e 's/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/<JWT>/g' \
    -e 's/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/<EMAIL>/g' \
    -e 's/[a-f0-9]{24}/<OID>/g' \
    -e 's/(__wtoken|__wrtoken|wtoken|wrtoken|wrefresh_token|access_token|refresh_token)=[^"&;[:space:]]+/\1=<REDACTED>/gI' \
    -e 's/Bearer [A-Za-z0-9._-]+/Bearer <REDACTED>/gI' \
    -e 's/-?[0-9]{1,3}\.[0-9]{3,}, ?-?[0-9]{1,3}\.[0-9]{3,}/<LATLON>/g'
}

# run "label" cmd args...
# Captures stdout JSON to ${SMOKE_DIR}/<label>.json. Stderr to
# ${SMOKE_DIR}/<label>.err. On non-zero exit, prints a REDACTED stderr
# tail AND the redacted error envelope (code+message), since wolt-cli
# in --format json puts errors in stdout via emitError.
run() {
  local label="$1"; shift
  local slug="${label// /_}"
  local out="${SMOKE_DIR}/${slug}.json"
  local err="${SMOKE_DIR}/${slug}.err"
  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if "$@" --format json >"${out}" 2>"${err}"; then
    printf "ok (%s bytes)\n" "$(wc -c <"${out}" | tr -d ' ')"
    pass=$((pass + 1))
  else
    local rc=$?
    printf "FAIL (exit %d)\n" "${rc}"
    {
      # Surface the envelope error first — it's the canonical reason
      # wolt-cli exited non-zero in JSON mode.
      jq -r '
        .errors // empty
        | "code:    \(.code // "-")\nmessage: \(.message // "-")"
      ' "${out}" 2>/dev/null
      # Then any stderr (verbose trace, panics, etc.) for completeness.
      head -10 "${err}" 2>/dev/null
    } | redact | sed 's/^/    | /' | head -20 || true
    fail=$((fail + 1))
    failures+=("${label}")
  fi
}

# run_mcp_checkout_preview "<venue>" — exercise the MCP wolt_checkout_preview
# tool end to end by spawning wolt-mcp over stdio. This is the MCP counterpart
# to the CLI "checkout preview" step and guards the exact regression PR #23
# fixed (the MCP handler now builds the shared purchase_plan payload). It needs
# a basket already on the account, so it only runs inside the cart round-trip.
# Preview-only — no order is ever placed. The structured preview lands in a
# local file (byte count only in the public log); stderr is redacted on failure.
run_mcp_checkout_preview() {
  local venue="$1"
  local label="mcp checkout preview"
  local out="${SMOKE_DIR}/mcp_checkout_preview.json"
  local err="${SMOKE_DIR}/mcp_checkout_preview.err"

  # Build wolt-mcp once if the workflow didn't already drop it in ./bin.
  if [ ! -x "${WOLT_MCP_BIN}" ]; then
    if ! go build -o "${WOLT_MCP_BIN}" ./cmd/wolt-mcp >"${err}" 2>&1; then
      printf "[%s] %-22s ... FAIL (could not build wolt-mcp)\n" "$(date -u +%H:%M:%S)" "${label}"
      redact <"${err}" | sed 's/^/    | /' | head -10 || true
      fail=$((fail + 1)); failures+=("${label}")
      return
    fi
  fi

  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  # WOLT_MCP_BIN is already exported above; only the per-call coords go here.
  if WOLT_SMOKE_LAT="${HEL_LAT}" WOLT_SMOKE_LON="${HEL_LON}" \
     go run ./scripts/mcp-checkout-smoke "${venue}" >"${out}" 2>"${err}"; then
    printf "ok (%s bytes)\n" "$(wc -c <"${out}" | tr -d ' ')"
    pass=$((pass + 1))
  else
    printf "FAIL\n"
    redact <"${err}" | sed 's/^/    | /' | head -20 || true
    fail=$((fail + 1)); failures+=("${label}")
  fi
}

# cart_add_cheapest "<venue>" "<query>" — add the cheapest in-stock item to the
# basket via the CLI's own resolver. Prefer the named query, but fall back to
# the venue's cheapest in-stock item when the query matches nothing orderable
# (sold out / off-menu), so a transient miss doesn't fail the round-trip. The
# resolved line id + 24-char venue id come back in the add envelope, which the
# preview and teardown steps below read. Writes the envelope to cart_add.json.
cart_add_cheapest() {
  local venue="$1" query="$2"
  local label="cart add"
  local out="${SMOKE_DIR}/cart_add.json"
  local err="${SMOKE_DIR}/cart_add.err"
  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if "${WOLT_BIN}" cart add "${venue}" --query "${query}" --cheapest --count 1 --format json >"${out}" 2>"${err}" \
     || "${WOLT_BIN}" cart add "${venue}" --cheapest --count 1 --format json >"${out}" 2>"${err}"; then
    local name price
    name="$(jq -r '.data.item_name // "item"' "${out}" 2>/dev/null || echo item)"
    price="$(jq -r '.data.item_price // 0' "${out}" 2>/dev/null || echo 0)"
    printf "ok (%s, %s c)\n" "${name}" "${price}"
    pass=$((pass + 1))
    return 0
  fi
  printf "FAIL\n"
  {
    jq -r '.errors // empty | "code:    \(.code // "-")\nmessage: \(.message // "-")"' "${out}" 2>/dev/null
    head -10 "${err}" 2>/dev/null
  } | redact | sed 's/^/    | /' | head -20 || true
  fail=$((fail + 1))
  failures+=("${label}")
  return 1
}

# seed_config_from_env — when CI hands us the full config blob, write
# it to ~/.wolt/.wolt-config.json with owner-only perms. Roundtrip
# through jq so we (a) validate it's well-formed JSON before disk
# touch, (b) overwrite the local location with Helsinki center for
# stable smoke results.
seed_config_from_env() {
  if [ -z "${WOLT_SMOKE_CONFIG_JSON:-}" ]; then
    return 0
  fi
  mkdir -p "${HOME}/.wolt"
  umask 077
  printf '%s' "${WOLT_SMOKE_CONFIG_JSON}" \
    | jq --argjson lat "${HEL_LAT}" --argjson lon "${HEL_LON}" \
        '.account.location = {lat: $lat, lon: $lon}' \
    >"${HOME}/.wolt/.wolt-config.json"
  chmod 600 "${HOME}/.wolt/.wolt-config.json"
}

seed_config_from_env

# Pre-flight: print a redacted HTTP trace of the first authenticated
# call so the public log shows the actual status code Wolt returned
# (instead of the smoke just printing "FAIL"). Verbose lines look like
# "[http] -> GET <url>" / "[http] <- GET <url> status=N duration=Yms" —
# the redactor scrubs any token/ID/email that sneaks in.
echo "-- pre-flight diagnostic --"
"${WOLT_BIN}" status --format json --verbose >/dev/null 2>"${SMOKE_DIR}/preflight.stderr" || true
grep -E '^\[(http|verbose)\]' "${SMOKE_DIR}/preflight.stderr" 2>/dev/null \
  | redact \
  | sed 's/^/    /' \
  | head -10 || true
echo "-- end pre-flight --"

# ---- read-only smoke surface --------------------------------------

# status doubles as the auth-refresh exerciser — if Wolt's refresh
# contract drifted, this is where it shows up first.
run "status"            "${WOLT_BIN}" status
run "account"           "${WOLT_BIN}" account
run "account orders"    "${WOLT_BIN}" account orders --limit 3
run "account payments"  "${WOLT_BIN}" account payments
run "account addresses" "${WOLT_BIN}" account addresses
run "account favorites" "${WOLT_BIN}" account favorites --limit 5

# Chase one order detail — this is the endpoint whose 429 behavior we
# rely on in stats. Skipped when the account has no orders.
if order_id="$(jq -r '.data.orders[0].purchase_id // .data.orders[0]._id // ""' "${SMOKE_DIR}/account_orders.json" 2>/dev/null)" && [ -n "${order_id}" ]; then
  run "account order"   "${WOLT_BIN}" account order "${order_id}"
else
  printf "[%s] %-22s ... skipped (no orders to drill into)\n" "$(date -u +%H:%M:%S)" "account order"
fi

run "feed summary"  "${WOLT_BIN}" feed --summary
run "top 5"         "${WOLT_BIN}" top 5
run "venues query"  "${WOLT_BIN}" venues --query burger --limit 3
run "venue static"  "${WOLT_BIN}" venue "${KNOWN_VENUE}"
run "venue menu"    "${WOLT_BIN}" venue menu "${KNOWN_VENUE}"
run "cart"          "${WOLT_BIN}" cart
run "cart count"    "${WOLT_BIN}" cart count

# ---- cart round-trip (opt-in; mutating) ---------------------------
# Add a cheeseburger from McDonald's Kamppi, preview checkout, then
# remove it — exercising the cart-mutation path the read-only surface
# can't (and which issue #19 silently broke). A trap clears the venue's
# basket on exit so a mid-run failure never strands a basket on the
# account. Checkout is preview-only; no order is ever placed.
if [ "${RUN_CART_SMOKE}" = "1" ]; then
  echo ""
  echo "-- cart round-trip (mutating) --"

  # Cleanup target; refined to the real venue id once the add resolves it.
  cart_venue_id="${MCD_VENUE}"
  cleanup_cart() {
    if [ -n "${cart_venue_id}" ]; then
      "${WOLT_BIN}" cart clear --venue-id "${cart_venue_id}" --format json >/dev/null 2>&1 || true
    fi
  }
  trap cleanup_cart EXIT

  run "mcd menu" "${WOLT_BIN}" venue menu "${MCD_VENUE}"

  # Resolve + add the item through the CLI's own `cart add --cheapest`, with a
  # fallback from the named query to the venue's cheapest item (see the helper).
  if cart_add_cheapest "${MCD_VENUE}" "${MCD_ITEM_QUERY}"; then
    # The add envelope carries the resolved line id + the 24-char venue id
    # (issue #19 fix). Scope the preview + teardown to them, not the slug.
    cart_item_id="$(jq -r '.data.line_id // ""' "${SMOKE_DIR}/cart_add.json" 2>/dev/null || true)"
    add_venue_id="$(jq -r '.data.venue_id // ""' "${SMOKE_DIR}/cart_add.json" 2>/dev/null || true)"
    if [ -n "${add_venue_id}" ]; then
      cart_venue_id="${add_venue_id}"
    fi

    run "checkout preview" "${WOLT_BIN}" checkout --venue-id "${cart_venue_id}"
    # Same basket, via the MCP tool — covers the handler the CLI path doesn't.
    run_mcp_checkout_preview "${cart_venue_id}"
    if [ -n "${cart_item_id}" ]; then
      run "cart remove" "${WOLT_BIN}" cart remove "${cart_item_id}" --all --venue-id "${cart_venue_id}"
    else
      # No line id surfaced — fall back to clearing the venue basket so the
      # account is left clean even though the targeted remove couldn't run.
      "${WOLT_BIN}" cart clear --venue-id "${cart_venue_id}" --format json >/dev/null 2>&1 || true
      printf "[%s] %-22s ... skipped (no line id in add envelope; basket cleared)\n" "$(date -u +%H:%M:%S)" "cart remove"
    fi
  fi
fi

# ---- summary -------------------------------------------------------

echo ""
echo "== summary =="
echo "passed: ${pass}"
echo "failed: ${fail}"
if [ "${fail}" -gt 0 ]; then
  printf "failed steps: %s\n" "$(IFS=', '; echo "${failures[*]}")"
  exit 1
fi
