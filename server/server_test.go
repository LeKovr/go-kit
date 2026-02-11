package server

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	cfg := Config{Listen: ":0"}
	srv := New(cfg)
	if srv == nil {
		t.Fatalf("New returned nil")
	}
	if srv.config.Listen != cfg.Listen {
		t.Fatalf("config not set correctly: %v", srv.config.Listen)
	}
	if srv.mux == nil {
		t.Fatalf("mux not initialized")
	}
}

func TestWithStatic(t *testing.T) {
	// create temp directory with a file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("world"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	fsys := os.DirFS(dir)
	srv := New(Config{Listen: ":0"}).WithStatic(fsys)

	// use httptest server to serve mux
	ts := httptest.NewServer(srv.ServeMux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/hello.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "world" {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestWithVersion(t *testing.T) {
	srv := New(Config{Listen: ":0", Version: VersionResponseConfig{Prefix: "/ver.js", CType: "text/javascript", Format: "var v='%s';"}})
	srv.WithVersion("1.2.3")
	ts := httptest.NewServer(srv.ServeMux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ver.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/javascript" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	expected := "var v='1.2.3';"
	if !strings.Contains(string(body), expected) {
		t.Fatalf("body does not contain expected: %s", string(body))
	}
}

func TestUseMiddleware(t *testing.T) {
	srv := New(Config{Listen: ":0"})
	// middleware that sets header
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test", "ok")
			next.ServeHTTP(w, r)
		})
	}
	srv.Use(mw)
	srv.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	ts := httptest.NewServer(srv.ServeMuxWithHandlers())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if val := resp.Header.Get("X-Test"); val != "ok" {
		t.Fatalf("middleware header missing: %s", val)
	}
}

func TestWithETag(t *testing.T) {
	srv := New(Config{Listen: ":0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("etag test"))
	})
	wrapped := srv.Config().WithETag(handler)
	ts := httptest.NewServer(wrapped)
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if etag := resp.Header.Get("ETag"); etag == "" {
		t.Fatal("ETag header missing")
	}
}
func TestWithAccessLog(t *testing.T) {
	srv := New(Config{Listen: ":0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("access log test"))
	})
	wrapped := srv.Config().WithAccessLog(handler)
	ts := httptest.NewServer(wrapped)
	defer ts.Close()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r) // Read from the pipe
	os.Stdout = old  // Restore original stdout

	if !strings.Contains(buf.String(), `"GET /" 200`) {
		t.Fatalf("Log data header missing: %s", buf.String())

	}

}

func TestWithShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	called := false
	worker := func(ctx context.Context) error {
		called = true
		return nil
	}
	srv := New(Config{Listen: ":0"}).WithShutdown(worker)
	err := srv.RunWorkers(ctx)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("RunWorkers returned error: %v", err)
	}
	if !called {
		t.Fatalf("worker not called")
	}
}
func TestRunWorkers(t *testing.T) {
	srv := New(Config{Listen: ":0"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	called := false
	worker := func(ctx context.Context) error {
		called = true
		return nil
	}
	err := srv.RunWorkers(ctx, worker)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("RunWorkers returned error: %v", err)
	}
	if !called {
		t.Fatalf("worker not called")
	}
}

func TestRun(t *testing.T) {
	srv := New(Config{Listen: ":0"})
	srv.mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	// create a listener to capture address
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv = srv.WithListener(ln)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Run(ctx); err != nil && !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("Run error: %v", err)
		}
	}()
	// give server time to start
	time.Sleep(100 * time.Millisecond)
	addr := ln.Addr().String()
	resp, err := http.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(body) != "pong" {
		t.Fatalf("unexpected body: %s", string(body))
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// Helper to expose Config method (not exported). We use reflection.
func (srv *Service) Config() Config {
	return srv.config
}
