package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestHTTPTransportUsesServiceProvidersAndPropagatesContext(t *testing.T) {
	telemetry := newHTTPTransportTestTelemetry(t)
	foreign := newHTTPTransportTestTelemetry(t)
	installHTTPTransportTestGlobals(t, foreign.service)
	var received context.Context
	done := make(chan struct{})
	handler := telemetry.service.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Context()
		_, _ = io.WriteString(w, "ok")
	}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		close(done)
	}))
	t.Cleanup(server.Close)
	transport := telemetry.service.HTTPTransport(nil,
		otelhttp.WithTracerProvider(foreign.service.tracerProvider),
		otelhttp.WithMeterProvider(foreign.service.meterProvider),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithSpanNameFormatter(func(string, *http.Request) string { return "custom client" }),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, parent := telemetry.service.Tracer("test-parent").Start(ctx, "parent")
	defer parent.End()
	bag, err := baggage.Parse("test-key=test-value")
	if err != nil {
		t.Fatal(err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)
	doHTTPTransportTestRequest(t, ctx, transport, server.URL)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("waiting for server instrumentation: %v", ctx.Err())
	}
	spans := telemetry.exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected client and server spans, got %v", spans)
	}
	byKind := make(map[trace.SpanKind]tracetest.SpanStub)
	for _, span := range spans {
		byKind[span.SpanKind] = span
	}
	clientSpan, serverSpan := byKind[trace.SpanKindClient], byKind[trace.SpanKindServer]
	if clientSpan.Name != "custom client" || clientSpan.Parent.SpanID() != parent.SpanContext().SpanID() || serverSpan.Parent.SpanID() != clientSpan.SpanContext.SpanID() {
		t.Fatalf("incorrect client option or parent chain: client=%v server=%v", clientSpan, serverSpan)
	}
	if clientSpan.SpanContext.TraceID() != parent.SpanContext().TraceID() || serverSpan.SpanContext.TraceID() != parent.SpanContext().TraceID() || !serverSpan.Parent.IsRemote() {
		t.Fatal("trace context did not propagate through the client to the server")
	}
	if trace.SpanContextFromContext(received).SpanID() != serverSpan.SpanContext.SpanID() || baggage.FromContext(received).Member("test-key").Value() != "test-value" {
		t.Fatal("server handler did not receive its span and baggage")
	}
	if got := httpTransportTestDurationCount(t, telemetry.reader); got != 1 {
		t.Fatalf("client duration count = %d, want 1", got)
	}
	if len(foreign.exporter.GetSpans()) != 0 || len(collectHTTPTransportTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
		t.Fatal("transport used foreign providers from options or globals")
	}
}

func TestHTTPTransportKeepsSignalsIndependentOfGlobals(t *testing.T) {
	for _, mode := range []string{"traces", "metrics", "disabled", "zero value"} {
		t.Run(mode, func(t *testing.T) {
			telemetry := newHTTPTransportTestTelemetry(t)
			foreign := newHTTPTransportTestTelemetry(t)
			installHTTPTransportTestGlobals(t, foreign.service)
			svc := telemetry.service
			switch mode {
			case "traces":
				svc.meterProvider = noopmetric.NewMeterProvider()
			case "metrics":
				svc.tracerProvider = nooptrace.NewTracerProvider()
			case "disabled":
				disabled, err := New(context.Background(), Config{}, "test", "test")
				if err != nil {
					t.Fatal(err)
				}
				svc = *disabled
			case "zero value":
				svc = Service{}
			}
			called := false
			base := httpTransportTestRoundTripper(func(*http.Request) (*http.Response, error) {
				called = true
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
			})
			ctx, parent := foreign.service.Tracer("test-parent").Start(context.Background(), "parent")
			defer parent.End()
			doHTTPTransportTestRequest(t, ctx, svc.HTTPTransport(base), "http://example.test/check")
			if !called {
				t.Fatal("transport did not delegate the request")
			}
			wantSpans, wantMetrics := 0, uint64(0)
			if mode == "traces" {
				wantSpans = 1
			}
			if mode == "metrics" {
				wantMetrics = 1
			}
			if got := len(telemetry.exporter.GetSpans()); got != wantSpans {
				t.Fatalf("spans = %d, want %d", got, wantSpans)
			}
			if got := httpTransportTestDurationCount(t, telemetry.reader); got != wantMetrics {
				t.Fatalf("client duration count = %d, want %d", got, wantMetrics)
			}
			if len(foreign.exporter.GetSpans()) != 0 || len(collectHTTPTransportTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
				t.Fatal("transport used global providers or the parent span's provider")
			}
		})
	}
}

func TestHTTPTransportFilterDelegatesWithoutInstrumentation(t *testing.T) {
	telemetry := newHTTPTransportTestTelemetry(t)
	var received *http.Request
	base := httpTransportTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		received = r
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	transport := telemetry.service.HTTPTransport(base, otelhttp.WithFilter(func(*http.Request) bool { return false }))
	ctx, parent := telemetry.service.Tracer("test-parent").Start(context.Background(), "parent")
	defer parent.End()
	doHTTPTransportTestRequest(t, ctx, transport, "http://example.test/check")
	if received == nil || received.Header.Get("traceparent") != "" || trace.SpanContextFromContext(received.Context()).SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("filtered request was not delegated without instrumentation")
	}
	if len(telemetry.exporter.GetSpans()) != 0 || len(collectHTTPTransportTestMetrics(t, telemetry.reader).ScopeMetrics) != 0 {
		t.Fatal("filtered request produced telemetry")
	}
}

type httpTransportTestTelemetry struct {
	service  Service
	exporter *tracetest.InMemoryExporter
	reader   *sdkmetric.ManualReader
}

func newHTTPTransportTestTelemetry(t *testing.T) httpTransportTestTelemetry {
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
			t.Errorf("shutdown tracer: %v", err)
		}
		if err := mp.Shutdown(ctx); err != nil {
			t.Errorf("shutdown meter: %v", err)
		}
	})
	return httpTransportTestTelemetry{service: Service{tracerProvider: tp, meterProvider: mp}, exporter: exporter, reader: reader}
}

func installHTTPTransportTestGlobals(t *testing.T, svc Service) {
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

type httpTransportTestRoundTripper func(*http.Request) (*http.Response, error)

func (f httpTransportTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func doHTTPTransportTestRequest(t *testing.T, ctx context.Context, transport http.RoundTripper, url string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("HTTP request: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("HTTP response: status=%d body=%q read=%v close=%v", response.StatusCode, body, readErr, closeErr)
	}
}

func collectHTTPTransportTestMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &metrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return metrics
}

func httpTransportTestDurationCount(t *testing.T, reader *sdkmetric.ManualReader) uint64 {
	t.Helper()
	var count uint64
	for _, scope := range collectHTTPTransportTestMetrics(t, reader).ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "http.client.request.duration" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok || metric.Unit != "s" {
				t.Fatalf("unexpected client duration: %v", metric)
			}
			for _, point := range histogram.DataPoints {
				count += point.Count
			}
		}
	}
	return count
}
