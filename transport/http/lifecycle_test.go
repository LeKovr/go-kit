package httpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunForcesCloseAfterGracePeriod(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	srv := New(Config{
		AccessLog:   AccessLogDisabled,
		GracePeriod: 20 * time.Millisecond,
	}).WithListener(ln)
	srv.ServeMux().HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(ctx) }()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, requestErr := http.Get("http://" + ln.Addr().String() + "/")
		if requestErr == nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()

	select {
	case runErr := <-runDone:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("Run error: got %v, want context deadline exceeded", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after grace period")
	}

	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request did not finish after forced close")
	}
}

func TestGracefulRequestContext(t *testing.T) {
	ln := newTestListener(t)
	closing := make(chan struct{})
	listener := &closeSignalListener{Listener: ln, closed: closing}
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "value")
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	requestCtx := make(chan context.Context, 1)
	srv := New(Config{GracePeriod: time.Second, AccessLog: AccessLogDisabled}).WithListener(listener)
	srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requestCtx <- r.Context()
		close(started)
		<-release
		if r.Context().Err() != nil {
			t.Error("active request canceled during drain")
		}
		if r.Context().Value(key{}) != "value" {
			t.Error("context value lost")
		}
		_, _ = w.Write([]byte("ok"))
	})
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	response := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err == nil {
			err = resp.Body.Close()
		}
		response <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler not started")
	}
	rctx := <-requestCtx
	cancel()
	select {
	case <-closing:
	case <-time.After(time.Second):
		t.Fatal("listener not closed")
	}
	if rctx.Err() != nil {
		t.Fatal("request canceled before drain finished")
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if rctx.Err() == nil {
		t.Fatal("request context left live")
	}
}

func TestImmediateStopCancelsRequest(t *testing.T) {
	ln := newTestListener(t)
	srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(ln)
	started, finished := make(chan struct{}), make(chan struct{})
	srv.ServeMux().HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(finished)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler not started")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler not canceled")
	}
}

func TestStartupCleanupAndSingleRun(t *testing.T) {
	for _, scenario := range []string{"canceled", "negative", "log", "tls"} {
		t.Run(scenario, func(t *testing.T) {
			ln := newTestListener(t)
			var closes atomic.Int32
			cfg := Config{AccessLog: AccessLogDisabled}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "canceled":
				cancel()
			case "negative":
				cfg.GracePeriod = -1
			case "log":
				cfg.AccessLog = filepath.Join(t.TempDir(), "missing", "access.log")
			case "tls":
				cfg.TLS.CertFile = filepath.Join(t.TempDir(), "missing.pem")
			}
			srv := New(cfg).WithListener(&countListener{Listener: ln, closes: &closes})
			err := srv.Run(ctx)
			if (err == nil) != (scenario == "canceled") {
				t.Fatalf("Run error = %v", err)
			}
			if closes.Load() != 1 {
				t.Fatalf("close count = %d", closes.Load())
			}
			if err := srv.Run(ctx); err == nil {
				t.Fatal("second Run succeeded")
			}
		})
	}
}

func TestServeAndCloseErrorsPreserved(t *testing.T) {
	serveErr, closeErr := errors.New("accept failed"), errors.New("close failed")
	ln := &errorListener{Listener: newTestListener(t), acceptErr: serveErr, closeErr: closeErr}
	err := New(Config{AccessLog: AccessLogDisabled}).WithListener(ln).Run(context.Background())
	if !errors.Is(err, serveErr) || !errors.Is(err, closeErr) {
		t.Fatalf("lost errors: %v", err)
	}
}

func TestExternalListenerCloseIsError(t *testing.T) {
	ln := newTestListener(t)
	_ = ln.Close()
	err := New(Config{AccessLog: AccessLogDisabled}).WithListener(ln).Run(context.Background())
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("unexpected close ignored: %v", err)
	}
}

func TestBindErrorDoesNotOpenLog(t *testing.T) {
	ln := newTestListener(t)
	path := filepath.Join(t.TempDir(), "access.log")
	srv := New(Config{Listen: ln.Addr().String(), AccessLog: path})
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("bind succeeded")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("log opened before bind: %v", err)
	}
}

