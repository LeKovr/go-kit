package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type DemoHandler struct {
	logger *slog.Logger
	tracer trace.Tracer

	requests metric.Int64Counter
}

func runServer(ctx context.Context, cfg Config, obs *observability.Service) error {
	const instrumentation = application + "/server"
	telemetry := newTelemetry(slog.Default(), obs, instrumentation)

	demoHandler, err := NewDemoHandler(telemetry)
	if err != nil {
		return err
	}

	srv := server.New(cfg.Server)
	srv.Use(obs.HTTPMiddleware())
	srv.ServeMux().HandleFunc("/demo", demoHandler.Handle)

	return srv.Run(ctx)
}

func NewDemoHandler(telemetry Telemetry) (*DemoHandler, error) {
	requests, err := telemetry.Meter.Int64Counter(
		"demo.custom.requests",
		metric.WithDescription("Number of custom demo handler calls."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &DemoHandler{
		logger:   telemetry.Logger,
		tracer:   telemetry.Tracer,
		requests: requests,
	}, nil
}

func (h DemoHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	h.logger.DebugContext(r.Context(), "demo request started", "method", r.Method, "path", r.URL.Path)

	h.RecordRequest(r.Context())
	h.Calculate(r.Context())

	h.logger.DebugContext(r.Context(), "demo request completed", "method", r.Method, "path", r.URL.Path)

	_, _ = w.Write([]byte("ok\n"))
}

func (h DemoHandler) RecordRequest(ctx context.Context) {
	h.requests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("demo.operation", "request"),
	))
}

func (h DemoHandler) Calculate(ctx context.Context) {
	ctx, span := h.tracer.Start(ctx, "demo.calculate")
	defer span.End()

	span.SetAttributes(attribute.String("demo.step", "calculate"))

	h.logger.InfoContext(ctx, "demo calculation started")

	time.Sleep(250 * time.Millisecond)

	h.logger.InfoContext(ctx, "demo calculation completed")
}
