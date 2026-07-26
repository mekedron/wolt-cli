package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
	"github.com/mekedron/wolt-cli/internal/service/venueresolve"
)

type venueReference struct {
	Input     string
	VenueID   string
	VenueSlug string
}

type cartAddVenueReference struct {
	Input                string
	ID                   string
	Slug                 string
	ExplicitSlug         string
	explicitSlugVerified bool
}

func resolveCartAddVenueReference(
	ctx context.Context,
	deps Dependencies,
	rawVenue string,
	rawExplicitSlug string,
) (cartAddVenueReference, error) {
	reference := cartAddVenueReference{
		Input:        normalizeVenueInput(rawVenue),
		ExplicitSlug: normalizeVenueInput(rawExplicitSlug),
	}
	if reference.Input == "" {
		return reference, fmt.Errorf("venue is required")
	}
	if domain.IsObjectID(reference.ExplicitSlug) {
		return reference, fmt.Errorf("--venue-slug must be a venue slug or Wolt venue URL, not a venue id")
	}

	resolved, _ := resolveVenueReference(ctx, deps, reference.Input)
	reference.ID = resolved.VenueID
	reference.Slug = strings.TrimSpace(resolved.VenueSlug)
	reference.explicitSlugVerified = reference.ExplicitSlug == ""
	if reference.ExplicitSlug == "" {
		return reference, nil
	}

	reference.Slug = reference.ExplicitSlug
	if strings.EqualFold(reference.ExplicitSlug, reference.Input) ||
		strings.EqualFold(reference.ExplicitSlug, resolved.VenueSlug) {
		reference.explicitSlugVerified = true
		return reference, nil
	}

	override, _ := resolveVenueReference(ctx, deps, reference.ExplicitSlug)
	if !domain.IsObjectID(override.VenueID) {
		if !domain.IsObjectID(reference.ID) {
			return reference, fmt.Errorf(
				"--venue-slug %q conflicts with venue %q",
				reference.ExplicitSlug,
				strings.TrimSpace(rawVenue),
			)
		}
		return reference, nil
	}
	if !strings.EqualFold(override.VenueID, reference.ID) {
		return reference, fmt.Errorf(
			"--venue-slug %q resolves to a different venue than %q",
			reference.ExplicitSlug,
			strings.TrimSpace(rawVenue),
		)
	}
	reference.explicitSlugVerified = true
	return reference, nil
}

func (reference cartAddVenueReference) basketSelectionKey() string {
	if domain.IsObjectID(reference.ID) {
		return strings.TrimSpace(reference.ID)
	}
	return reference.Input
}

func (reference *cartAddVenueReference) applyItemSlugHint(
	ctx context.Context,
	deps Dependencies,
	rawHint string,
) error {
	hint := normalizeVenueInput(rawHint)
	if hint == "" {
		return nil
	}
	if reference.Slug != "" {
		if !strings.EqualFold(reference.Slug, hint) {
			return fmt.Errorf(
				"item URL venue %q conflicts with venue %q",
				hint,
				reference.Input,
			)
		}
		return nil
	}

	hintReference, _ := resolveVenueReference(ctx, deps, hint)
	if !domain.IsObjectID(hintReference.VenueID) {
		return fmt.Errorf(
			"could not verify that item URL venue %q belongs to venue %q",
			hint,
			reference.Input,
		)
	}
	if !strings.EqualFold(hintReference.VenueID, reference.ID) {
		return fmt.Errorf(
			"item URL venue %q resolves to a different venue than %q",
			hint,
			reference.Input,
		)
	}
	reference.Slug = hint
	return nil
}

func (reference *cartAddVenueReference) verifyBasket(
	identity payloadutil.BasketVenueIdentity,
) error {
	if identity.Conflict {
		return fmt.Errorf("the selected basket contains conflicting canonical venue ids")
	}
	if domain.IsObjectID(reference.ID) &&
		domain.IsObjectID(identity.ID) &&
		!strings.EqualFold(identity.ID, reference.ID) {
		return fmt.Errorf("the selected basket belongs to a different venue")
	}
	if reference.ExplicitSlug != "" &&
		!reference.explicitSlugVerified &&
		identity.Slug != "" {
		if !strings.EqualFold(identity.Slug, reference.ExplicitSlug) {
			return fmt.Errorf("the selected basket venue does not match --venue-slug")
		}
		reference.explicitSlugVerified = true
	}
	return nil
}

