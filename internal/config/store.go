package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mekedron/wolt-cli/internal/domain"
)

const (
	defaultDirName  = ".wolt"
	defaultFileName = ".wolt-config.json"
	envConfigPath   = "WOLT_CONFIG_PATH"
	lockRetryDelay  = 25 * time.Millisecond
)

var (
	// ErrConfigNotFound is returned when config file does not exist.
	ErrConfigNotFound = errors.New("config file not found")
	// ErrInvalidConfig is returned when config payload is malformed.
	ErrInvalidConfig = errors.New("config file is invalid")
)

// Store loads and writes profile configuration.
type Store struct {
	path string
	mu   sync.RWMutex
}

// Mutator updates a configuration while Store holds its cross-process write
// lock. It returns whether the resulting configuration should be persisted.
type Mutator func(cfg *domain.Config) (bool, error)

// NewStore creates a store using env overrides or defaults.
func NewStore() (*Store, error) {
	if cfg := os.Getenv(envConfigPath); cfg != "" {
		return &Store{path: cfg}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return &Store{path: filepath.Join(home, defaultDirName, defaultFileName)}, nil
}

// Path returns current config path.
func (s *Store) Path() string {
	return s.path
}

// Load reads and validates configuration.
func (s *Store) Load(_ context.Context) (domain.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (domain.Config, error) {
	payload, err := readConfigFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Config{}, ErrConfigNotFound
		}
		return domain.Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg domain.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return domain.Config{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	normalized, err := normalizeSingleAccountConfig(cfg)
	if err != nil {
		return domain.Config{}, err
	}
	return normalized, nil
}

func normalizeSingleAccountConfig(cfg domain.Config) (domain.Config, error) {
	// Profiles[0] is the canonical in-memory representation; Account is the
	// on-disk shape. After Load, both are populated and mirror each other. Any
	// in-memory mutation goes through Profiles[0], so prefer it on Save —
	// otherwise stale Account values would silently overwrite rotated tokens.
	if len(cfg.Profiles) > 0 {
		selected := cfg.Profiles[0]
		for _, profile := range cfg.Profiles {
			if profile.IsDefault {
				selected = profile
				break
			}
		}
		selected.Name = "default"
		selected.IsDefault = true
		return domain.Config{Account: accountFromProfile(selected), Profiles: []domain.Profile{selected}}, nil
	}
	if accountHasData(cfg.Account) {
		profile := profileFromAccount(cfg.Account)
		return domain.Config{Account: accountFromProfile(profile), Profiles: []domain.Profile{profile}}, nil
	}
	return domain.Config{}, fmt.Errorf("%w: profiles is empty", ErrInvalidConfig)
}

// Save writes a configuration payload.
func (s *Store) Save(ctx context.Context, cfg domain.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := s.acquireWriteLock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.saveUnlocked(cfg)
}

// Update applies a read-modify-write transaction under both the in-process
// mutex and a cross-process lock. A missing file is presented as an empty
// Config so explicit login can create it atomically.
func (s *Store) Update(ctx context.Context, mutate Mutator) error {
	if mutate == nil {
		return fmt.Errorf("config mutator is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := s.acquireWriteLock(ctx)
	if err != nil {
		return err
	}
	defer release()

	cfg, err := s.loadUnlocked()
	if err != nil {
		if !errors.Is(err, ErrConfigNotFound) {
			return err
		}
		cfg = domain.Config{}
	}
	changed, err := mutate(&cfg)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return s.saveUnlocked(cfg)
}

func (s *Store) saveUnlocked(cfg domain.Config) error {
	normalized, err := normalizeSingleAccountConfig(cfg)
	if err != nil {
		return err
	}
	if !accountHasData(normalized.Account) {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty config: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	payload, err := json.Marshal(domain.Config{Account: normalized.Account})
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temp.Write(payload); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := replaceFile(tempPath, s.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	_ = os.Chmod(s.path, 0o600)
	return nil
}

func (s *Store) acquireWriteLock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	lockPath := s.path + ".lock"
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, acquired, err := tryAcquireConfigFileLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("acquire config lock: %w", err)
		}
		if acquired {
			return func() {
				_ = release()
			}, nil
		}
		timer := time.NewTimer(lockRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func accountHasData(account domain.Account) bool {
	return account.WToken != "" ||
		account.WRefreshToken != "" ||
		len(account.Cookies) > 0 ||
		account.WoltAddressID != "" ||
		account.Location.Lat != 0 ||
		account.Location.Lon != 0
}

func profileFromAccount(account domain.Account) domain.Profile {
	return domain.Profile{
		Name:          "default",
		IsDefault:     true,
		Location:      account.Location,
		WToken:        account.WToken,
		WRefreshToken: account.WRefreshToken,
		Cookies:       account.Cookies,
		WoltAddressID: account.WoltAddressID,
	}
}

func accountFromProfile(profile domain.Profile) domain.Account {
	return domain.Account{
		Location:      profile.Location,
		WToken:        profile.WToken,
		WRefreshToken: profile.WRefreshToken,
		Cookies:       profile.Cookies,
		WoltAddressID: profile.WoltAddressID,
	}
}
