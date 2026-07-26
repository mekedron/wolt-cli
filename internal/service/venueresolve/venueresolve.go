// Package venueresolve contains direct venue identity resolution shared by
// the CLI and MCP server.
package venueresolve

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

// PageAPI is the supported subset of Wolt venue-page operations needed for
// direct slug, ID, and URL resolution.
type PageAPI interface {
	VenuePageStatic(context.Context, string) (map[string]any, error)
	VenuePageDynamic(context.Context, string, woltgateway.VenuePageDynamicOptions) (map[string]any, error)
}

// Result contains the canonical identity recovered from supported venue-page
// payloads. A caller-provided slug is never copied into ID.
type Result struct {
	Input         string
	ID            string
	Slug          string
	StaticPayload map[string]any
}

// Options customizes direct resolution without duplicating its semantics.
// StaticLoader lets the CLI retain its process-local and on-disk caches.
type Options struct {
	StaticLoader func(context.Context, string) (map[string]any, error)
	Dynamic      woltgateway.VenuePageDynamicOptions
}

// Normalize converts a slug, venue ID, or nested Wolt URL into the direct
// upstream reference.
func Normalize(raw string) string {
	return domain.VenueSlugFromReference(raw)
}

// Resolve performs best-effort direct resolution through the supported static
// and dynamic venue-page endpoints. Upstream failures leave unresolved fields
// empty so callers can decide whether their operation may continue.
func Resolve(ctx context.Context, api PageAPI, raw string, options Options) Result {
	input := Normalize(raw)
	result := Result{Input: raw}
	if input == "" {
		return result
	}
	if domain.IsObjectID(input) {
		result.ID = strings.TrimSpace(input)
	} else {
		result.Slug = strings.TrimSpace(input)
	}
	if api == nil {
		return result
	}

	loadStatic := options.StaticLoader
	if loadStatic == nil {
		loadStatic = api.VenuePageStatic
	}
	if payload, err := loadStatic(ctx, input); err == nil {
		result.StaticPayload = payload
		applyPayload(&result, payload)
	}
	if domain.IsObjectID(result.ID) && result.Slug != "" {
		return result
	}
	if payload, err := RequestDynamic(ctx, api, input, options.Dynamic); err == nil {
		applyPayload(&result, payload)
	}
	return result
}

// RequestDynamic retries a public venue-page request anonymously only when
// Wolt rejects optional credentials. Other errors retain their classification.
func RequestDynamic(
	ctx context.Context,
	api PageAPI,
	reference string,
	options woltgateway.VenuePageDynamicOptions,
) (map[string]any, error) {
	if api == nil {
		return nil, errors.New("venue API is unavailable")
	}
	payload, err := api.VenuePageDynamic(ctx, reference, options)
	if err == nil || !options.Auth.HasCredentials() || !woltgateway.HasStatus(err, http.StatusUnauthorized) {
		return payload, err
	}
	options.Auth = woltgateway.AuthContext{}
	return api.VenuePageDynamic(ctx, reference, options)
}

func applyPayload(result *Result, payload map[string]any) {
	if result == nil {
		return
	}
	identity := observability.ExtractVenueIdentity(
		observability.VenueIdentity{ID: result.ID, Slug: result.Slug},
		payload,
	)
	if identity.ID != "" {
		result.ID = strings.TrimSpace(identity.ID)
	}
	if identity.Slug != "" {
		result.Slug = strings.TrimSpace(identity.Slug)
	}
}
