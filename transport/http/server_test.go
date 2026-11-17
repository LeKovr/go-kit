package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	srv := New(Config{Listen: ":0", UseETag: true})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("etag test"))
	})
	srv.ServeMux().HandleFunc("/", handler)
	ts := httptest.NewServer(srv.ServeMuxWithHandlers())
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
	file := filepath.Join(t.TempDir(), "access.log")
	srv := New(Config{AccessLog: file})
	srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	response := requestAndStop(t, srv, "/")
	if response != "ok" {
		t.Fatalf("body = %q", response)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"GET /" 200`) {
		t.Fatalf("log = %q", data)
	}
}

func TestRun(t *testing.T) {
	srv := New(Config{AccessLog: AccessLogDisabled})
	srv.ServeMux().HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("pong")) })
	if body := requestAndStop(t, srv, "/ping"); body != "pong" {
		t.Fatalf("body = %q", body)
	}
}

func requestAndStop(t *testing.T, srv *Service, path string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.WithListener(ln)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	resp, err := http.Get("http://" + ln.Addr().String() + path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return string(body)
}
