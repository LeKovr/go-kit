package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
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
	var calls []string

	svc := Service{
		shutdowns: []func(context.Context) error{
			func(context.Context) error {
				calls = append(calls, "first")
				return firstErr
			},
			nil,
			func(context.Context) error {
				calls = append(calls, "second")
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
	if len(calls) != 2 || calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("unexpected shutdown calls: %v", calls)
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

func TestServiceHTTPMiddlewareCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	var gotSpan bool
	svc := Service{
		serviceName:    "test-service",
		tracerProvider: provider,
		meterProvider:  noopmetric.NewMeterProvider(),
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSpan = oteltrace.SpanContextFromContext(r.Context()).IsValid()
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/items/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if !gotSpan {
		t.Fatal("handler context does not contain a valid span")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	if spans[0].Name != "POST /items/42" {
		t.Fatalf("unexpected span name: %q", spans[0].Name)
	}
}

func TestServiceHTTPMiddlewareUsesConfiguredServiceName(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	svc := Service{
		serviceName:    "test-service",
		tracerProvider: provider,
		meterProvider:  noopmetric.NewMeterProvider(),
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/service", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if spans := exporter.GetSpans(); len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
}

func TestHTTPMiddlewareRecordsBaseMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	defer func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	svc := Service{
		serviceName:    "test-service",
		tracerProvider: nooptrace.NewTracerProvider(),
		meterProvider:  provider,
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics-test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	for _, name := range []string{
		"http.server.request.duration",
		"http.server.response.body.size",
	} {
		if !hasMetric(rm, name) {
			t.Fatalf("expected metric %q, got %v", name, metricNames(rm))
		}
	}
}

func TestHTTPMiddlewareWithTracesDisabledDoesNotUseGlobalProvider(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	svc, err := New(context.Background(), Config{
		EnableTraces:  false,
		EnableMetrics: false,
	}, "test-service", "test-version")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if oteltrace.SpanContextFromContext(r.Context()).IsValid() {
			t.Fatal("disabled traces should not create a span")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/disabled-traces", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if spans := exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("expected no spans, got %d", len(spans))
	}
}

func TestHTTPMiddlewareWithMetricsDisabledDoesNotUseGlobalProvider(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	defer func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	svc, err := New(context.Background(), Config{
		EnableTraces:  false,
		EnableMetrics: false,
	}, "test-service", "test-version")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/disabled-metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	if hasMetric(rm, "http.server.request.duration") {
		t.Fatal("disabled metrics should not record through global meter provider")
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

func TestHTTPMiddlewareIgnoresDefaultPaths(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	svc := Service{
		serviceName:    "test-service",
		tracerProvider: provider,
		meterProvider:  noopmetric.NewMeterProvider(),
	}
	handler := svc.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if oteltrace.SpanContextFromContext(r.Context()).IsValid() {
			t.Fatal("ignored request should not have a valid span")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if spans := exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("expected no spans, got %d", len(spans))
	}
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

func metricNames(rm metricdata.ResourceMetrics) []string {
	var names []string
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			names = append(names, metric.Name)
		}
	}
	return names
}
