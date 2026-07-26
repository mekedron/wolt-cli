package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
	"github.com/mekedron/wolt-cli/internal/service/venueresolve"
)

var venueSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func looksLikeObjectID(value string) bool {
	return domain.IsObjectID(value)
}

// venueRef captures the parts of a venue identity that downstream API calls
// might need. After resolution, ID is always populated when possible.
type venueRef struct {
	Input         string
	ID            string
	Slug          string
	StaticPayload map[string]any
	Conflict      bool
}

// normalizeVenueInput strips a full Wolt URL down to the segment immediately
// after venue/restaurant. This also handles copy-pasted item/category URLs,
// whose last path segment is not the venue identity.
func normalizeVenueInput(raw string) string {
	return venueresolve.Normalize(raw)
}

// resolveVenueRef turns whatever the LLM passed (slug / object ID / URL) into a
// {ID, Slug} pair. The supported static venue page accepts both slugs and raw
// IDs, so it is the primary source in both directions. The retired restaurant
// endpoint is deliberately not used: it consistently returns HTTP 410 and can
// obscure otherwise valid closed or scheduled-order venues.
func (tc *ToolCtx) resolveVenueRef(ctx context.Context, raw string) (venueRef, error) {
	input := normalizeVenueInput(raw)
	if input == "" {
		return venueRef{}, fmt.Errorf("venue identifier is required (slug, id, or wolt.com URL)")
	}
	resolved := venueresolve.Resolve(ctx, tc.wolt, raw, venueresolve.Options{})
	return venueRef{
		Input:         resolved.Input,
		ID:            resolved.ID,
		Slug:          resolved.Slug,
		StaticPayload: resolved.StaticPayload,
	}, nil
}

func applyVenueIdentity(ref *venueRef, payload map[string]any) {
	if ref == nil {
		return
	}
	identity := observability.ExtractVenueIdentity(
		observability.VenueIdentity{ID: ref.ID, Slug: ref.Slug},
		payload,
	)
	if identity.ID != "" {
		ref.ID = strings.TrimSpace(identity.ID)
	}
	if identity.Slug != "" {
		ref.Slug = strings.TrimSpace(identity.Slug)
	}
}

// requestVenuePageDynamic retries the public dynamic venue page anonymously
// only when Wolt rejects optional credentials. Other failures retain their
// original classification.
func (tc *ToolCtx) requestVenuePageDynamic(
	ctx context.Context,
	slug string,
	options woltgateway.VenuePageDynamicOptions,
) (map[string]any, error) {
	return venueresolve.RequestDynamic(ctx, tc.wolt, slug, options)
}

// resolveVenueRefWithSearch extends direct slug/ID/URL resolution with the
// search endpoint's venue section. The latter is independent of the discovery
// feed and therefore includes closed and scheduled-order venues.
func (tc *ToolCtx) resolveVenueRefWithSearch(
	ctx context.Context,
	raw string,
	location domain.Location,
) (venueRef, map[string]any, error) {
	ref, err := tc.resolveVenueRef(ctx, raw)
	if err != nil {
		return venueRef{}, nil, err
	}
	input := normalizeVenueInput(raw)
	// A supported dynamic venue page can resolve a direct slug even when the
	// static page is temporarily unavailable. Once both canonical identifiers
	// are known, search/discovery would only add an avoidable failure mode.
	if isDirectVenueReference(raw, input) &&
		looksLikeObjectID(ref.ID) &&
		strings.TrimSpace(ref.Slug) != "" {
		return ref, nil, nil
	}
	if tc.wolt == nil {
		return ref, nil, fmt.Errorf("venue resolver is unavailable")
	}

	query := strings.TrimSpace(raw)
	if domain.CanonicalVenueURL(raw, input) != "" {
		query = input
	}
	payload, searchErr := tc.wolt.Search(ctx, location, query)
	var candidates []map[string]any
	if searchErr == nil {
		candidates = observability.ExtractVenueSearchCandidates(payload)
	}

	exact := exactVenueCandidates(input, query, candidates)
	var discoveryErr error
	if len(exact) == 0 {
		// Wolt's dedicated search and discovery endpoints do not always expose
		// the same venues. Keep dedicated search as the primary source (it can
		// find closed/scheduled venues), then accept only an exact identity
		// match from discovery as a compatibility fallback.
		var items []domain.Item
		items, discoveryErr = tc.wolt.Items(ctx, location)
		if discoveryErr == nil {
			discoveryCandidates := discoveryVenueCandidates(items)
			exact = exactVenueCandidates(input, query, discoveryCandidates)
		}
		if len(exact) == 0 {
			if searchErr != nil {
				return ref, nil, searchErr
			}
			if discoveryErr != nil {
				return ref, nil, fmt.Errorf("exact venue discovery fallback unavailable: %w", discoveryErr)
			}
		}
	}
	if len(exact) == 0 {
		return ref, nil, fmt.Errorf("venue not found by exact name, slug, id, or URL: %s", raw)
	}
	if len(exact) > 1 {
		return ref, nil, fmt.Errorf("venue reference is ambiguous: %s matched %d venues", raw, len(exact))
	}

	candidate := exact[0]
	resolved, resolveErr := tc.resolveVenueRef(ctx, firstNonEmpty(
		asString(candidate["slug"]),
		asString(candidate["venue_id"]),
	))
	if resolveErr != nil {
		return ref, nil, resolveErr
	}
	applyVenueIdentity(&resolved, candidate)
	return resolved, candidate, nil
}

func isDirectVenueReference(raw string, normalized string) bool {
	if domain.IsObjectID(normalized) {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Host != "" {
		return true
	}
	return venueSlugPattern.MatchString(strings.TrimSpace(normalized))
}

func discoveryVenueCandidates(items []domain.Item) []map[string]any {
	candidates := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.Venue == nil {
			continue
		}
		venueID := strings.TrimSpace(domain.NormalizeID(item.Venue.ID))
		if venueID == "" {
			venueID = domain.NormalizeObjectID(item.Link.Target)
		}
		slug := strings.TrimSpace(item.Venue.Slug)
		if venueID == "" && slug == "" {
			continue
		}
		candidate := map[string]any{
			"venue_id": venueID,
			"slug":     slug,
			"name":     firstNonEmpty(item.Title, item.Venue.Name),
			"address":  item.Venue.Address,
			"currency": item.Venue.Currency,
		}
		if item.Venue.Online != nil {
			candidate["order_now_available"] = *item.Venue.Online
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func exactVenueCandidates(input string, raw string, candidates []map[string]any) []map[string]any {
	exact := make([]map[string]any, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if !equalVenueIdentifier(input, candidate) && !equalVenueIdentifier(raw, candidate) {
			continue
		}
		key := strings.ToLower(firstNonEmpty(
			asString(candidate["venue_id"]),
			asString(candidate["slug"]),
		))
		if key == "" {
			key = strings.ToLower(strings.Join([]string{
				asString(candidate["name"]),
				asString(candidate["address"]),
			}, "\x00"))
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		exact = append(exact, candidate)
	}
	return exact
}

func equalVenueIdentifier(input string, candidate map[string]any) bool {
	needle := strings.TrimSpace(input)
	if needle == "" {
		return false
	}
	for _, value := range []string{
		asString(candidate["venue_id"]),
		asString(candidate["slug"]),
		asString(candidate["name"]),
		asString(candidate["canonical_url"]),
		normalizeVenueInput(asString(candidate["canonical_url"])),
	} {
		if strings.EqualFold(needle, strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}
