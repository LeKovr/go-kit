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
	"github.com/LeKovr/go-kit/slogger"
)

type ClientConfig struct {
	URL     string        `default:"http://localhost:8080/demo" description:"HTTP server URL"      env:"URL"     long:"url"`
	Timeout time.Duration `default:"3s"                         description:"HTTP request timeout" env:"TIMEOUT" long:"timeout"`
}

func runClient(ctx context.Context, cfg ClientConfig, obs *observability.Service) error {
	logger := slogger.WithScope(slog.Default(), application+"/http-client")

	client := &http.Client{
		Transport: obs.HTTPTransport(http.DefaultTransport),
		Timeout:   cfg.Timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	logger.DebugContext(ctx, "demo client response received", "status", resp.Status)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	_, err = io.Copy(os.Stdout, resp.Body)

	return err
}
