package grpcserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func TestExplicitShutdownDrainsWithCallerDeadline(t *testing.T) {
	for _, scenario := range []string{"graceful", "deadline", "run cancellation", "second shutdown deadline"} {
		t.Run(scenario, func(t *testing.T) {
			ln := testListener(t)
			closing, entered := make(chan struct{}), make(chan struct{})
			srv := New(Config{}).WithListener(&signalListener{Listener: ln, closing: closing, entered: entered})
			started := make(chan context.Context, 1)
			release := make(chan struct{})

			releaseHandler := sync.OnceFunc(func() { close(release) })
			defer releaseHandler()

			pb.RegisterHealthServer(srv.Server(), &checkServer{check: func(ctx context.Context) (*pb.HealthCheckResponse, error) {
				started <- ctx

				select {
				case <-release:
					return &pb.HealthCheckResponse{Status: pb.HealthCheckResponse_SERVING}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}})

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()

			runDone := make(chan error, 1)
			go func() { runDone <- srv.Run(runCtx) }()

			client := healthClient(t, ln)
			rpcDone := make(chan error, 1)

			go func() { _, err := client.Check(ctx, &pb.HealthCheckRequest{}, grpc.WaitForReady(true)); rpcDone <- err }()

			var requestCtx context.Context

			select {
			case requestCtx = <-started:
			case <-ctx.Done():
				t.Fatal("RPC did not start")
			}

			grace := time.Second
			if scenario == "deadline" {
				grace = 20 * time.Millisecond
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), grace)
			defer shutdownCancel()

			if scenario == "run cancellation" {
				shutdownCtx = context.Background()
			}

			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- srv.Shutdown(shutdownCtx) }()

			select {
			case <-closing:
			case <-ctx.Done():
				t.Fatal("listener not closed")
			}

			if scenario == "second shutdown deadline" {
				shortCtx, cancelShort := context.WithTimeout(ctx, time.Millisecond)
				defer cancelShort()

				if err := srv.Shutdown(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("second Shutdown = %v, want deadline exceeded", err)
				}
			}

			graceful := scenario == "graceful" || scenario == "second shutdown deadline"
			if graceful {
				if requestCtx.Err() != nil {
					t.Fatal("active request canceled before shutdown deadline")
				}

				select {
				case err := <-runDone:
					t.Fatalf("Run returned before RPC completed: %v", err)
				default:
				}

				releaseHandler()
			}

			if scenario == "run cancellation" {
				cancelRun()
			}

			wantErr := context.DeadlineExceeded
			if scenario == "run cancellation" {
				wantErr = context.Canceled
			}

			var shutdownErr error

			select {
			case shutdownErr = <-shutdownDone:
			case <-ctx.Done():
				t.Fatal("Shutdown did not return")
			}

			var runErr error

			select {
			case runErr = <-runDone:
			case <-ctx.Done():
				t.Fatal("Run did not return")
			}

			if graceful {
				if shutdownErr != nil || runErr != nil {
					t.Fatalf("shutdown=%v, run=%v", shutdownErr, runErr)
				}
			} else if !errors.Is(shutdownErr, wantErr) || !errors.Is(runErr, wantErr) {
				t.Fatalf("shutdown=%v, run=%v", shutdownErr, runErr)
			}

			if err := <-rpcDone; (err == nil) != graceful {
				t.Fatalf("RPC error=%v", err)
			}

			if err := srv.Shutdown(context.Background()); (err == nil) != (shutdownErr == nil) {
				t.Fatalf("repeated shutdown lost result: %v", err)
			}
		})
	}
}