// itemReference is the resolved form of any item input accepted by the CLI:
// a 24-char Mongo ObjectID, a Wolt item URL of the form
// `https://wolt.com/<locale>/<country>/<city>/venue/<slug>/itemid-<id>`,
// or the same URL pattern with `/menuitem-<id>` or trailing `?itemid=<id>`.
//
// VenueSlugHint is populated when the input was a URL that carries the
// venue slug, letting callers skip an extra slug lookup.
type itemReference struct {
	Input         string
	ItemID        string
	VenueSlugHint string
}

func resolveItemReference(raw string) itemReference {
	ref := itemReference{Input: raw}
	value := strings.TrimSpace(raw)
	if value == "" {
		return ref
	}
	if looksLikeObjectID(value) {
		ref.ItemID = value
		return ref
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Host != "" && domain.IsWoltURL(value) {
		ref.VenueSlugHint, ref.ItemID = parseItemURL(parsed)
		if ref.ItemID != "" {
			return ref
		}
	}
	// Fall back to the trailing path segment in case the user pasted
	// something URL-shaped that we couldn't fully match.
	if err == nil && parsed.Host != "" && domain.IsWoltURL(value) {
		if candidate := extractTrailingObjectID(parsed.Path); candidate != "" {
			ref.ItemID = candidate
			return ref
		}
	}
	return ref
}

func parseItemURL(parsed *url.URL) (slugHint, itemID string) {
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, segment := range parts {
		if id, ok := extractIDFromSegment(segment); ok {
			itemID = id
			if i > 0 && parts[i-1] != "" {
				if i-1 >= 1 && (parts[i-1] != "venue" && parts[i-1] != "restaurant") {
					slugHint = parts[i-1]
				}
			}
		}
	}
	if itemID == "" {
		// Some URLs encode the item id as a query parameter (e.g. `?itemid=...`).
		for _, key := range []string{"itemid", "item_id", "item"} {
			if value := strings.TrimSpace(parsed.Query().Get(key)); looksLikeObjectID(value) {
				itemID = value
				break
			}
		}
	}
	if slugHint == "" {
		// Best-effort: pick the segment that follows /venue/ or /restaurant/.
		for i, segment := range parts {
			if (segment == "venue" || segment == "restaurant") && i+1 < len(parts) {
				next := strings.TrimSpace(parts[i+1])
				if next != "" && !strings.HasPrefix(next, "itemid-") && !strings.HasPrefix(next, "menuitem-") {
					slugHint = next
				}
				break
			}
		}
	}
	return slugHint, itemID
}

func extractIDFromSegment(segment string) (string, bool) {
	segment = strings.TrimSpace(segment)
	for _, prefix := range []string{"itemid-", "menuitem-", "item-"} {
		if strings.HasPrefix(segment, prefix) {
			candidate := strings.TrimPrefix(segment, prefix)
			if looksLikeObjectID(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// looksURLShaped reports whether a string is plausibly a URL (i.e. contains a
// scheme). Used by cart commands to distinguish "user pasted a broken Wolt
// URL" from "user passed a non-hex item id we should forward to upstream".
func looksURLShaped(value string) bool {
	return strings.Contains(value, "://")
}

func extractTrailingObjectID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		segment := strings.TrimSpace(parts[i])
		if looksLikeObjectID(segment) {
			return segment
		}
		if id, ok := extractIDFromSegment(segment); ok {
			return id
		}
	}
	return ""
}

// itemNameMatch is one candidate item returned from a venue assortment search.
type itemNameMatch struct {
	ID       string
	Name     string
	Category string
}

// resolveItemByName looks up an item inside a single venue's assortment by
// (case-insensitive) name. The caller passes the venue slug, the user's
// query, and the resolved request language. Returns:
//   - the unique match, or
//   - an explanatory error when nothing matched, or
//   - an "ambiguous, did you mean…" error listing the top candidates.
//
// An exact case-insensitive name match always wins, even when other items
// contain the same query as a substring.
func resolveItemByName(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	venueIDForExtract string,
	query string,
	language string,
	auth woltgateway.AuthContext,
) (itemNameMatch, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return itemNameMatch{}, fmt.Errorf("--query cannot be empty")
	}
	payload, err := requestAssortmentItemsSearchPayload(ctx, deps, venueSlug, strings.TrimSpace(query), language, auth)
	if err != nil {
		return itemNameMatch{}, fmt.Errorf("venue item search failed: %w", err)
	}
	items := observability.ExtractMenuItems(payload, venueIDForExtract, venueSlug)
	matches := make([]itemNameMatch, 0, len(items))
	exact := make([]itemNameMatch, 0, 2)
	for _, item := range items {
		id := strings.TrimSpace(asString(item["item_id"]))
		name := strings.TrimSpace(asString(item["name"]))
		if id == "" || name == "" {
			continue
		}
		entry := itemNameMatch{ID: id, Name: name, Category: asString(item["category"])}
		lowered := strings.ToLower(name)
		switch {
		case lowered == needle:
			exact = append(exact, entry)
		case strings.Contains(lowered, needle):
			matches = append(matches, entry)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return itemNameMatch{}, ambiguousItemError(query, venueSlug, exact)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return itemNameMatch{}, ambiguousItemError(query, venueSlug, matches)
	}
	return itemNameMatch{}, fmt.Errorf("no items in %q matched %q; try a broader query or paste the item id", venueSlug, query)
}

func ambiguousItemError(query, venueSlug string, candidates []itemNameMatch) error {
	limit := len(candidates)
	if limit > 5 {
		limit = 5
	}
	hints := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		c := candidates[i]
		hints = append(hints, fmt.Sprintf("%s (%s)", c.Name, c.ID))
	}
	more := ""
	if len(candidates) > limit {
		more = fmt.Sprintf(", and %d more", len(candidates)-limit)
	}
	return fmt.Errorf(
		"%q matched %d items in %q (be more specific or pass the item id): %s%s",
		query,
		len(candidates),
		venueSlug,
		strings.Join(hints, "; "),
		more,
	)
}

// cartItemCandidate is a single orderable menu item resolved by the
// cheapest-in-stock selector (id, display name, and base price in minor units).
type cartItemCandidate struct {
	ID    string
	Name  string
	Price int
}

// selectCheapestMenuItem picks the cheapest orderable item from a normalized
// menu (the shape produced by observability.ExtractMenuItems / emitted by
// `venue menu`). "Orderable" means in stock (not sold out) and carrying a real
// positive base price — a zero price can't be posted to a basket. When query is
// non-empty only items whose name contains it (case-insensitive) are
// considered; an empty query takes the venue's cheapest orderable item. Ties on
// price are broken by name so the choice is deterministic across runs. Returns
// false when nothing orderable matched.
//
// Unlike resolveItemByName this never fails on multiple matches. That makes it
// useful for unattended scripts which need a deterministic orderable choice
// without pinning a volatile item id.
func selectCheapestMenuItem(items []map[string]any, query string) (cartItemCandidate, bool) {
	needle := strings.ToLower(strings.TrimSpace(query))
	var best cartItemCandidate
	found := false
	for _, item := range items {
		if available, exists := item["is_available"]; (exists && !asBool(available)) ||
			(!exists && asBool(item["is_sold_out"])) {
			continue
		}
		price := asInt(asMap(item["base_price"])["amount"])
		if price <= 0 {
			continue
		}
		id := strings.TrimSpace(asString(item["item_id"]))
		name := strings.TrimSpace(asString(item["name"]))
		if id == "" || name == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
			continue
		}
		if !found || price < best.Price || (price == best.Price && name < best.Name) {
			best = cartItemCandidate{ID: id, Name: name, Price: price}
			found = true
		}
	}
	return best, found
}

// resolveCheapestItem fetches a venue's assortment and returns its cheapest
// orderable item, optionally constrained to names containing query. It powers
// `cart add --cheapest`: a deterministic, sold-out-aware alternative to
// resolveItemByName that never errors on ambiguity. Returns an explanatory
// error only when nothing orderable matched.
func resolveCheapestItem(ctx context.Context, deps Dependencies, venueSlug, venueIDForExtract, query string) (cartItemCandidate, error) {
	if deps.Wolt == nil {
		return cartItemCandidate{}, fmt.Errorf("menu lookup unavailable")
	}
	payload, err := deps.Wolt.AssortmentByVenueSlug(ctx, venueSlug)
	if err != nil {
		return cartItemCandidate{}, fmt.Errorf("venue menu lookup failed: %w", err)
	}
	items := observability.ExtractMenuItems(payload, venueIDForExtract, venueSlug)
	candidate, ok := selectCheapestMenuItem(items, query)
	if !ok {
		if strings.TrimSpace(query) != "" {
			return cartItemCandidate{}, fmt.Errorf("no in-stock item in %q matched %q; broaden --query or drop it to take the venue's cheapest item", venueSlug, query)
		}
		return cartItemCandidate{}, fmt.Errorf("no in-stock items found in %q", venueSlug)
	}
	return candidate, nil
}

func normalizeVenueInput(raw string) string {
	return venueresolve.Normalize(raw)
}

func venueIdentityFromInput(raw string) observability.VenueIdentity {
	normalized := normalizeVenueInput(raw)
	identity := observability.VenueIdentity{
		CanonicalURL: domain.CanonicalVenueURL(raw, normalized),
	}
	if looksLikeObjectID(normalized) {
		identity.ID = normalized
	} else {
		identity.Slug = normalized
	}
	return identity
}

func loadVenuePageDynamic(
	ctx context.Context,
	deps Dependencies,
	reference string,
	options woltgateway.VenuePageDynamicOptions,
) (map[string]any, error) {
	return venueresolve.RequestDynamic(ctx, deps.Wolt, reference, options)
}

func resolveVenueReference(ctx context.Context, deps Dependencies, raw string) (venueReference, error) {
	input := normalizeVenueInput(raw)
	ref := venueReference{Input: raw}
	if input == "" {
		return ref, nil
	}
	// The lightweight ID cache preserves the resolver's fast path. Commands
	// that need canonical metadata load the cached static payload separately.
	if !domain.IsObjectID(input) {
		if cachedID, ok := lookupCachedVenueID(input); ok {
			ref.VenueID = cachedID
			ref.VenueSlug = input
			return ref, nil
		}
	}
	resolved := venueresolve.Resolve(ctx, deps.Wolt, raw, venueresolve.Options{
		StaticLoader: func(ctx context.Context, reference string) (map[string]any, error) {
			if deps.Wolt == nil {
				return nil, fmt.Errorf("venue API is unavailable")
			}
			if domain.IsObjectID(reference) {
				return deps.Wolt.VenuePageStatic(ctx, reference)
			}
			return cachedVenuePageStatic(ctx, deps, reference)
		},
	})
	ref.VenueID = resolved.ID
	ref.VenueSlug = resolved.Slug
	if !domain.IsObjectID(input) && domain.IsObjectID(ref.VenueID) {
		rememberVenueID(input, ref.VenueID)
	}
	return ref, nil
}

// resolveVenueSlugFromID preserves the lightweight lookup used by cart and
// checkout callers while sharing the canonical static/dynamic page resolver.
// The retired restaurant-detail endpoint is intentionally not consulted.
func resolveVenueSlugFromID(ctx context.Context, deps Dependencies, venueID string) string {
	ref, _ := resolveVenueReference(ctx, deps, venueID)
	return strings.TrimSpace(ref.VenueSlug)
}
