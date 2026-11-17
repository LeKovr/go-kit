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

func TestShutdownForcesCloseAfterDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	srv := New(Config{
		AccessLog: AccessLogDisabled,
	}).WithListener(ln)
	srv.ServeMux().HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelShutdown()

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.Shutdown(shutdownCtx) }()

	select {
	case runErr := <-runDone:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("Run error: got %v, want context deadline exceeded", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after shutdown deadline")
	}

	if err := <-shutdownDone; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error: %v", err)
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
	srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(listener)
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

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.Shutdown(shutdownCtx) }()

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

	if err := <-shutdownDone; err != nil {
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
	for _, scenario := range []string{"canceled", "log", "tls"} {
		t.Run(scenario, func(t *testing.T) {
			ln := newTestListener(t)
			var closes atomic.Int32
			cfg := Config{AccessLog: AccessLogDisabled}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "canceled":
				cancel()
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

func TestServeErrorForcesCloseActiveRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer serverConn.Close()
		defer clientConn.Close()

		serveErr, closeErr := errors.New("accept failed"), errors.New("close failed")

		ln := &fatalAcceptListener{
			conn: serverConn, fail: make(chan struct{}), closed: make(chan struct{}),
			acceptErr: serveErr, closeErr: closeErr,
		}
		defer ln.Close()

		srv := New(Config{AccessLog: AccessLogDisabled}).WithListener(ln)
		started := make(chan context.Context, 1)
		handlerDone := make(chan struct{})

		srv.ServeMux().HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
			started <- r.Context()

			<-r.Context().Done()
			close(handlerDone)
		})

		done := make(chan error, 1)
		go func() { done <- srv.Run(context.Background()) }()

		requestDone := make(chan struct{})
		go func() {
			defer close(requestDone)

			_, _ = io.WriteString(clientConn, "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
			_, _ = io.Copy(io.Discard, clientConn)
		}()

		requestCtx := <-started

		close(ln.fail)

		runErr := <-done
		if !errors.Is(runErr, serveErr) || !errors.Is(runErr, closeErr) {
			t.Fatalf("serve or cleanup error lost: %v", runErr)
		}

		if requestCtx.Err() == nil {
			t.Fatal("request context left live after serve failure")
		}

		<-handlerDone
		<-requestDone
	})
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
