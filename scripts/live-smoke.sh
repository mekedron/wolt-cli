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
# previews checkout (no order is ever placed) and a trap removes only the
# quantity added by this run. Never add login/logout or real order placement
# here.

set -euo pipefail

readonly WOLT_BIN="${WOLT_BIN:-./bin/wolt}"
# Defaults preserve the repository's existing CI fixture. Every
# market-specific value is configurable so forks and local runs can exercise
# the same contracts in another supported Wolt market.
readonly SMOKE_LAT="${WOLT_SMOKE_LAT:-60.1699}"
readonly SMOKE_LON="${WOLT_SMOKE_LON:-24.9384}"
readonly SMOKE_VENUE="${WOLT_SMOKE_VENUE:-}"
readonly SMOKE_DIR="${SMOKE_DIR:-${TMPDIR:-/tmp}/wolt-smoke}"

# Cart round-trip (add → checkout preview → remove). Opt-in: only runs
# when WOLT_SMOKE_CART=1 (CI sets it) so local runs stay read-only and
# never touch your real cart. By default, the script discovers an open venue
# and a currently orderable item without required options. An explicit venue
# override remains available for controlled CI fixtures.
readonly RUN_CART_SMOKE="${WOLT_SMOKE_CART:-0}"
readonly CART_VENUE="${WOLT_SMOKE_CART_VENUE:-}"

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
skipped=0
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

