package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/config"
	locationgateway "github.com/mekedron/wolt-cli/internal/gateway/location"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/profile"
)

var version = "dev"

const (
	defaultWoltHTTPMinInterval = 220 * time.Millisecond
	woltHTTPMinIntervalEnv     = "WOLT_HTTP_MIN_INTERVAL_MS"
)

func main() {
	store, err := config.NewStore()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	deps := cli.Dependencies{
		Wolt: woltgateway.NewClient(
			woltgateway.WithRequestMinInterval(resolveWoltRequestMinInterval()),
		),
		Profiles: profile.NewResolver(store),
		Location: locationgateway.NewClient(),
		Config:   store,
		Version:  version,
	}

	exitCode := cli.Execute(context.Background(), os.Args[1:], deps, os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func resolveWoltRequestMinInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv(woltHTTPMinIntervalEnv))
	if raw == "" {
		return defaultWoltHTTPMinInterval
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return defaultWoltHTTPMinInterval
	}
	return time.Duration(ms) * time.Millisecond
}
