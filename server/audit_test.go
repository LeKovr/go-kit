package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuditGracefulShutdownWaitsForHandler(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	srv := New(Config{GracePeriod: time.Second}).WithListener(ln)
	srv.ServeMux().HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	go http.Get("http://" + ln.Addr().String() + "/") //nolint:errcheck
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned before active handler completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not finish")
	}
}

func TestAuditETagEmptyResponse(t *testing.T) {
	h := New(Config{UseETag: true}).ServeMuxWithHandlers()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
}

func TestAuditETagPreservesFlusher(t *testing.T) {
	h := New(Config{UseETag: true})
	h.ServeMux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("ETag middleware removed http.Flusher")
		}
	})
	h.ServeMuxWithHandlers().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
