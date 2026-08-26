package main

import (
	"log/slog"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/slogger"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry groups scoped observability dependencies for an application component.
type Telemetry struct {
	Logger *slog.Logger
	Tracer trace.Tracer
	Meter  metric.Meter
}

func newTelemetry(baseLogger *slog.Logger, obs *observability.Service, scope string) Telemetry {
	return Telemetry{
		Logger: slogger.WithScope(baseLogger, scope),
		Tracer: obs.Tracer(scope),
		Meter:  obs.Meter(scope),
	}
}
