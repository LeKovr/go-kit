package main

import (
	"log/slog"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/slogger"
	"go.opentelemetry.io/otel/metric"
)

// Telemetry groups scoped observability dependencies for an application component.
type Telemetry struct {
	Logger *slog.Logger
	Meter  metric.Meter
}

func newTelemetry(baseLogger *slog.Logger, obs *observability.Service, scope string) Telemetry {
	return Telemetry{
		Logger: slogger.WithScope(baseLogger, scope),
		Meter:  obs.Meter(scope),
	}
}
