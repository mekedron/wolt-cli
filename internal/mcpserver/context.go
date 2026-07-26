package mcpserver

import (
	"context"
	"log/slog"
	"sync"

	configstore "github.com/mekedron/wolt-cli/internal/config"
	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

// ProfileResolver resolves the default profile from persisted config.
type ProfileResolver interface {
	Find(ctx context.Context, profileName string) (domain.Profile, error)
}

// LocationResolver geocodes free-form address strings.
type LocationResolver interface {
	Get(ctx context.Context, address string) (domain.Location, error)
}

// ConfigStore reads and updates the shared on-disk account configuration.
type ConfigStore interface {
	configstore.Manager
}

// Deps wires runtime dependencies into NewServer.
type Deps struct {
	Wolt     woltgateway.API
	Profiles ProfileResolver
	Location LocationResolver
	Config   ConfigStore
	Version  string
	Locale   string
	Logger   *slog.Logger
}

// ToolCtx is the shared receiver every tool handler hangs off. Keeping it as a
// struct (rather than passing 5 deps through every closure) makes the tool
// files smaller and avoids accidentally varying the dependency surface.
type ToolCtx struct {
	wolt     woltgateway.API
	profiles ProfileResolver
	location LocationResolver
	config   ConfigStore
	version  string
	locale   string
	logger   *slog.Logger

	// tokenRefreshMu serializes access-token rotation. MCP clients can issue
	// several authenticated tools concurrently; without coordination each
	// request can observe the same stale token and rotate it independently.
	tokenRefreshMu        sync.Mutex
	currentAccessToken    string
	currentRefreshToken   string
	supersededTokenHashes map[[32]byte]struct{}
	supersededTokenOrder  [][32]byte
	profileAuthHash       [32]byte
	hasProfileAuthHash    bool
}

func newToolCtx(deps Deps) *ToolCtx {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &ToolCtx{
		wolt:     deps.Wolt,
		profiles: deps.Profiles,
		location: deps.Location,
		config:   deps.Config,
		version:  deps.Version,
		locale:   deps.Locale,
		logger:   logger,
	}
}
