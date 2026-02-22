package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
)

var (
	// ErrDefaultProfileNotFound indicates config has no default profile.
	ErrDefaultProfileNotFound = errors.New("no default profile found")
	// ErrProfileNotFound indicates requested profile does not exist.
	ErrProfileNotFound = errors.New("profile not found")
)

// Loader provides config payloads.
type Loader interface {
	Load(ctx context.Context) (domain.Config, error)
}

// Resolver resolves profile names.
type Resolver struct {
	loader Loader
}

// NewResolver creates a profile resolver.
func NewResolver(loader Loader) *Resolver {
	return &Resolver{loader: loader}
}

// Find resolves explicit profile names or defaults.
func (r *Resolver) Find(ctx context.Context, profileName string) (domain.Profile, error) {
	cfg, err := r.loader.Load(ctx)
	if err != nil {
		return domain.Profile{}, err
	}
	if strings.TrimSpace(profileName) == "" {
		for _, profile := range cfg.Profiles {
			if profile.IsDefault {
				return profile, nil
			}
		}
		return domain.Profile{}, ErrDefaultProfileNotFound
	}

	want := strings.ToLower(strings.TrimSpace(profileName))
	for _, profile := range cfg.Profiles {
		if strings.ToLower(profile.Name) == want {
			return profile, nil
		}
	}
	available := make([]string, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		available = append(available, profile.Name)
	}
	return domain.Profile{}, fmt.Errorf("%w: %s (available: %s)", ErrProfileNotFound, want, strings.Join(available, ", "))
}

// NewFileResolver constructs a resolver from local config file.
func NewFileResolver() (*Resolver, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	return NewResolver(store), nil
}
