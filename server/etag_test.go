package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestETagConditionalGet(t *testing.T) {
	srv := New(Config{UseETag: true})
	srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("response"))
	})
	handler := srv.ServeMuxWithHandlers()

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	tag := first.Header().Get("ETag")
	if !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) {
		t.Fatalf("ETag is not a quoted entity-tag: %q", tag)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("If-None-Match", tag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified {
		t.Fatalf("status: got %d, want %d", second.Code, http.StatusNotModified)
	}
	if second.Body.Len() != 0 {
		t.Fatalf("304 response has a body: %q", second.Body.String())
	}
}

func TestETagFlushSwitchesToStreaming(t *testing.T) {
	srv := New(Config{UseETag: true})
	srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("second"))
	})

	recorder := httptest.NewRecorder()
	srv.ServeMuxWithHandlers().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !recorder.Flushed {
		t.Fatal("response was not flushed")
	}
	if tag := recorder.Header().Get("ETag"); tag != "" {
		t.Fatalf("streaming response unexpectedly has ETag %q", tag)
	}
	if body := recorder.Body.String(); body != "firstsecond" {
		t.Fatalf("body: got %q, want %q", body, "firstsecond")
	}
}

func TestETagConfiguredLimitSwitchesToStreaming(t *testing.T) {
	const limit = 16
	body := bytes.Repeat([]byte("x"), limit+1)
	srv := New(Config{UseETag: true, ETagMaxBodyBytes: limit})
	srv.ServeMux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})

	recorder := httptest.NewRecorder()
	srv.ServeMuxWithHandlers().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if tag := recorder.Header().Get("ETag"); tag != "" {
		t.Fatalf("large response unexpectedly has ETag %q", tag)
	}
	if !bytes.Equal(recorder.Body.Bytes(), body) {
		t.Fatal("large response body changed")
	}
}
