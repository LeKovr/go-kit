// Package server runs concurrent workers under a shared context.
package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// Worker returns nil on normal completion, including cooperative cancellation.
type Worker func(context.Context) error

// Run starts all workers, cancels their context on SIGINT/SIGTERM, parent
// cancellation or the first error, and waits for every worker to finish.
// A successful worker does not cancel its peers.
func Run(ctx context.Context, workers ...Worker) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, workerCtx := errgroup.WithContext(ctx)
	for _, worker := range workers {
		group.Go(func() error { return worker(workerCtx) })
	}

	return group.Wait()
}
