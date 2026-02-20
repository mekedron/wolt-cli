package profile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/mekedron/wolt-cli/internal/service/profile"
)

type stubLoader struct {
	cfg domain.Config
	err error
}

func (s *stubLoader) Load(context.Context) (domain.Config, error) {
	if s.err != nil {
		return domain.Config{}, s.err
	}
	return s.cfg, nil
}

func TestResolverFindDefault(t *testing.T) {
	resolver := profile.NewResolver(&stubLoader{cfg: domain.Config{Profiles: []domain.Profile{{Name: "default", IsDefault: true}}}})
	result, err := resolver.Find(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "default" {
		t.Fatalf("expected default profile, got %s", result.Name)
	}
}

func TestResolverFindNamed(t *testing.T) {
	resolver := profile.NewResolver(&stubLoader{cfg: domain.Config{Profiles: []domain.Profile{{Name: "work"}}}})
	result, err := resolver.Find(context.Background(), "WORK")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "work" {
		t.Fatalf("expected work profile, got %s", result.Name)
	}
}

func TestResolverFindNotFound(t *testing.T) {
	resolver := profile.NewResolver(&stubLoader{cfg: domain.Config{Profiles: []domain.Profile{{Name: "default", IsDefault: true}}}})
	_, err := resolver.Find(context.Background(), "missing")
	if !errors.Is(err, profile.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}
