package config

import (
	"context"
	"errors"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// Manager is the minimum configuration surface needed by ApplyUpdate.
type Manager interface {
	Load(ctx context.Context) (domain.Config, error)
	Save(ctx context.Context, cfg domain.Config) error
}

type atomicUpdater interface {
	Update(ctx context.Context, mutate Mutator) error
}

// ApplyUpdate uses Store's atomic transaction when available and retains a
// load/save fallback for alternate managers used by embedders and tests.
func ApplyUpdate(ctx context.Context, manager Manager, mutate Mutator) error {
	if updater, ok := manager.(atomicUpdater); ok {
		return updater.Update(ctx, mutate)
	}

	cfg, err := manager.Load(ctx)
	if err != nil {
		if !errors.Is(err, ErrConfigNotFound) {
			return err
		}
		cfg = domain.Config{}
	}
	changed, err := mutate(&cfg)
	if err != nil || !changed {
		return err
	}
	return manager.Save(ctx, cfg)
}
