# Workflows

## 1) Authenticate and Validate Session

```bash
wolt configure --profile-name default --wtoken "<token>" --wrtoken "<refresh-token>" --overwrite
wolt profile status --profile default --format json --verbose
wolt profile show --profile default --format json
```

If status fails with upstream/auth errors, keep `--verbose` and surface exact error code/message.

## 2) Find Venue, Inspect Item, Add to Cart, Preview Checkout

```bash
# Discover/search
wolt search venues --profile default --query "burger" --limit 10 --format json
wolt venue show <venue-slug> --profile default --format json

# Resolve item and options
wolt venue search <venue-slug> --query "whopper" --include-options --limit 10 --profile default --format json
wolt item options <venue-slug> <item-id> --profile default --format json

# Mutation (confirm with user first)
wolt cart add <venue-id> <item-id> --venue-slug <venue-slug> --option "<group-id>=<value-id>" --profile default --format json

# Validate basket and pricing preview
wolt cart show --venue-id <venue-id> --details --profile default --format json
wolt checkout preview --venue-id <venue-id> --delivery-mode standard --profile default --format json
```

## 3) Large Marketplace Venue Strategy (Partial Assortments)

Use this path when `venue menu` is incomplete or returns partial-assortment guidance:

```bash
wolt venue categories <venue-slug> --profile default --format json
wolt venue search <venue-slug> --query "milk" --profile default --format json
wolt venue menu <venue-slug> --category <category-slug> --include-options --profile default --format json
```

Use `--full-catalog` only when explicitly needed; it can be slow.

## 4) Orders and Payment/Profile Inspection

```bash
wolt profile orders --profile default --limit 20 --format json
wolt profile orders show <purchase-id> --profile default --format json
wolt profile payments --profile default --mask-sensitive --format json
wolt profile favorites --profile default --format json
```

## 5) Address Book Operations (Mutating)

Confirm intent before add/update/remove/use.

```bash
wolt profile addresses --profile default --format json
wolt profile addresses add --profile default --address "<formatted>" --lat <lat> --lon <lon> --label home --format json
wolt profile addresses links --profile default --format json
wolt profile addresses use <address-id> --profile default --format json
```

## 6) Location Override Rules

- Valid: `--address "Kamppi, Helsinki"`
- Valid: `--lat 60.1699 --lon 24.9384`
- Invalid: `--address ... --lat ... --lon ...` together
- Invalid: only one coordinate flag

On invalid combinations, expect `WOLT_INVALID_ARGUMENT`.
