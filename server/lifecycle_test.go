package server

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestParallelShutdownPreservesWorkerContextAndErrors(t *testing.T) {
	prepareSignals(t)
	synctest.Test(t, func(t *testing.T) {
		type key struct{}

		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "value"))
		defer cancel()

		started := make(chan context.Context, 1)
		callbacks := make(chan context.Context, 2)
		release := make(chan struct{})
		errRun, errStop1, errStop2 := errors.New("run"), errors.New("stop1"), errors.New("stop2")
		srv := New(Config{GracePeriod: time.Minute}).WithShutdown(
			func(ctx context.Context) error { callbacks <- ctx; <-release; return errStop1 },
			func(ctx context.Context) error { callbacks <- ctx; <-release; return errStop2 },
		)

		done := make(chan error, 1)
		go func() {
			done <- srv.Run(ctx, func(ctx context.Context) error {
				started <- ctx

				<-ctx.Done()

				return errors.Join(context.Canceled, errRun)
			})
		}()

		workerCtx := <-started

		cancel()

		first, second := <-callbacks, <-callbacks
		if first != second || first.Err() != nil || first.Value(key{}) != "value" {
			t.Fatal("callbacks did not receive one fresh context with parent values")
		}

		if _, ok := first.Deadline(); !ok {
			t.Fatal("missing shutdown deadline")
		}

		if _, ok := workerCtx.Deadline(); ok {
			t.Fatal("worker inherited a deadline")
		}

		if workerCtx.Err() != nil || workerCtx.Value(key{}) != "value" {
			t.Fatal("worker context canceled before shutdown finished or values lost")
		}

		close(release)

		err := <-done
		for _, want := range []error{errRun, errStop1, errStop2, context.Canceled} {
			if !errors.Is(err, want) {
				t.Fatalf("lost %v in %v", want, err)
			}
		}

		if err := srv.Run(context.Background()); err == nil {
			t.Fatal("second Run succeeded")
		}
	})
}

func TestShutdownDeadlineCancelsWorkersButWaitsForCallbacks(t *testing.T) {
	prepareSignals(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		started, workerDone := make(chan struct{}), make(chan struct{})
		callbackStarted, release := make(chan struct{}), make(chan struct{})
		lateErr := errors.New("late cleanup")
		srv := New(Config{GracePeriod: time.Second}).WithShutdown(
			func(context.Context) error { close(callbackStarted); <-release; return lateErr },
			func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
		)

		done := make(chan error, 1)
		go func() {
			done <- srv.Run(ctx, func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				close(workerDone)

				return nil
			})
		}()

		<-started

		stoppingAt := time.Now()

		cancel()
		<-callbackStarted
		<-workerDone

		if time.Since(stoppingAt) != time.Second {
			t.Fatal("worker canceled outside shared deadline")
		}

		select {
		case err := <-done:
			t.Fatalf("abandoned callback: %v", err)
		default:
		}

		close(release)

		if err := <-done; !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, lateErr) {
			t.Fatalf("lost deadline or late error: %v", err)
		}
	})
}

func TestServiceWorkerFailureAndFiniteWorkers(t *testing.T) {
	prepareSignals(t)
	synctest.Test(t, func(t *testing.T) {
		finiteDone, stopped := make(chan struct{}), make(chan struct{})
		errFirst, errLate := errors.New("first"), errors.New("late")
		calls := 0
		srv := New(Config{GracePeriod: time.Second}).WithShutdown(func(context.Context) error {
			calls++

			close(stopped)

			return nil
		})

		err := srv.Run(context.Background(),
			func(context.Context) error { close(finiteDone); return nil },
			func(ctx context.Context) error {
				<-finiteDone

				if ctx.Err() != nil {
					t.Error("finite worker stopped peers")
				}

				return errFirst
			},
			func(context.Context) error { <-stopped; return errLate },
		)
		if calls != 1 || !errors.Is(err, errFirst) || !errors.Is(err, errLate) {
			t.Fatalf("calls=%d, errors=%v", calls, err)
		}
	})
}

func prepareSignals(t *testing.T) {
	t.Helper()
	// The runtime signal goroutine must be initialized outside the synctest bubble.
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	t.Cleanup(stop)
}

func TestShutdownRegistrationAndValidation(t *testing.T) {
	for _, scenario := range []string{"nil run", "nil stop", "replace", "clear"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			srv := New(Config{GracePeriod: time.Second}).WithShutdown(func(context.Context) error { calls++; return nil })

			var workers []Worker

			switch scenario {
			case "nil run":
				workers = []Worker{nil}
			case "nil stop":
				srv.WithShutdown(nil)
			case "replace":
				srv.WithShutdown(func(context.Context) error { calls += 2; return nil })
			case "clear":
				srv.WithShutdown()
			}

			err := srv.Run(context.Background(), workers...)

			wantCalls := 0
			if scenario == "replace" {
				wantCalls = 2
			}

			wantErr := scenario == "nil run" || scenario == "nil stop"
			if (err != nil) != wantErr || calls != wantCalls {
				t.Fatalf("error=%v, calls=%d", err, calls)
			}
		})
	}
}

func TestServiceWithoutShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := New(Config{}).Run(ctx); err != nil {
		t.Fatalf("empty service: %v", err)
	}

	if err := New(Config{GracePeriod: time.Second}).Run(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}); err != nil {
		t.Fatalf("context-only worker: %v", err)
	}
}
