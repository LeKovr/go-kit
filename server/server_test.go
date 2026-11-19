package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunFiniteWorkers(t *testing.T) {
	if err := New(Config{GracePeriod: time.Second}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32

	worker := func(context.Context) error { calls.Add(1); return nil }
	if err := New(Config{GracePeriod: time.Second}).Run(context.Background(), worker, worker, worker); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestSuccessDoesNotCancelPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	finished := make(chan struct{})

	err := New(Config{GracePeriod: time.Second}).Run(ctx,
		func(context.Context) error { close(finished); return nil },
		func(ctx context.Context) error {
			<-finished

			if err := ctx.Err(); err != nil {
				return err
			}

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestErrorCancelsAndWaitsForCleanup(t *testing.T) {
	want := errors.New("serve failed")
	cleanupErr := errors.New("later cleanup error")
	started, cleaning, release := make(chan struct{}), make(chan struct{}), make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- New(Config{GracePeriod: time.Second}).Run(context.Background(),
			func(context.Context) error { <-started; return want },
			func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				close(cleaning)
				<-release

				return cleanupErr
			},
		)
	}()

	select {
	case <-cleaning:
	case <-time.After(time.Second):
		t.Fatal("peer not canceled")
	}

	select {
	case err := <-done:
		t.Fatalf("returned before cleanup: %v", err)
	default:
	}

	close(release)

	select {
	case err := <-done:
		if !errors.Is(err, want) || !errors.Is(err, cleanupErr) {
			t.Fatalf("lost worker or cleanup error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return")
	}
}

func TestCancellationDoesNotHideCleanupError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cleanupErr := errors.New("cleanup failed")
	want := errors.Join(context.Canceled, cleanupErr)

	err := New(Config{GracePeriod: time.Second}).Run(ctx, func(context.Context) error { return want })
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) {
		t.Fatalf("error lost: %v", err)
	}

	if err := New(Config{GracePeriod: time.Second}).Run(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownUnblocksWorker(t *testing.T) {
	for _, trigger := range []string{"cancellation", "worker error"} {
		t.Run(trigger, func(t *testing.T) {
			type contextKey struct{}

			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "value"))
			t.Cleanup(cancel)

			started, stopped, workerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
			releaseShutdown := make(chan struct{})
			stopWorker := sync.OnceFunc(func() { close(stopped) })
			finishShutdown := sync.OnceFunc(func() { close(releaseShutdown) })

			t.Cleanup(stopWorker)
			t.Cleanup(finishShutdown)

			shutdownErr := errors.New("shutdown failed")

			var calls atomic.Int32

			srv := New(Config{GracePeriod: time.Minute}).WithShutdown(func(ctx context.Context) error {
				calls.Add(1)

				if ctx.Err() != nil || ctx.Value(contextKey{}) != "value" {
					t.Errorf("shutdown context: err=%v, value=%v", ctx.Err(), ctx.Value(contextKey{}))
				}

				if _, ok := ctx.Deadline(); !ok {
					t.Error("shutdown context has no deadline")
				}

				stopWorker()
				<-releaseShutdown

				return shutdownErr
			})
			workers := []Worker{func(context.Context) error {
				close(started)
				// Cancellation cannot unblock this worker; only shutdown can.
				<-stopped
				close(workerDone)

				return nil
			}}

			var runErr error

			if trigger == "worker error" {
				runErr = errors.New("worker failed")

				workers = append(workers, func(context.Context) error {
					<-started
					return runErr
				})
			}

			done := make(chan error, 1)
			go func() { done <- srv.Run(ctx, workers...) }()

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not start")
			}

			if trigger == "cancellation" {
				cancel()
			}

			select {
			case <-workerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not unblock worker")
			}

			select {
			case err := <-done:
				t.Fatalf("Run returned before shutdown completed: %v", err)
			default:
			}

			finishShutdown()

			select {
			case err := <-done:
				if !errors.Is(err, shutdownErr) || (runErr != nil && !errors.Is(err, runErr)) {
					t.Fatalf("lost worker or shutdown error: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not finish after shutdown")
			}

			if calls.Load() != 1 {
				t.Fatalf("shutdown calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestShutdownAfterFiniteWorkers(t *testing.T) {
	for _, count := range []int{0, 2} {
		var workersDone, shutdownCalls atomic.Int32

		srv := New(Config{GracePeriod: time.Second}).WithShutdown(func(context.Context) error {
			shutdownCalls.Add(1)

			if workersDone.Load() != int32(count) {
				t.Errorf("shutdown before all workers finished")
			}

			return nil
		})

		workers := make([]Worker, count)
		for i := range workers {
			workers[i] = func(context.Context) error { workersDone.Add(1); return nil }
		}

		done := make(chan error, 1)
		go func() { done <- srv.Run(context.Background(), workers...) }()

		select {
		case err := <-done:
			if err != nil || shutdownCalls.Load() != 1 {
				t.Fatalf("workers=%d: error=%v, shutdown calls=%d", count, err, shutdownCalls.Load())
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("shutdown waits for cancellation after %d finite workers", count)
		}
	}
}

func TestShutdownGracePeriod(t *testing.T) {
	t.Run("zero expires immediately", func(t *testing.T) {
		err := New(Config{}).WithShutdown(func(ctx context.Context) error {
			return ctx.Err()
		}).Run(context.Background())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want shutdown deadline exceeded", err)
		}
	})
	t.Run("negative rejected before starting", func(t *testing.T) {
		called := false
		worker := func(context.Context) error { called = true; return nil }

		err := New(Config{GracePeriod: -time.Second}).WithShutdown(worker).Run(context.Background(), worker)
		if err == nil || called {
			t.Fatalf("error=%v, worker or shutdown called=%v", err, called)
		}
	})
}
