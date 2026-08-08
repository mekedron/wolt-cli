# Workflows

## 1) Authenticate and Validate Session

```bash
# Browser-driven login (default): opens managed Chrome at 127.0.0.1:9222,
# waits for the user to sign in to wolt.com, then extracts auth cookies.
wolt login

# Manual token login (no browser): pass tokens directly.
wolt login --wtoken "<token>" --wrtoken "<refresh-token>"

wolt status --format json --verbose
wolt account --format json
```

If any auth-gated command fails because the session expired, the CLI now exits with `WOLT_AUTH_REQUIRED` and the message "Your Wolt session expired or is missing. Run \"wolt login\" to refresh." — direct the user to re-run `wolt login`.

## 1a) Quickest "What Should I Eat?" Loop

```bash
wolt top 10                         # single ranked table, no jq
wolt feed --summary                 # one line per section overview
wolt feed --query "burger"          # filter the feed (matches brand carousels too)
```

## 2) Find Venue, Inspect Item, Add to Cart, Preview Checkout

```bash
# Discover/search
wolt venues  --query "burger" --limit 10 --format json
wolt items --query "whopper" --limit 20 --format json
wolt venue <venue-slug>  --format json

# Resolve item and options
wolt venue menu <venue-slug> --query "whopper" --include-options --limit 10  --format json
wolt venue item <venue-slug> <item-id>  --format json

# Mutation (confirm with user first)
wolt cart add <venue-id> <item-id> --venue-slug <venue-slug> --option "<group-id>=<value-id>"  --format json

# Validate basket and pricing preview
wolt cart --venue-id <venue-id> --details  --format json
wolt checkout --venue-id <venue-id> --delivery-mode standard  --format json
```

## 3) Large Marketplace Venue Strategy (Partial Assortments)

Use this path when `venue menu` is incomplete or returns partial-assortment guidance:

```bash
wolt venue categories <venue-slug>  --format json
wolt venue menu <venue-slug> --query "milk"  --format json
wolt venue menu <venue-slug> --category <category-slug> --include-options  --format json
```

Use `--full-catalog` only when explicitly needed; it can be slow.

## 4) Orders and Payment/Profile Inspection

```bash
wolt account orders  --limit 20 --format json
wolt account order <purchase-id>  --format json
wolt account payments  --mask-sensitive --format json
wolt account favorites  --format json
```

## 5) Address Book Operations (Mutating)

Confirm intent before add/update/remove/use.

```bash
wolt account addresses  --format json
wolt account addresses add  --address "<formatted>" --lat <lat> --lon <lon> --label home --format json
wolt account addresses links  --format json
wolt account addresses use <address-id>  --format json
```

## 6) Location Override Rules

- Valid: `--address "Kamppi, Helsinki"`
- Valid: `--lat 60.1699 --lon 24.9384`
- Invalid: `--address ... --lat ... --lon ...` together
- Invalid: only one coordinate flag

On invalid combinations, expect `WOLT_INVALID_ARGUMENT`.
