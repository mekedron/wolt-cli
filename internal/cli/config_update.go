package cli

import (
	"context"
	"strings"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

type credentialPersistence struct {
	manager    ConfigManager
	expected   configstore.Credentials
	configured bool
	enabled    bool
	initErr    error
}

func newCredentialPersistence(
	ctx context.Context,
	deps Dependencies,
	auth woltgateway.AuthContext,
	allow bool,
) *credentialPersistence {
	state := &credentialPersistence{}
	if !allow || deps.Config == nil {
		return state
	}
	state.manager = deps.Config
	state.configured = true

	cfg, err := deps.Config.Load(ctx)
	if err != nil {
		state.initErr = err
		return state
	}
	persisted := configstore.CredentialsFromConfig(cfg)
	current := credentialsFromAuth(auth)
	if !persisted.Equal(current) {
		return state
	}
	state.expected = persisted
	state.enabled = true
	return state
}

func (s *credentialPersistence) persistAccess(
	ctx context.Context,
	accessToken string,
) (attempted bool, persisted bool, err error) {
	if s == nil || !s.configured {
		return false, false, nil
	}
	if s.initErr != nil {
		return true, false, s.initErr
	}
	if !s.enabled {
		return true, false, nil
	}

	swapped := false
	err = configstore.ApplyUpdate(ctx, s.manager, func(cfg *domain.Config) (bool, error) {
		if !configstore.CompareAndSwapAccess(cfg, s.expected, accessToken) {
			return false, nil
		}
		swapped = true
		return true, nil
	})
	if err != nil {
		return true, false, err
	}
	if !swapped {
		s.enabled = false
		return true, false, nil
	}
	s.expected.AccessToken = strings.TrimSpace(accessToken)
	return true, true, nil
}

func (s *credentialPersistence) disable() {
	if s == nil {
		return
	}
	s.configured = false
	s.enabled = false
	s.initErr = nil
}

func credentialsFromAuth(auth woltgateway.AuthContext) configstore.Credentials {
	return configstore.Credentials{
		AccessToken:  auth.WToken,
		RefreshToken: auth.RefreshToken,
		Cookies:      append([]string(nil), auth.Cookies...),
	}
}

func allowAutomaticCredentialPersistence(flags globalFlags) bool {
	return strings.TrimSpace(flags.WToken) == "" &&
		strings.TrimSpace(flags.WRefreshToken) == "" &&
		len(flags.Cookies) == 0
}
