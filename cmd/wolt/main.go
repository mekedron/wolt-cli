package main

import (
	"context"
	"os"

	"github.com/mekedron/wolt-cli/internal/cli"
	"github.com/mekedron/wolt-cli/internal/config"
	locationgateway "github.com/mekedron/wolt-cli/internal/gateway/location"
	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/profile"
)

var version = "dev"

func main() {
	store, err := config.NewStore()
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	deps := cli.Dependencies{
		Wolt:     woltgateway.NewClient(),
		Profiles: profile.NewResolver(store),
		Location: locationgateway.NewClient(),
		Config:   store,
		Version:  version,
	}

	exitCode := cli.Execute(context.Background(), os.Args[1:], deps, os.Stdout, os.Stderr)
	os.Exit(exitCode)
}
