package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hyperscale/fabric"
	"github.com/hyperscale/fabric/demo/cmd/acme-a-consumer/app"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", slog.Any("error", err)) // nolint: noctx

		os.Exit(1)
	}
}

// run exists because os.Exit skips deferred calls, so the signal handler must be
// released from a function that returns normally.
func run() error {
	ctx, stop := fabric.SignalContext(context.Background())
	defer stop()

	a, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to build application: %w", err)
	}

	// Run boots the providers, blocks until SIGINT or SIGTERM, then drains
	// within the configured shutdown budget.
	if err := a.Run(ctx); err != nil {
		return fmt.Errorf("service run: %w", err)
	}

	return nil
}
