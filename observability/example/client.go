package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type ClientConfig struct {
	URL     string        `default:"http://localhost:8080/demo" description:"HTTP server URL"      env:"URL"     long:"url"`
	Timeout time.Duration `default:"3s"                         description:"HTTP request timeout" env:"TIMEOUT" long:"timeout"`
}

func runClient(ctx context.Context, cfg ClientConfig, obs *observability.Service) error {
	// otelhttp.NewTransport uses process-wide providers and propagators by default.
	restore := obs.InstallGlobal()
	defer restore()

	ctx, span := obs.Tracer(application+"/client").Start(ctx, "demo.client.request")
	defer span.End()

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   cfg.Timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "demo client request started", "url", cfg.URL)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	slog.DebugContext(ctx, "demo client response received", "status", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status: %s: %s", resp.Status, string(body))
	}

	_, err = os.Stdout.Write(body)

	return err
}
