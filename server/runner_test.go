package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunFiniteWorkers(t *testing.T) {
	if err := Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	worker := func(context.Context) error { calls.Add(1); return nil }
	if err := Run(context.Background(), worker, worker, worker); err != nil {
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
	err := Run(ctx,
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
	started, cleaning, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(),
			func(context.Context) error { <-started; return want },
			func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				close(cleaning)
				<-release
				return errors.New("later cleanup error")
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
		if err != want {
			t.Fatalf("got %v, want original error", err)
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
	err := Run(ctx, func(context.Context) error { return want })
	if err != want || !errors.Is(err, cleanupErr) {
		t.Fatalf("error lost: %v", err)
	}
	if err := Run(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