func TestShutdownDeadlineWhileStopWaitsForHandler(t *testing.T) {
	ln := testListener(t)
	srv := New(Config{}, grpc.WaitForHandlers(true)).WithListener(ln)
	started, release := make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()

	pb.RegisterHealthServer(srv.Server(), &checkServer{check: func(ctx context.Context) (*pb.HealthCheckResponse, error) {
		close(started)
		<-release

		return nil, ctx.Err()
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- srv.Run(runCtx) }()

	client := healthClient(t, ln)
	go func() { _, _ = client.Check(ctx, &pb.HealthCheckRequest{}, grpc.WaitForReady(true)) }()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("RPC did not start")
	}

	stop()

	deadline, cancelShutdown := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelShutdown()

	shutdown := make(chan error, 1)
	go func() { shutdown <- srv.Shutdown(deadline) }()

	select {
	case err := <-shutdown:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Shutdown = %v, want deadline exceeded", err)
		}
	case <-ctx.Done():
		t.Fatal("Shutdown waited for the blocked handler")
	}

	finish()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not stop")
	}
}

// Pause Run as it wraps the Serve error, before its deferred cleanup.
type gatedAcceptError struct {
	ready, resume chan struct{}
}

func (e *gatedAcceptError) Error() string {
	select {
	case <-e.ready:
	default:
		close(e.ready)
	}
	<-e.resume

	return "accept failed"
}

type failingBufListener struct {
	*bufconn.Listener
	err *gatedAcceptError
}

func (ln failingBufListener) Accept() (net.Conn, error) {
	conn, err := ln.Listener.Accept()
	if err != nil {
		return nil, ln.err
	}

	return conn, nil
}

func TestRunCancellationDuringShutdownAfterServeError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		acceptErr := &gatedAcceptError{ready: make(chan struct{}), resume: make(chan struct{})}
		ln := failingBufListener{bufconn.Listen(65536), acceptErr}
		srv := New(Config{}).WithListener(ln)
		started := make(chan context.Context, 1)
		pb.RegisterHealthServer(srv.Server(), &checkServer{check: func(ctx context.Context) (*pb.HealthCheckResponse, error) {
			started <- ctx
			<-ctx.Done()

			return nil, ctx.Err()
		}})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		run := make(chan error, 1)
		go func() { run <- srv.Run(ctx) }()

		conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		go pb.NewHealthClient(conn).Check(context.Background(), &pb.HealthCheckRequest{})
		rpcCtx := <-started

		ln.Close()
		<-acceptErr.ready

		shutdown := make(chan error, 1)
		go func() { shutdown <- srv.Shutdown(context.Background()) }()
		synctest.Wait()
		close(acceptErr.resume)
		synctest.Wait()

		cancel()
		synctest.Wait()
		if rpcCtx.Err() == nil {
			t.Fatal("Run cancellation left the active RPC running")
		}
		if err := <-shutdown; !errors.Is(err, context.Canceled) {
			t.Fatalf("Shutdown = %v, want cancellation", err)
		}
		if err := <-run; !errors.Is(err, acceptErr) {
			t.Fatalf("Run = %v, want listener error", err)
		}
	})
}

func TestShutdownBeforeAndDuringRun(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		for range 20 {
			ln := &countListener{Listener: testListener(t)}
			srv := New(Config{}).WithListener(ln)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			if !concurrent {
				if err := srv.Shutdown(ctx); err != nil {
					t.Fatal(err)
				}
			}

			var group sync.WaitGroup

			group.Go(func() {
				if err := srv.Run(ctx); err != nil {
					t.Error(err)
				}
			})

			for range 3 {
				group.Go(func() {
					if err := srv.Shutdown(ctx); err != nil {
						t.Error(err)
					}
				})
			}

			group.Wait()
			cancel()

			if ln.closes.Load() != 1 {
				t.Fatalf("listener closed %d times", ln.closes.Load())
			}

			if err := srv.Run(context.Background()); err == nil {
				t.Fatal("second Run succeeded")
			}
		}
	}
}

type countListener struct {
	net.Listener
	closes atomic.Int32
}

func (ln *countListener) Close() error {
	ln.closes.Add(1)
	return ln.Listener.Close()
}

func TestShutdownAfterStartupFailure(t *testing.T) {
	srv := New(Config{Listen: "invalid address"})
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("expected listen error")
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