is_success_envelope() {
  jq -e '
    type == "object"
    and has("data")
    and ((.error // null) == null)
  ' "$1" >/dev/null 2>&1
}

is_menu_envelope() {
  jq -e '
    type == "object"
    and has("data")
    and ((.error // null) == null)
    and ((.data.venue_id // null) | type == "string")
    and (.data.venue_id | test("^[0-9A-Fa-f]{24}$"))
    and ((.data.items // null) | type == "array")
  ' "$1" >/dev/null 2>&1
}

is_cart_envelope() {
  local file="$1" expected_venue="$2"
  jq -e --arg expected_venue "${expected_venue}" '
    type == "object"
    and ((.error // null) == null)
    and ((.data // null) | type == "object")
    and ((.data.venue_id // null) | type == "string")
    and ((.data.venue_id | ascii_downcase) == ($expected_venue | ascii_downcase))
    and ((.data.lines // null) | type == "array")
    and all(
      .data.lines[]?;
      ((.item_id // null) | type == "string")
      and (.item_id | test("^\\S+$"))
    )
  ' "${file}" >/dev/null 2>&1
}

is_cart_add_envelope() {
  local file="$1" expected_item="$2" expected_venue="$3"
  jq -e \
    --arg expected_item "${expected_item}" \
    --arg expected_venue "${expected_venue}" '
    type == "object"
    and ((.error // null) == null)
    and ((.data // null) | type == "object")
    and .data.mutation == "add"
    and ((.data.line_id // null) | type == "string")
    and (.data.line_id | test("^\\S+$"))
    and ((.data.venue_id // null) | type == "string")
    and (.data.venue_id | test("^[0-9A-Fa-f]{24}$"))
    and ((.data.line_id | ascii_downcase) == ($expected_item | ascii_downcase))
    and ((.data.venue_id | ascii_downcase) == ($expected_venue | ascii_downcase))
  ' "${file}" >/dev/null 2>&1
}

is_cart_remove_envelope() {
  local file="$1" expected_item="$2" expected_venue="$3"
  jq -e \
    --arg expected_item "${expected_item}" \
    --arg expected_venue "${expected_venue}" '
    type == "object"
    and ((.error // null) == null)
    and ((.data // null) | type == "object")
    and (
      .data.mutation == "remove"
      or .data.mutation == "clear"
    )
    and ((.data.line_id // null) | type == "string")
    and ((.data.venue_id // null) | type == "string")
    and ((.data.line_id | ascii_downcase) == ($expected_item | ascii_downcase))
    and ((.data.venue_id | ascii_downcase) == ($expected_venue | ascii_downcase))
    and ((.data.removed_count // null) | type == "number")
    and .data.removed_count == 1
  ' "${file}" >/dev/null 2>&1
}

# load_venue_menu "<venue>" [menu flags...]
# Loads a normal menu when the root assortment is complete. For partial
# catalogs, selects a category exposed by the venue-categories contract and
# retries against that authoritative category endpoint. This keeps live smoke
# independent of venue type and assortment shape. Output remains the selected
# menu envelope so callers can apply their usual validation and redaction.
load_venue_menu() {
  local venue="$1"
  shift

  local probe_out="${SMOKE_DIR}/venue_menu_probe.json"
  local probe_err="${SMOKE_DIR}/venue_menu_probe.err"
  local probe_rc
  if "${WOLT_BIN}" venue menu "${venue}" "$@" \
    >"${probe_out}" 2>"${probe_err}"; then
    cat "${probe_out}"
    cat "${probe_err}" >&2
    return 0
  else
    probe_rc=$?
  fi

  if ! jq -e '
      .error.code == "WOLT_INVALID_ARGUMENT"
      and ((.error.message // "") | test("assortment is partial"; "i"))
    ' "${probe_out}" >/dev/null 2>&1; then
    cat "${probe_out}"
    cat "${probe_err}" >&2
    return "${probe_rc}"
  fi

  local categories_out categories_err
  categories_out="${SMOKE_DIR}/venue_menu_categories.json"
  categories_err="${SMOKE_DIR}/venue_menu_categories.err"
  if ! "${WOLT_BIN}" venue categories "${venue}" --format json \
    >"${categories_out}" 2>"${categories_err}"; then
    cat "${probe_out}"
    cat "${probe_err}" "${categories_err}" >&2
    return "${probe_rc}"
  fi

  local -a categories=()
  local category
  while IFS= read -r category; do
    category="${category%$'\r'}"
    if [ -n "${category}" ]; then
      categories+=("${category}")
    fi
  done < <(
    jq -r '
      [
        .data.categories[]?
        | select(((.slug // "") | type) == "string")
        | select(.slug | length > 0)
      ]
      | sort_by([
          (if ((.item_refs_count // 0) > 0) then 0
           elif (.leaf // false) == true then 1
           else 2 end),
          (-1 * (.item_refs_count // 0)),
          (-1 * (.level // 0)),
          .slug
        ])
      | .[:5][]
      | .slug
    ' "${categories_out}" 2>/dev/null
  )

  local category_out="${SMOKE_DIR}/venue_menu_category.json"
  local category_err="${SMOKE_DIR}/venue_menu_category.err"
  local empty_out="${SMOKE_DIR}/venue_menu_empty_category.json"
  local empty_err="${SMOKE_DIR}/venue_menu_empty_category.err"
  : >"${category_out}"
  : >"${category_err}"
  : >"${empty_out}"
  : >"${empty_err}"
  for category in "${categories[@]}"; do
    if ! "${WOLT_BIN}" venue menu "${venue}" --category "${category}" "$@" \
      >"${category_out}" 2>"${category_err}"; then
      continue
    fi
    if ! is_menu_envelope "${category_out}"; then
      continue
    fi
    if jq -e '
        ((.data.items // null) | type == "array")
        and (.data.items | length > 0)
      ' "${category_out}" >/dev/null 2>&1; then
      cat "${category_out}"
      cat "${category_err}" >&2
      return 0
    fi
    if [ ! -s "${empty_out}" ]; then
      cp "${category_out}" "${empty_out}"
      cp "${category_err}" "${empty_err}"
    fi
  done
  if [ -s "${empty_out}" ]; then
    cat "${empty_out}"
    cat "${empty_err}" >&2
    return 0
  fi

  cat "${probe_out}"
  cat "${probe_err}" "${categories_err}" "${category_err}" >&2
  return "${probe_rc}"
}

# run_validated "label" validator cmd args...
# Captures stdout JSON to ${SMOKE_DIR}/<label>.json. Stderr to
# ${SMOKE_DIR}/<label>.err and delegates result classification to validator.
run_validated() {
  local label="$1" validator="$2"
  shift 2
  local slug="${label// /_}"
  local out="${SMOKE_DIR}/${slug}.json"
  local err="${SMOKE_DIR}/${slug}.err"
  local rc
  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if "$@" --format json >"${out}" 2>"${err}"; then
    rc=0
  else
    rc=$?
  fi
  if "${validator}" "${out}" "${rc}"; then
    printf "ok (%s bytes)\n" "$(wc -c <"${out}" | tr -d ' ')"
    pass=$((pass + 1))
    return
  fi
  if [ "${rc}" -eq 0 ]; then
    printf "FAIL (invalid success envelope)\n"
  else
    printf "FAIL (exit %d)\n" "${rc}"
  fi
  {
    jq -r '
      .error // empty
      | "code:    \(.code // "-")\nmessage: \(.message // "-")"
    ' "${out}" 2>/dev/null
    head -10 "${err}" 2>/dev/null
  } | redact | sed 's/^/    | /' | head -20 || true
  fail=$((fail + 1))
  failures+=("${label}")
}

is_success_result() {
  local out="$1" rc="$2"
  [ "${rc}" -eq 0 ] && is_success_envelope "${out}"
}

is_checkout_preview_result() {
  local out="$1" rc="$2"
  if [ "${rc}" -eq 0 ]; then
    is_success_envelope "${out}"
    return
  fi
  jq -e '
    (.error.code == "WOLT_DELIVERY_MODE_UNAVAILABLE"
      or .error.code == "WOLT_CART_ITEMS_UNAVAILABLE")
    and ((.error.message // null) | type == "string")
    and (.error.message | length > 0)
  ' "${out}" >/dev/null 2>&1
}

# run "label" cmd args...
run() {
  local label="$1"
  shift
  run_validated "${label}" is_success_result "$@"
}

# ensure_wolt_mcp "<label>" "<errfile>" — build wolt-mcp on demand when the
# workflow didn't already drop it in ./bin. Returns non-zero after reporting the
# failure itself, so callers just bail out.
ensure_wolt_mcp() {
  local label="$1"
  local err="$2"
  if [ -x "${WOLT_MCP_BIN}" ]; then
    return 0
  fi
  if go build -o "${WOLT_MCP_BIN}" ./cmd/wolt-mcp >"${err}" 2>&1; then
    return 0
  fi
  printf "[%s] %-22s ... FAIL (could not build wolt-mcp)\n" "$(date -u +%H:%M:%S)" "${label}"
  redact <"${err}" | sed 's/^/    | /' | head -10 || true
  fail=$((fail + 1)); failures+=("${label}")
  return 1
}

# run_mcp_venue_smoke "<venue>" — exercise the MCP read-only venue tools
# (wolt_venue_detail and wolt_venue_hours) against the same dynamically
# discovered venue as the CLI. It is read-only and therefore belongs in the
# always-on surface rather than the opt-in cart round-trip.
run_mcp_venue_smoke() {
  local venue="$1"
  local label="mcp venue tools"
  local out="${SMOKE_DIR}/mcp_venue.txt"
  local err="${SMOKE_DIR}/mcp_venue.err"

  ensure_wolt_mcp "${label}" "${err}" || return

  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if WOLT_SMOKE_LAT="${SMOKE_LAT}" WOLT_SMOKE_LON="${SMOKE_LON}" \
     go run ./scripts/mcp-venue-smoke "${venue}" >"${out}" 2>"${err}"; then
    printf "ok\n"
    pass=$((pass + 1))
  else
    printf "FAIL\n"
    redact <"${err}" | sed 's/^/    | /' | head -20 || true
    fail=$((fail + 1)); failures+=("${label}")
  fi
}

skip_step() {
  local label="$1" reason="$2"
  printf "[%s] %-22s ... skipped (%s)\n" "$(date -u +%H:%M:%S)" "${label}" "${reason}"
  skipped=$((skipped + 1))
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

  ensure_wolt_mcp "${label}" "${err}" || return

  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  # WOLT_MCP_BIN is already exported above; only the per-call coords go here.
  if WOLT_SMOKE_LAT="${SMOKE_LAT}" WOLT_SMOKE_LON="${SMOKE_LON}" \
     go run ./scripts/mcp-checkout-smoke "${venue}" >"${out}" 2>"${err}"; then
    printf "ok (%s bytes)\n" "$(wc -c <"${out}" | tr -d ' ')"
    pass=$((pass + 1))
  else
    printf "FAIL\n"
    redact <"${err}" | sed 's/^/    | /' | head -20 || true
    fail=$((fail + 1)); failures+=("${label}")
  fi
}

# discover_cart_fixture prints "<venue-id><TAB><item-id>" for the first open
# discovery venue whose current menu has an orderable, priced item without
# required option groups and which is not already in that venue's basket.
# An explicit WOLT_SMOKE_CART_VENUE limits discovery to that venue. Menu/stock
# absence is a missing live prerequisite, not a product failure.
discover_cart_fixture() {
  local -a candidates=()
  local venue resolved_venue_id item candidate_item code
  local hard_fixture_failures=0
  : >"${SMOKE_DIR}/cart_fixture_failures.err"
  if [ -n "${CART_VENUE}" ]; then
    candidates+=("${CART_VENUE}")
  else
    while IFS= read -r venue; do
      if [ -n "${venue}" ]; then
        candidates+=("${venue}")
      fi
    done < <(
      jq -r '.data.items[]? | .slug // empty' "${SMOKE_DIR}/venues_open.json" 2>/dev/null \
        | head -10
    )
  fi

  for venue in "${candidates[@]}"; do
    if ! load_venue_menu "${venue}" --include-options --format json \
      >"${SMOKE_DIR}/cart_fixture_menu.json" \
      2>"${SMOKE_DIR}/cart_fixture_menu.err"; then
      code="$(
        jq -r '.error.code // empty' \
          "${SMOKE_DIR}/cart_fixture_menu.json" 2>/dev/null || true
      )"
      case "${code}" in
        WOLT_NOT_FOUND|WOLT_VENUE_UNRESOLVED)
          # Discovery and menu loading are separate live calls. A venue can
          # disappear between them without indicating a product regression.
          ;;
        *)
          hard_fixture_failures=$((hard_fixture_failures + 1))
          {
            printf 'venue %s (%s)\n' "${venue}" "${code:-no structured error code}"
            jq -r '
              .error // empty
              | "code:    \(.code // "-")\nmessage: \(.message // "-")"
            ' "${SMOKE_DIR}/cart_fixture_menu.json" 2>/dev/null
            head -10 "${SMOKE_DIR}/cart_fixture_menu.err" 2>/dev/null
          } >>"${SMOKE_DIR}/cart_fixture_failures.err"
          ;;
      esac
      continue
    fi
    if ! is_menu_envelope "${SMOKE_DIR}/cart_fixture_menu.json"; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      {
        printf 'venue %s (invalid success envelope)\n' "${venue}"
        head -10 "${SMOKE_DIR}/cart_fixture_menu.err" 2>/dev/null
      } >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi
    if ! resolved_venue_id="$(
      jq -r '.data.venue_id' "${SMOKE_DIR}/cart_fixture_menu.json" 2>/dev/null
    )" || [ -z "${resolved_venue_id}" ]; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      printf 'venue %s identity could not be extracted safely\n' "${venue}" \
        >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi

    # The cleanup trap is armed before mutation so it also covers a successful
    # POST followed by a malformed response or local write failure. Select an
    # item absent from the basket first, making that pre-armed removal harmless
    # when the add itself did not mutate anything.
    if ! "${WOLT_BIN}" cart --venue-id "${resolved_venue_id}" --format json \
      >"${SMOKE_DIR}/cart_fixture_cart.json" \
      2>"${SMOKE_DIR}/cart_fixture_cart.err"; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      {
        printf 'cart for venue %s could not be inspected safely\n' "${venue}"
        jq -r '
          .error // empty
          | "code:    \(.code // "-")\nmessage: \(.message // "-")"
        ' "${SMOKE_DIR}/cart_fixture_cart.json" 2>/dev/null
        head -10 "${SMOKE_DIR}/cart_fixture_cart.err" 2>/dev/null
      } >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi
    if ! is_cart_envelope \
      "${SMOKE_DIR}/cart_fixture_cart.json" "${resolved_venue_id}"; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      {
        printf 'cart for venue %s returned an invalid success envelope\n' "${venue}"
        head -10 "${SMOKE_DIR}/cart_fixture_cart.err" 2>/dev/null
      } >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi
    if ! jq -r '.data.lines[]?.item_id' \
      "${SMOKE_DIR}/cart_fixture_cart.json" \
      >"${SMOKE_DIR}/cart_fixture_existing_items.txt" 2>/dev/null; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      printf 'cart for venue %s could not be indexed safely\n' "${venue}" \
        >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi

    if ! jq -r '
        .data.items[]?
        | select(
            ((.item_id // .id // "") | test("^\\S+$"))
            and .is_available == true
            and ((.base_price.amount // .price.amount // 0) > 0)
            and (((.option_group_ids // []) | length) == 0)
          )
        | (.item_id // .id)
      ' "${SMOKE_DIR}/cart_fixture_menu.json" \
      >"${SMOKE_DIR}/cart_fixture_candidates.txt" 2>/dev/null; then
      hard_fixture_failures=$((hard_fixture_failures + 1))
      printf 'menu for venue %s could not be indexed safely\n' "${venue}" \
        >>"${SMOKE_DIR}/cart_fixture_failures.err"
      continue
    fi
    item=""
    while IFS= read -r candidate_item; do
      if [ -n "${candidate_item}" ] &&
         ! grep -Fxiq -- "${candidate_item}" \
           "${SMOKE_DIR}/cart_fixture_existing_items.txt"; then
        item="${candidate_item}"
        break
      fi
    done <"${SMOKE_DIR}/cart_fixture_candidates.txt"
    if [ -n "${item}" ]; then
      if [ "${hard_fixture_failures}" -gt 0 ]; then
        return 2
      fi
      printf '%s\t%s\n' "${resolved_venue_id}" "${item}"
      return 0
    fi
  done
  if [ "${hard_fixture_failures}" -gt 0 ]; then
    return 2
  fi
  return 1
}

# cart_add_discovered "<venue-id>" "<item-id>" adds a preflighted live item
# using the canonical identities verified above. If the item disappears
# between preflight and mutation, classify that race as a skipped prerequisite.
cart_add_discovered() {
  local venue_id="$1" item_id="$2"
  local label="cart add"
  local out="${SMOKE_DIR}/cart_add.json"
  local err="${SMOKE_DIR}/cart_add.err"
  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if "${WOLT_BIN}" cart add "${venue_id}" "${item_id}" --count 1 --format json \
    >"${out}" 2>"${err}"; then
    if ! is_cart_add_envelope "${out}" "${item_id}" "${venue_id}"; then
      printf "FAIL (invalid cart-add envelope)\n"
      redact <"${err}" | sed 's/^/    | /' | head -10 || true
      fail=$((fail + 1))
      failures+=("${label}")
      return 1
    fi
    local name price
    name="$(jq -r '.data.item_name // "item"' "${out}" 2>/dev/null || echo item)"
    price="$(jq -r '.data.item_price // 0' "${out}" 2>/dev/null || echo 0)"
    printf "ok (%s, %s c)\n" "${name}" "${price}"
    pass=$((pass + 1))
    return 0
  fi
  local code
  code="$(jq -r '.error.code // ""' "${out}" 2>/dev/null || true)"
  case "${code}" in
    WOLT_ITEM_UNAVAILABLE|WOLT_ITEM_NOT_FOUND|WOLT_NOT_FOUND)
      printf "skipped (item changed after preflight)\n"
      skipped=$((skipped + 1))
      return 2
      ;;
  esac
  printf "FAIL\n"
  {
    jq -r '.error // empty | "code:    \(.code // "-")\nmessage: \(.message // "-")"' "${out}" 2>/dev/null
    head -10 "${err}" 2>/dev/null
  } | redact | sed 's/^/    | /' | head -20 || true
  fail=$((fail + 1))
  failures+=("${label}")
  return 1
}

cart_remove_added() {
  local venue_id="$1" item_id="$2"
  local label="cart remove"
  local out="${SMOKE_DIR}/cart_remove.json"
  local err="${SMOKE_DIR}/cart_remove.err"
  printf "[%s] %-22s ... " "$(date -u +%H:%M:%S)" "${label}"
  if "${WOLT_BIN}" cart remove "${item_id}" \
    --count 1 \
    --venue-id "${venue_id}" \
    --format json >"${out}" 2>"${err}"; then
    if is_cart_remove_envelope "${out}" "${item_id}" "${venue_id}"; then
      printf "ok (removed 1)\n"
      pass=$((pass + 1))
      return 0
    fi
    printf "FAIL (invalid cart-remove envelope)\n"
  else
    local rc=$?
    printf "FAIL (exit %d)\n" "${rc}"
  fi
  {
    jq -r '.error // empty | "code:    \(.code // "-")\nmessage: \(.message // "-")"' "${out}" 2>/dev/null
    head -10 "${err}" 2>/dev/null
  } | redact | sed 's/^/    | /' | head -20 || true
  fail=$((fail + 1))
  failures+=("${label}")
  return 1
}

# seed_config_from_env — when CI hands us the full config blob, write
# it to ~/.wolt/.wolt-config.json with owner-only perms. Roundtrip
# through jq so we (a) validate it's well-formed JSON before disk
# touch, (b) overwrite the local location with the configured smoke
# coordinates for stable results.
seed_config_from_env() {
  if [ -z "${WOLT_SMOKE_CONFIG_JSON:-}" ]; then
    return 0
  fi
  mkdir -p "${HOME}/.wolt"
  umask 077
  printf '%s' "${WOLT_SMOKE_CONFIG_JSON}" \
    | jq --argjson lat "${SMOKE_LAT}" --argjson lon "${SMOKE_LON}" \
        '.account.location = {lat: $lat, lon: $lon}' \
    >"${HOME}/.wolt/.wolt-config.json"
  chmod 600 "${HOME}/.wolt/.wolt-config.json"
}

seed_config_from_env
unset WOLT_SMOKE_CONFIG_JSON

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
  skip_step "account order" "no orders to drill into"
fi

run "feed summary"  "${WOLT_BIN}" feed --summary
run "top 5"         "${WOLT_BIN}" top 5
run "venues list"   "${WOLT_BIN}" venues --limit 10
run "venues open"   "${WOLT_BIN}" venues --open-now --limit 10

resolved_smoke_venue="${SMOKE_VENUE}"
if [ -z "${resolved_smoke_venue}" ]; then
  resolved_smoke_venue="$(
    jq -r 'first(.data.items[]? | (.slug // .venue_id // empty)) // empty' \
      "${SMOKE_DIR}/venues_list.json" 2>/dev/null
  )"
fi
if [ -n "${resolved_smoke_venue}" ]; then
  run "venue static" "${WOLT_BIN}" venue "${resolved_smoke_venue}"
  run "venue hours"  "${WOLT_BIN}" venue hours "${resolved_smoke_venue}"
  run "venue menu" load_venue_menu "${resolved_smoke_venue}"
  run_mcp_venue_smoke "${resolved_smoke_venue}"
else
  skip_step "venue static" "no venue was returned for the configured location"
  skip_step "venue hours" "no venue was returned for the configured location"
  skip_step "venue menu" "no venue was returned for the configured location"
  skip_step "mcp venue tools" "no venue was returned for the configured location"
fi
run "cart"          "${WOLT_BIN}" cart
run "cart count"    "${WOLT_BIN}" cart count

# ---- cart round-trip (opt-in; mutating) ---------------------------
# Add one currently orderable item, preview checkout, then remove exactly one
# unit of the same line. The cleanup trap never clears a pre-existing basket.
# Checkout is preview-only; no order is ever placed.
if [ "${RUN_CART_SMOKE}" = "1" ]; then
  echo ""
  echo "-- cart round-trip (mutating) --"

  added_item_id=""
  added_venue_id=""
  cleanup_added_item() {
    if [ -n "${added_item_id}" ] && [ -n "${added_venue_id}" ]; then
      "${WOLT_BIN}" cart remove "${added_item_id}" \
        --count 1 \
        --venue-id "${added_venue_id}" \
        --format json >/dev/null 2>&1 || true
    fi
  }
  trap cleanup_added_item EXIT

  fixture=""
  if fixture="$(discover_cart_fixture)"; then
    IFS=$'\t' read -r fixture_venue_id fixture_item_id <<<"${fixture}"
    # The fixture item was verified absent, so pre-arming exact cleanup is safe
    # even when the add mutates upstream and then exits or renders incorrectly.
    added_item_id="${fixture_item_id}"
    added_venue_id="${fixture_venue_id}"
    if cart_add_discovered "${fixture_venue_id}" "${fixture_item_id}"; then
      run_validated \
        "checkout preview" \
        is_checkout_preview_result \
        "${WOLT_BIN}" checkout --venue-id "${added_venue_id}"
      # Same basket, via the MCP tool — covers the handler the CLI path doesn't.
      run_mcp_checkout_preview "${added_venue_id}"
      if cart_remove_added "${added_venue_id}" "${added_item_id}"; then
        added_item_id=""
        added_venue_id=""
      fi
    fi
  else
    fixture_status=$?
    if [ "${fixture_status}" -eq 1 ]; then
      skip_step "cart round-trip" "no currently orderable option-free item was available"
    else
      printf "[%s] %-22s ... FAIL (one or more fixture checks failed unexpectedly)\n" \
        "$(date -u +%H:%M:%S)" "cart fixture"
      redact <"${SMOKE_DIR}/cart_fixture_failures.err" | sed 's/^/    | /' | head -20 || true
      fail=$((fail + 1))
      failures+=("cart fixture")
    fi
  fi
fi

# ---- summary -------------------------------------------------------

echo ""
echo "== summary =="
echo "passed: ${pass}"
echo "failed: ${fail}"
echo "skipped: ${skipped}"
if [ "${fail}" -gt 0 ]; then
  printf "failed steps: %s\n" "$(IFS=', '; echo "${failures[*]}")"
  exit 1
fi
