package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/baggage"
)

func TestJetStreamHTTPHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	obs := observability.Service{}
	for _, tt := range []struct {
		name   string
		method string
		err    error
		status int
		body   string
	}{
		{"puback", http.MethodGet, nil, http.StatusAccepted, "accepted stream=DEMO_CALCULATE sequence=42\n"},
		{"timeout", http.MethodGet, context.DeadlineExceeded, http.StatusGatewayTimeout, "Gateway Timeout\n"},
		{"broker error", http.MethodGet, errors.New("private broker detail"), http.StatusBadGateway, "Bad Gateway\n"},
		{"unsupported method", http.MethodPost, nil, http.StatusMethodNotAllowed, "method not allowed\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/demo/jetstream", nil)
			r.Header.Set("baggage", "job.id=calculation-42")
			w := httptest.NewRecorder()
			called := false
			publish := func(ctx context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
				called = true
				if msg.Subject != jetStreamSubject || string(msg.Data) != "calculate" {
					t.Fatalf("message = (%q, %q)", msg.Subject, msg.Data)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("publish context has no deadline")
				}
				if got := baggage.FromContext(ctx).Member("job.id").Value(); got != "calculation-42" {
					t.Errorf("job.id = %q, want calculation-42", got)
				}
				return &jetstream.PubAck{Stream: jetStreamName, Sequence: 42}, tt.err
			}
			obs.HTTPMiddleware()(jetStreamHTTPHandler(publish, 3*time.Second, logger)).ServeHTTP(w, r)
			if called != (tt.method == http.MethodGet) {
				t.Errorf("publish called = %t, method = %s", called, tt.method)
			}
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Errorf("response = (%d, %q), want (%d, %q)", w.Code, w.Body.String(), tt.status, tt.body)
			}
		})
	}
}
