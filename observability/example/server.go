package main

import (
	"context"
	"net/http"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type DemoHandler struct {
	tracer trace.Tracer

	requests metric.Int64Counter
}

func runServer(ctx context.Context, cfg Config, obs *observability.Service) error {
	const instrumentation = application + "/server"

	demoHandler, err := NewDemoHandler(
		obs.Tracer(instrumentation),
		obs.Meter(instrumentation),
	)
	if err != nil {
		return err
	}

	srv := server.New(cfg.Server)
	srv.Use(obs.HTTPMiddleware())
	srv.ServeMux().HandleFunc("/demo", demoHandler.Handle)

	return srv.Run(ctx)
}

func NewDemoHandler(tracer trace.Tracer, meter metric.Meter) (*DemoHandler, error) {
	requests, err := meter.Int64Counter(
		"demo.custom.requests",
		metric.WithDescription("Number of custom demo handler calls."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &DemoHandler{
		tracer:   tracer,
		requests: requests,
	}, nil
}

func (h DemoHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	h.RecordRequest(r.Context())
	h.Calculate(r.Context())

	_, _ = w.Write([]byte("ok\n"))
}

func (h DemoHandler) RecordRequest(ctx context.Context) {
	h.requests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("demo.operation", "request"),
	))
}

func (h DemoHandler) Calculate(ctx context.Context) {
	_, span := h.tracer.Start(ctx, "demo.calculate")
	defer span.End()

	span.SetAttributes(attribute.String("demo.step", "calculate"))
	time.Sleep(250 * time.Millisecond)
}
