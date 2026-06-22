package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Config holds all config vars.
type Config struct {
	Server        server.Config        `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	Observability observability.Config `group:"OpenTelemetry Options" namespace:"otel" env-namespace:"OTEL"`

	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump
}

const application = "observability-example"

var version = "0.0-dev"

func main() {
	config.SetApplicationVersion(application, version)

	var cfg Config
	err := config.Open(&cfg)

	defer func() {
		config.Close(err, os.Exit)
	}()

	if err != nil {
		return
	}

	ctx := context.Background()

	var obs *observability.Service
	if obs, err = observability.New(ctx, cfg.Observability, application, version); err != nil {
		return
	}

	defer func() {
		if er := obs.Shutdown(ctx); er != nil {
			slog.Error("observability shutdown", "err", er)
		}
	}()

	// observability.New keeps providers local; the app opts in to globals so
	// standard instrumentation, such as otelhttp.NewTransport, can use them.
	restore := obs.InstallGlobal()
	defer restore()

	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("external ok\n"))
	}))
	defer external.Close()

	const instrumentation = application + "/custom"

	demoHandler, err := NewDemoHandler(
		obs.Tracer(instrumentation),
		obs.Meter(instrumentation),
		external.URL,
	)
	if err != nil {
		return
	}

	srv := server.New(cfg.Server)
	srv.Use(obs.HTTPMiddleware())
	srv.ServeMux().HandleFunc("/demo", demoHandler.Handle)

	if err = srv.Run(ctx); err != nil {
		return
	}
}

type DemoHandler struct {
	tracer      trace.Tracer
	client      *http.Client
	externalURL string

	requests metric.Int64Counter
}

func NewDemoHandler(tracer trace.Tracer, meter metric.Meter, externalURL string) (*DemoHandler, error) {
	requests, err := meter.Int64Counter(
		"demo.custom.requests",
		metric.WithDescription("Number of custom demo handler calls."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &DemoHandler{
		tracer: tracer,
		client: &http.Client{
			// NewTransport uses the providers installed by obs.InstallGlobal().
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   3 * time.Second,
		},
		externalURL: externalURL,
		requests:    requests,
	}, nil
}

func (h DemoHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	h.RecordRequest(r.Context())
	h.Calculate(r.Context())

	if er := h.CallExternal(r.Context()); er != nil {
		http.Error(w, er.Error(), http.StatusInternalServerError)

		return
	}

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

func (h DemoHandler) CallExternal(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.externalURL, nil)
	if err != nil {
		return err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("external request failed: %s", resp.Status)
	}

	return nil
}
