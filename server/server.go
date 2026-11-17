// Package server runs concurrent workers under a shared context.
package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Worker returns nil on normal completion, including cooperative cancellation.
type Worker func(context.Context) error

// Config controls coordinated shutdown. Defaults are applied by config.Open.
type Config struct {
	// GracePeriod bounds graceful shutdown, not the return of uncooperative workers.
	// Zero expires immediately; negative values are rejected by Run.
	GracePeriod time.Duration `default:"10s" description:"Shutdown timeout" long:"grace" env:"GRACE"`
}

// Service coordinates workers and parallel shutdown callbacks.
// Configure it before calling Run.
type Service struct {
	config     Config
	onShutdown []Worker
	started    atomic.Bool
}

func New(cfg Config) *Service {
	return &Service{config: cfg}
}

// WithShutdown replaces the callbacks; an empty list clears them. Call before Run.
// Each callback must initiate shutdown itself, without waiting for worker cancellation.
func (srv *Service) WithShutdown(workers ...Worker) *Service {
	srv.onShutdown = append([]Worker(nil), workers...)
	return srv
}

// Run starts workers once. A signal, parent cancellation, worker error or completion
// of all workers starts all shutdown callbacks with one shared GracePeriod deadline.
// Worker contexts are canceled after callbacks finish or the deadline expires.
// Run waits for every worker and callback, even beyond the deadline, and joins errors.
func (srv *Service) Run(ctx context.Context, workers ...Worker) error {
	if srv.config.GracePeriod < 0 {
		return errors.New("shutdown grace period must not be negative")
	}

	for i, worker := range workers {
		if worker == nil {
			return fmt.Errorf("worker %d is nil", i)
		}
	}

	for i, worker := range srv.onShutdown {
		if worker == nil {
			return fmt.Errorf("shutdown %d is nil", i)
		}
	}

	if !srv.started.CompareAndSwap(false, true) {
		return errors.New("service already run")
	}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	workerCtx, cancelWorkers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWorkers()

	errs := make([]error, len(workers)+len(srv.onShutdown))
	failed := make(chan struct{}, 1)

	var runs sync.WaitGroup

	for i, worker := range workers {
		runs.Go(func() {
			if err := worker(workerCtx); err != nil {
				errs[i] = fmt.Errorf("worker %d: %w", i, err)

				select {
				case failed <- struct{}{}:
				default:
				}
			}
		})
	}

	runDone := make(chan struct{})
	if len(workers) == 0 {
		close(runDone)
	} else {
		go func() { runs.Wait(); close(runDone) }()
	}

	select {
	case <-signalCtx.Done():
	case <-failed:
	case <-runDone:
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), srv.config.GracePeriod)
	defer cancel()

	var shutdowns sync.WaitGroup

	for i, worker := range srv.onShutdown {
		shutdowns.Go(func() {
			if err := worker(shutdownCtx); err != nil {
				errs[len(workers)+i] = fmt.Errorf("shutdown %d: %w", i, err)
			}
		})
	}

	shutdownDone := make(chan struct{})
	if len(srv.onShutdown) == 0 {
		close(shutdownDone)
	} else {
		go func() { shutdowns.Wait(); close(shutdownDone) }()
	}

	var deadlineErr error

	wait := func(done <-chan struct{}) {
		select {
		case <-done:
			return
		default:
		}

		select {
		case <-done:
		case <-shutdownCtx.Done():
			deadlineErr = shutdownCtx.Err()

			cancelWorkers()
			<-done
		}
	}
	wait(shutdownDone)
	cancelWorkers()
	wait(runDone)

	return errors.Join(append(errs, deadlineErr)...)
}
