package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/baggage"
)

func TestNATSHTTPHandler(t *testing.T) {
	const timeout = 3 * time.Second
	const reply = "calculated 42\n"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	obs := observability.Service{}
	for _, tt := range []struct {
		name   string
		method string
		err    error
		status int
		body   string
	}{
		{"success", http.MethodGet, nil, http.StatusOK, reply},
		{"no responders", http.MethodGet, nats.ErrNoResponders, http.StatusServiceUnavailable, "Service Unavailable\n"},
		{"wrapped deadline", http.MethodGet, fmt.Errorf("request: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, "Gateway Timeout\n"},
		{"NATS timeout", http.MethodGet, nats.ErrTimeout, http.StatusGatewayTimeout, "Gateway Timeout\n"},
		{"unexpected error", http.MethodGet, errors.New("private broker detail"), http.StatusBadGateway, "Bad Gateway\n"},
		{"unsupported method", http.MethodPost, nil, http.StatusMethodNotAllowed, "method not allowed\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type contextKey struct{}
			ctx := context.WithValue(context.Background(), contextKey{}, "request value")
			r := httptest.NewRequest(tt.method, "/nats", nil).WithContext(ctx)
			r.Header.Set("baggage", "job.id=calculation-42")
			w := httptest.NewRecorder()
			called := false
			started := time.Now()
			request := func(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
				called = true
				if tt.method != http.MethodGet {
					t.Fatal("unsupported method called NATS")
				}
				if got := ctx.Value(contextKey{}); got != "request value" {
					t.Errorf("request context value = %v, want request value", got)
				}
				if got := baggage.FromContext(ctx).Member("job.id").Value(); got != "calculation-42" {
					t.Errorf("job.id = %q, want calculation-42", got)
				}
				if deadline, ok := ctx.Deadline(); !ok || deadline.Before(started.Add(timeout)) || deadline.After(time.Now().Add(timeout)) {
					t.Errorf("request deadline = %v (present: %t), want configured timeout %v", deadline, ok, timeout)
				}
				if msg.Subject != "demo.calculate" || string(msg.Data) != "calculate" {
					t.Errorf("NATS request = (%q, %q), want (demo.calculate, calculate)", msg.Subject, msg.Data)
				}
				return &nats.Msg{Data: []byte(reply)}, tt.err
			}

			obs.HTTPMiddleware()(natsHTTPHandler(request, timeout, logger)).ServeHTTP(w, r)

			if called != (tt.method == http.MethodGet) {
				t.Errorf("NATS called = %t, method = %s", called, tt.method)
			}
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Errorf("response = (%d, %q), want (%d, %q)", w.Code, w.Body.String(), tt.status, tt.body)
			}
		})
	}
}

func TestNATSPublishHTTPHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range []struct {
		name      string
		method    string
		err       error
		failAt    int
		wantCalls int
		status    int
		body      string
	}{
		{"success", http.MethodGet, nil, 0, 3, http.StatusAccepted, "accepted\n"},
		{"first publish fails", http.MethodGet, nats.ErrConnectionClosed, 1, 1, http.StatusServiceUnavailable, "Service Unavailable\n"},
		{"second publish fails", http.MethodGet, errors.New("private broker detail"), 2, 2, http.StatusBadGateway, "Bad Gateway\n"},
		{"third publish fails", http.MethodGet, errors.New("private broker detail"), 3, 3, http.StatusBadGateway, "Bad Gateway\n"},
		{"unsupported method", http.MethodPost, nil, 0, 0, http.StatusMethodNotAllowed, "method not allowed\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/demo/nats/publish", nil)
			w := httptest.NewRecorder()
			var payloads []string
			publish := func(ctx context.Context, msg *nats.Msg) error {
				payloads = append(payloads, string(msg.Data))
				if ctx != r.Context() {
					t.Error("publish did not receive the HTTP request context")
				}
				if msg.Subject != natsSubject || msg.Reply != "" {
					t.Errorf("unexpected publish message: %+v", msg)
				}
				if len(payloads) == tt.failAt {
					return tt.err
				}
				return nil
			}

			natsPublishHTTPHandler(publish, logger).ServeHTTP(w, r)

			wantPayloads := []string{"calculate1", "calculate2", "calculate3"}[:tt.wantCalls]
			if !slices.Equal(payloads, wantPayloads) {
				t.Errorf("payloads = %v, want %v", payloads, wantPayloads)
			}
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Errorf("response = (%d, %q), want (%d, %q)", w.Code, w.Body.String(), tt.status, tt.body)
			}
		})
	}
}
