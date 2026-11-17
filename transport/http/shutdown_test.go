package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestExplicitShutdownDrainsWithCallerDeadline(t *testing.T) {
	for _, scenario := range []string{"graceful", "deadline", "run cancellation"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				serverConn, clientConn := net.Pipe()
				defer serverConn.Close()
				defer clientConn.Close()

				ln := &fatalAcceptListener{conn: serverConn, fail: make(chan struct{}), closed: make(chan struct{})}
				srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(ln)
				started := make(chan context.Context, 1)
				release := make(chan struct{})

				srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					started <- r.Context()

					select {
					case <-release:
						_, _ = io.WriteString(w, "ok")
					case <-r.Context().Done():
					}
				})

				runCtx, cancelRun := context.WithCancel(context.Background())
				defer cancelRun()

				runDone := make(chan error, 1)
				go func() { runDone <- srv.Run(runCtx) }()

				requestDone := make(chan struct{})
				go func() {
					defer close(requestDone)

					_, _ = io.WriteString(clientConn, "GET / HTTP/1.1\r\nHost: example.test\r\nConnection: close\r\n\r\n")
					_, _ = io.Copy(io.Discard, clientConn)
				}()

				requestCtx := <-started

				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				if scenario == "run cancellation" {
					ctx = context.Background()
				}

				shutdownDone := make(chan error, 1)
				go func() { shutdownDone <- srv.Shutdown(ctx) }()

				<-ln.closed
				synctest.Wait()

				if requestCtx.Err() != nil {
					t.Fatal("active request canceled before shutdown deadline")
				}

				select {
				case err := <-runDone:
					t.Fatalf("Run returned before drain: %v", err)
				default:
				}

				if scenario == "graceful" {
					close(release)
				} else if scenario == "run cancellation" {
					cancelRun()
				}

				wantErr := context.DeadlineExceeded
				if scenario == "run cancellation" {
					wantErr = context.Canceled
				}

				shutdownErr, runErr := <-shutdownDone, <-runDone
				if scenario == "graceful" {
					if shutdownErr != nil || runErr != nil {
						t.Fatalf("shutdown=%v, run=%v", shutdownErr, runErr)
					}
				} else if !errors.Is(shutdownErr, wantErr) || !errors.Is(runErr, wantErr) {
					t.Fatalf("shutdown=%v, run=%v", shutdownErr, runErr)
				}

				if requestCtx.Err() == nil {
					t.Fatal("request context left live")
				}

				<-requestDone

				if err := srv.Shutdown(context.Background()); (err == nil) != (shutdownErr == nil) {
					t.Fatalf("repeated shutdown lost result: %v", err)
				}
			})
		})
	}
}

func TestShutdownBeforeAndDuringRun(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		for range 20 {
			ln := newTestListener(t)

			var closes atomic.Int32

			srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(&countListener{Listener: ln, closes: &closes})

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

			if closes.Load() != 1 {
				t.Fatalf("listener closed %d times", closes.Load())
			}

			if err := srv.Run(context.Background()); err == nil {
				t.Fatal("second Run succeeded")
			}
		}
	}
}

func TestShutdownAfterStartupFailure(t *testing.T) {
	srv := New(Config{Listen: "invalid address", AccessLog: AccessLogDisabled})
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("expected listen error")
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
