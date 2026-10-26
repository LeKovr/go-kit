package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type testTelemetry struct {
	service  Service
	exporter *tracetest.InMemoryExporter
	reader   *sdkmetric.ManualReader
}

func newTestTelemetry(t *testing.T, traces, metrics bool) testTelemetry {
	t.Helper()
	t.Setenv("OTEL_SEMCONV_STABILITY_OPT_IN", "")
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
		if err := mp.Shutdown(ctx); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})
	svc := Service{tracerProvider: nooptrace.NewTracerProvider(), meterProvider: noopmetric.NewMeterProvider()}
	if traces {
		svc.tracerProvider = tp
	}
	if metrics {
		svc.meterProvider = mp
	}
	return testTelemetry{service: svc, exporter: exporter, reader: reader}
}

func installTestGlobals(t *testing.T, svc Service) {
	t.Helper()
	tp, mp, propagator := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(svc.tracerProvider)
	otel.SetMeterProvider(svc.meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	t.Cleanup(func() {
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		otel.SetTextMapPropagator(propagator)
	})
}

func hasMetric(rm metricdata.ResourceMetrics, name string) bool {
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return true
			}
		}
	}
	return false
}
