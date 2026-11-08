package server_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/LeKovr/go-kit/server"
	httpserver "github.com/LeKovr/go-kit/server/http"
)

type legacyMux interface {
	ServeMux() *http.ServeMux
	ServeMuxWithHandlers() http.Handler
}

type legacyAPI interface {
	legacyMux
	WithListener(net.Listener) *server.Service
	WithStatic(fs.FS) *server.Service
	WithVersion(string) *server.Service
	Use(server.Handler) *server.Service
	WithShutdown(server.Worker) *server.Service
	WithWorkers(...server.Worker) *server.Service
	WithNWorkers(int, server.Worker) *server.Service
	WithHTTPWorkers() *server.Service
	Run(context.Context, ...server.Worker) error
	RunWorkers(context.Context, ...server.Worker) error
}

var (
	_ legacyMux                           = server.Service{}
	_ legacyAPI                           = (*server.Service)(nil)
	_ func(server.Config) *server.Service = server.New
	_ *httpserver.Config                  = (*server.Config)(nil)
	_ *httpserver.TLSConfig               = (*server.TLSConfig)(nil)
	_ *httpserver.VersionResponseConfig   = (*server.VersionResponseConfig)(nil)
	_ *httpserver.Handler                 = (*server.Handler)(nil)
)

func TestLegacyRunWorkersWaitsWithoutHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var calls atomic.Int32
	worker := func(context.Context) error { calls.Add(1); return nil }
	srv := server.New(server.Config{Listen: "invalid address"}).
		WithWorkers(worker).WithNWorkers(2, worker).WithNWorkers(0, worker).WithNWorkers(-1, worker)
	if err := srv.RunWorkers(ctx, worker); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || calls.Load() != 4 {
		t.Fatalf("returned before cancellation or lost workers: context=%v, calls=%d", ctx.Err(), calls.Load())
	}
}

func TestLegacyShutdownContext(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "value")
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	cancel()
	cleaning, callbackReady := make(chan struct{}), make(chan struct{})
	called, replaced := 0, 0
	cleanupErr := errors.New("cleanup failed")
	const grace = time.Minute
	srv := server.New(server.Config{GracePeriod: grace}).
		WithShutdown(func(context.Context) error { replaced++; return nil }).
		WithShutdown(func(ctx context.Context) error {
			called++
			deadline, ok := ctx.Deadline()
			remaining := time.Until(deadline)
			if ctx.Err() != nil || ctx.Value(key{}) != "value" || !ok || remaining < grace/2 || remaining > grace {
				t.Errorf("invalid shutdown context: error=%v, value=%v, deadline=%v", ctx.Err(), ctx.Value(key{}), deadline)
			}
			close(callbackReady)
			<-cleaning
			return cleanupErr
		})
	err := srv.RunWorkers(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		close(cleaning)
		<-callbackReady
		return nil
	})
	if err != cleanupErr || called != 1 || replaced != 0 {
		t.Fatalf("shutdown: error=%v, calls=%d, replaced calls=%d", err, called, replaced)
	}
}

func TestLegacyCancellation(t *testing.T) {
	wrapped := fmt.Errorf("worker stopped: %w", context.Canceled)
	joined := errors.Join(context.Canceled, errors.New("cleanup failed"))
	for _, tc := range []struct {
		name      string
		err, want error
		cancel    bool
	}{
		{"direct", context.Canceled, nil, true},
		{"wrapped", wrapped, nil, true},
		{"joined", joined, joined, true},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded, true},
		{"unexpected", context.Canceled, context.Canceled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			if err := server.New(server.Config{}).RunWorkers(ctx, func(context.Context) error { return tc.err }); err != tc.want {
				t.Fatalf("error=%v, want original %v", err, tc.want)
			}
		})
	}
}

func TestLegacyCanceledWorkerDoesNotHideCleanupError(t *testing.T) {
	// Initialize macOS signal machinery outside the synctest bubble.
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		release := make(chan struct{})
		cleanupErr := errors.New("late cleanup failed")
		done := make(chan error, 1)
		go func() {
			done <- server.New(server.Config{}).RunWorkers(ctx,
				func(ctx context.Context) error { <-ctx.Done(); return fmt.Errorf("stopped: %w", ctx.Err()) },
				func(ctx context.Context) error { <-ctx.Done(); <-release; return cleanupErr },
			)
		}()
		synctest.Wait()
		cancel()
		// Let cancellation reach errgroup before cleanup can fail.
		synctest.Wait()
		close(release)
		if err := <-done; err != cleanupErr {
			t.Fatalf("cleanup error lost: %v", err)
		}
	})
}

func TestLegacyHTTPRegisteredOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, mode := range []string{"Run", "RunWorkers"} {
		t.Run(mode, func(t *testing.T) {
			ln := legacyListen(t)
			srv := server.New(server.Config{AccessLog: server.AccessLogDisabled}).
				WithListener(ln).WithHTTPWorkers().WithHTTPWorkers()
			run := srv.Run
			if mode == "RunWorkers" {
				run = srv.RunWorkers
			}
			if err := run(ctx); err != nil {
				t.Fatal(err)
			}
			if ln.closes.Load() != 1 {
				t.Fatalf("listener closes=%d", ln.closes.Load())
			}
		})
	}
}

func TestLegacyBindFailureDoesNotRegisterArguments(t *testing.T) {
	occupied := legacyListen(t)
	var registered, arguments atomic.Int32
	argument := func(context.Context) error { arguments.Add(1); return nil }
	srv := server.New(server.Config{Listen: occupied.Addr().String()}).
		WithWorkers(func(context.Context) error { registered.Add(1); return nil })
	if err := srv.Run(context.Background(), argument); err == nil {
		t.Fatal("bind succeeded")
	}
	if registered.Load() != 0 || arguments.Load() != 0 {
		t.Fatal("workers started before bind failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.WithListener(legacyListen(t)).Run(ctx, argument); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 1 || arguments.Load() != 1 {
		t.Fatalf("retry: registered calls=%d, argument calls=%d", registered.Load(), arguments.Load())
	}
}

func TestLegacyRepeatedHTTPRunDoesNotLeakListener(t *testing.T) {
	reserved := legacyListen(t)
	addr := reserved.Addr().String()
	_ = reserved.Close()
	srv := server.New(server.Config{Listen: addr, AccessLog: server.AccessLogDisabled}).WithHTTPWorkers()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.RunWorkers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("second HTTP run succeeded")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("rejected Run leaked listener: %v", err)
	}
	_ = ln.Close()
}

type legacyListener struct {
	net.Listener
	closes atomic.Int32
}

func (ln *legacyListener) Close() error { ln.closes.Add(1); return ln.Listener.Close() }

func legacyListen(t *testing.T) *legacyListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return &legacyListener{Listener: ln}
}
