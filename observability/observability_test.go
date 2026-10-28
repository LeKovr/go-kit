package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestServiceShutdownWithoutProviders(t *testing.T) {
	var svc Service
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestServiceShutdownRunsAllHooksAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first shutdown")
	secondErr := errors.New("second shutdown")
	secondStarted := make(chan struct{})

	svc := Service{
		config: Config{ShutdownTimeout: time.Second},
		shutdowns: []func(context.Context) error{
			func(ctx context.Context) error {
				select {
				case <-secondStarted:
					return firstErr
				case <-ctx.Done():
					return ctx.Err()
				}
			},
			nil,
			func(context.Context) error {
				close(secondStarted)
				return secondErr
			},
		},
	}

	err := svc.Shutdown(context.Background())
	if !errors.Is(err, firstErr) {
		t.Fatalf("shutdown error does not include first error: %v", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("shutdown error does not include second error: %v", err)
	}
}

func TestTraceSampling(t *testing.T) {
	zero, quarter, full := 0, 25, 100
	for _, tt := range []struct {
		name    string
		percent *int
		low     bool
		high    bool
	}{
		{name: "nil", low: true, high: true},
		{name: "zero", percent: &zero},
		{name: "quarter", percent: &quarter, low: true},
		{name: "full", percent: &full, low: true, high: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sampler, err := newTraceSampler(Config{TraceSamplePercent: tt.percent})
			if err != nil {
				t.Fatal(err)
			}
			for _, root := range []struct {
				id   oteltrace.TraceID
				want bool
			}{
				{oteltrace.TraceID{8: 0x3f, 9: 0xff, 10: 0xff, 11: 0xff, 12: 0xff, 13: 0xff, 14: 0xff, 15: 0xff}, tt.low},
				{oteltrace.TraceID{8: 0x40}, tt.high},
			} {
				result := sampler.ShouldSample(trace.SamplingParameters{ParentContext: context.Background(), TraceID: root.id})
				if got := result.Decision == trace.RecordAndSample; got != root.want {
					t.Errorf("root %s sampled=%v, want %v", root.id, got, root.want)
				}
			}
			for _, remote := range []bool{false, true} {
				for _, flags := range []oteltrace.TraceFlags{0, oteltrace.FlagsSampled} {
					parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
						TraceID: oteltrace.TraceID{1}, SpanID: oteltrace.SpanID{2}, TraceFlags: flags, Remote: remote,
					})
					result := sampler.ShouldSample(trace.SamplingParameters{
						ParentContext: oteltrace.ContextWithSpanContext(context.Background(), parent), TraceID: parent.TraceID(),
					})
					if got := result.Decision == trace.RecordAndSample; got != parent.IsSampled() {
						t.Errorf("remote=%v flags=%v: sampled=%v", remote, flags, got)
					}
				}
			}
		})
	}
}

func TestNewTraceSamplingIgnoresEnvironment(t *testing.T) {
	zero, full := 0, 100
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	for _, percent := range []*int{nil, &zero, &full} {
		want := percent == nil || *percent == 100
		envSampler := "always_off"
		if !want {
			envSampler = "always_on"
		}
		t.Setenv("OTEL_TRACES_SAMPLER", envSampler)
		svc, err := New(context.Background(), Config{
			EnableTraces: true, TraceSamplePercent: percent, OTLPEndpoint: "http://127.0.0.1:4317",
			ShutdownTimeout: time.Second,
		}, "test-service", "test-version")
		if err != nil {
			t.Fatal(err)
		}
		_, span := svc.Tracer("test").Start(context.Background(), "root")
		got := span.SpanContext().IsSampled()
		// Keep the exporter queue empty: this test only checks sampling, not delivery.
		if err := svc.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		span.End()
		if got != want {
			t.Errorf("percent=%v: sampled=%v, want %v", percent, got, want)
		}
	}
}

func TestNewValidatesTraceSamplePercentBeforeExporter(t *testing.T) {
	for _, percent := range []int{-1, 101} {
		_, err := New(context.Background(), Config{
			EnableTraces: true, TraceSamplePercent: &percent, OTLPEndpoint: "not-a-url",
		}, "test-service", "test-version")
		if err == nil || !strings.Contains(err.Error(), "trace sample percent") {
			t.Fatalf("percent=%d: expected sampling validation before endpoint validation, got %v", percent, err)
		}
	}
	for _, percent := range []int{-1, 101} {
		svc, err := New(context.Background(), Config{TraceSamplePercent: &percent}, "test-service", "test-version")
		if err != nil {
			t.Fatalf("disabled traces, percent=%d: %v", percent, err)
		}
		if err := svc.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServiceInstallGlobalRestoresPreviousProviders(t *testing.T) {
	previousTracerProvider := otel.GetTracerProvider()
	previousMeterProvider := otel.GetMeterProvider()

	tracerProvider := trace.NewTracerProvider()
	meterProvider := sdkmetric.NewMeterProvider()
	defer func() {
		_ = tracerProvider.Shutdown(context.Background())
		_ = meterProvider.Shutdown(context.Background())
	}()

	svc := Service{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
	}
	restore := svc.InstallGlobal()

	if otel.GetTracerProvider() != tracerProvider {
		t.Fatal("global tracer provider was not installed")
	}
	if otel.GetMeterProvider() != meterProvider {
		t.Fatal("global meter provider was not installed")
	}

	restore()

	if otel.GetTracerProvider() != previousTracerProvider {
		t.Fatal("global tracer provider was not restored")
	}
	if otel.GetMeterProvider() != previousMeterProvider {
		t.Fatal("global meter provider was not restored")
	}
}

func TestValidateOTLPEndpointURL(t *testing.T) {
	tests := map[string]bool{
		"http://127.0.0.1:4317":  true,
		"https://collector:4317": true,
		"collector:4317":         false,
		"grpc://collector:4317":  false,
		"http://":                false,
	}

	for endpoint, want := range tests {
		err := validateOTLPEndpointURL(endpoint)
		if got := err == nil; got != want {
			t.Fatalf("%q: valid=%v, want %v, err=%v", endpoint, got, want, err)
		}
	}
}

func TestNewWithTracesAndMetricsDisabledSkipsExporterSetup(t *testing.T) {
	svc, err := New(context.Background(), Config{
		OTLPEndpoint:   "not-a-url",
		EnableTraces:   false,
		EnableMetrics:  false,
		MetricInterval: 0,
	}, "test-service", "test-version")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if svc.tracerProvider == nil {
		t.Fatal("tracer provider should be set to noop")
	}
	if svc.meterProvider == nil {
		t.Fatal("meter provider should be set to noop")
	}
}

func TestServiceMeterRecordsBusinessMetric(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	svc := Service{meterProvider: provider}
	meter := svc.Meter("test/business")
	processed, err := meter.Int64Counter("business.processed", otelmetric.WithDescription("Processed business events."))
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	processed.Add(context.Background(), 1)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	if !hasMetric(rm, "business.processed") {
		t.Fatal("expected business metric")
	}
}

func TestServiceMeterWithoutProviderDoesNotUseGlobalProvider(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	defer func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	var svc Service
	meter := svc.Meter("test/business")
	processed, err := meter.Int64Counter("business.processed")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	processed.Add(context.Background(), 1)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	if hasMetric(rm, "business.processed") {
		t.Fatal("empty service should not record through global meter provider")
	}
}
