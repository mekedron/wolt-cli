package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/mekedron/wolt-cli/internal/domain"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
)

var unknownCommandPattern = regexp.MustCompile(`unknown command "([^"]+)"`)

// ProfileResolver resolves profile selections.
type ProfileResolver interface {
	Find(ctx context.Context, profileName string) (domain.Profile, error)
}

// LocationResolver resolves addresses to coordinates.
type LocationResolver interface {
	Get(ctx context.Context, address string) (domain.Location, error)
}

// ConfigManager stores profile config payloads.
type ConfigManager interface {
	Path() string
	Load(ctx context.Context) (domain.Config, error)
	Save(ctx context.Context, cfg domain.Config) error
}

// Dependencies wires runtime services.
type Dependencies struct {
	Wolt     woltgateway.API
	Profiles ProfileResolver
	Location LocationResolver
	Config   ConfigManager
	Version  string
}

var errVersionShown = fmt.Errorf("version shown")

// Execute runs the CLI with injected dependencies.
func Execute(ctx context.Context, args []string, deps Dependencies, stdout io.Writer, stderr io.Writer) int {
	cmd := NewRootCommand(deps)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.ExecuteContext(ctx)
	if err == nil || err == errVersionShown {
		return 0
	}
	var controlled *exitError
	if errors.As(err, &controlled) {
		return controlled.code
	}

	if matches := unknownCommandPattern.FindStringSubmatch(err.Error()); len(matches) > 1 {
		_, _ = fmt.Fprintf(stderr, "No such command '%s'\n", matches[1])
		return 2
	}

	if msg := err.Error(); msg != "" {
		_, _ = fmt.Fprintln(stderr, msg)
	}
	return 1
}