func TestConcurrentRunRejected(t *testing.T) {
	ln := newTestListener(t)
	entered := make(chan struct{})
	srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(&acceptSignalListener{Listener: ln, entered: entered})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	<-entered
	if err := srv.Run(ctx); err == nil {
		t.Fatal("concurrent Run succeeded")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newTestListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

type closeSignalListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (ln *closeSignalListener) Close() error {
	err := ln.Listener.Close()
	ln.once.Do(func() { close(ln.closed) })
	return err
}

type countListener struct {
	net.Listener
	closes *atomic.Int32
}

func (ln *countListener) Close() error { ln.closes.Add(1); return ln.Listener.Close() }

type errorListener struct {
	net.Listener
	acceptErr, closeErr error
}

func (ln *errorListener) Accept() (net.Conn, error) { return nil, ln.acceptErr }
func (ln *errorListener) Close() error              { return errors.Join(ln.Listener.Close(), ln.closeErr) }

type acceptSignalListener struct {
	net.Listener
	entered chan struct{}
	once    sync.Once
}

func (ln *acceptSignalListener) Accept() (net.Conn, error) {
	ln.once.Do(func() { close(ln.entered) })
	return ln.Listener.Accept()
}

func TestNewAndCanceledRunDoNotOpenResources(t *testing.T) {
	file := filepath.Join(t.TempDir(), "access.log")
	cfg := Config{Listen: "invalid address", AccessLog: file}
	srv := New(cfg)
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New opened access log: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.Run(ctx); err != nil {
		t.Fatalf("canceled Run opened listener: %v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled Run opened log: %v", err)
	}
}

func TestServeErrorDrainsActiveRequest(t *testing.T) {
	for _, scenario := range []string{"graceful", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				serverConn, clientConn := net.Pipe()
				defer serverConn.Close()
				defer clientConn.Close()
				fail := make(chan struct{})
				serveErr, closeErr := errors.New("accept failed"), errors.New("close failed")
				ln := &fatalAcceptListener{
					conn: serverConn, fail: fail, closed: make(chan struct{}),
					acceptErr: serveErr, closeErr: closeErr,
				}
				defer ln.Close()
				const grace = time.Second
				srv := New(Config{GracePeriod: grace, AccessLog: AccessLogDisabled}).WithListener(ln)
				started := make(chan context.Context, 1)
				release := make(chan struct{})
				releaseHandler := sync.OnceFunc(func() { close(release) })
				defer releaseHandler()
				srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					started <- r.Context()
					<-release
					_, _ = w.Write([]byte("completed"))
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- srv.Run(ctx) }()
				transport := &http.Transport{
					DialContext: func(context.Context, string, string) (net.Conn, error) {
						return clientConn, nil
					},
					DisableKeepAlives: true,
				}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport}
				type response struct {
					body string
					err  error
				}
				requestDone := make(chan response, 1)
				go func() {
					resp, err := client.Get("http://example.test/")
					if err != nil {
						requestDone <- response{err: err}
						return
					}
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					requestDone <- response{body: string(body), err: err}
				}()
				requestCtx := <-started
				failedAt := time.Now()
				close(fail)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("Run returned before drain: %v", err)
				default:
				}
				if requestCtx.Err() != nil {
					t.Fatal("request canceled before drain")
				}
				if scenario == "graceful" {
					releaseHandler()
				}
				runErr := <-done
				if !errors.Is(runErr, serveErr) || !errors.Is(runErr, closeErr) {
					t.Fatalf("serve or cleanup error lost: %v", runErr)
				}
				if errors.Is(runErr, context.DeadlineExceeded) != (scenario == "timeout") {
					t.Fatalf("unexpected shutdown error: %v", runErr)
				}
				result := <-requestDone
				if scenario == "graceful" {
					if result.err != nil || result.body != "completed" {
						t.Fatalf("response = %q, error = %v", result.body, result.err)
					}
				} else {
					if time.Since(failedAt) < grace {
						t.Fatal("forced stop before grace expired")
					}
					if result.err == nil || requestCtx.Err() == nil {
						t.Fatalf("request not stopped: response error = %v, context error = %v", result.err, requestCtx.Err())
					}
				}
			})
		})
	}
}

type fatalAcceptListener struct {
	conn                net.Conn
	fail, closed        chan struct{}
	acceptErr, closeErr error
	accepted            bool
	closeOnce           sync.Once
}

func (ln *fatalAcceptListener) Accept() (net.Conn, error) {
	if !ln.accepted {
		ln.accepted = true
		return ln.conn, nil
	}
	select {
	case <-ln.fail:
		return nil, ln.acceptErr
	case <-ln.closed:
		return nil, net.ErrClosed
	}
}

func (ln *fatalAcceptListener) Close() error {
	ln.closeOnce.Do(func() { close(ln.closed) })
	return ln.closeErr
}

func (ln *fatalAcceptListener) Addr() net.Addr { return ln.conn.LocalAddr() }
