package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestPGXTracerUsesServiceProvidersAndParent(t *testing.T) {
	telemetry := newTestTelemetry(t, true, true)
	foreign := newTestTelemetry(t, true, true)
	installTestGlobals(t, foreign.service)
	ctx, parent := telemetry.service.Tracer("test-parent").Start(t.Context(), "parent")
	defer parent.End()
	tracer := telemetry.service.PGXTracer()
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT $1", Args: []any{"secret"}})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	spans := telemetry.exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one query span, got %v", spans)
	}
	span := spans[0]
	if span.Name != "query SELECT" || span.SpanKind != trace.SpanKindClient || !span.Parent.Equal(parent.SpanContext()) || span.SpanContext.TraceID() != parent.SpanContext().TraceID() {
		t.Fatalf("incorrect query span or parent: %+v", span)
	}
	if got := pgxTestDurationCount(t, telemetry.reader); got != 1 {
		t.Fatalf("operation duration count = %d, want 1", got)
	}
	if len(foreign.exporter.GetSpans()) != 0 || len(collectPGXTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
		t.Fatal("PGX tracer used global providers")
	}
}

func TestPGXTracerKeepsSignalsIndependentOfGlobals(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		traces, metrics, parent, zero bool
	}{
		{name: "traces", traces: true, parent: true},
		{name: "metrics", metrics: true, parent: true},
		{name: "disabled", parent: true},
		{name: "zero value", parent: true, zero: true},
		{name: "without parent", traces: true, metrics: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			telemetry := newTestTelemetry(t, tt.traces, tt.metrics)
			foreign := newTestTelemetry(t, true, true)
			installTestGlobals(t, foreign.service)
			svc := telemetry.service
			if tt.zero {
				svc = Service{}
			}
			ctx := t.Context()
			if tt.parent {
				var parent trace.Span
				ctx, parent = foreign.service.Tracer("test-parent").Start(ctx, "parent")
				defer parent.End()
			}
			tracer := svc.PGXTracer()
			ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})
			wantSpans, wantMetrics := 0, uint64(0)
			if tt.traces && tt.parent {
				wantSpans = 1
			}
			if tt.metrics {
				wantMetrics = 1
			}
			if got := len(telemetry.exporter.GetSpans()); got != wantSpans {
				t.Fatalf("spans = %d, want %d", got, wantSpans)
			}
			if got := pgxTestDurationCount(t, telemetry.reader); got != wantMetrics {
				t.Fatalf("operation duration count = %d, want %d", got, wantMetrics)
			}
			if len(foreign.exporter.GetSpans()) != 0 || len(collectPGXTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
				t.Fatal("PGX tracer used global providers or the parent's provider")
			}
		})
	}
}

func TestPGXTracerMetricOperationNamesWithoutRecordingSpan(t *testing.T) {
	for _, tt := range []struct {
		name, sql, want string
		traces          bool
	}{
		{"compact SELECT without traces", "SELECT('private-literal')", "SELECT", false},
		{"INSERT without parent", "insert into private_table values ($1)", "INSERT", true},
		{"unknown SQL", "private-secret", "UNKNOWN", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			telemetry := newTestTelemetry(t, tt.traces, true)
			tracer := telemetry.service.PGXTracer()
			ctx := tracer.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: tt.sql, Args: []any{"private-argument"}})
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("private-error")})

			seen := make(map[string]bool)
			for _, scope := range collectPGXTestMetrics(t, telemetry.reader).ScopeMetrics {
				for _, metric := range scope.Metrics {
					var attrs attribute.Set
					switch metric.Name {
					case "db.client.operation.duration":
						data, ok := metric.Data.(metricdata.Histogram[float64])
						if !ok || len(data.DataPoints) != 1 || data.DataPoints[0].Count != 1 {
							t.Fatalf("unexpected operation duration: %+v", metric)
						}
						attrs = data.DataPoints[0].Attributes
					case "db.client.operation.errors":
						data, ok := metric.Data.(metricdata.Sum[int64])
						if !ok || len(data.DataPoints) != 1 || data.DataPoints[0].Value != 1 {
							t.Fatalf("unexpected operation errors: %+v", metric)
						}
						attrs = data.DataPoints[0].Attributes
					default:
						continue
					}
					seen[metric.Name] = true
					if name, ok := attrs.Value("db.operation.name"); !ok || name.AsString() != tt.want {
						t.Errorf("metric %s operation = %v, want %q", metric.Name, name, tt.want)
					}
					for _, attr := range attrs.ToSlice() {
						if strings.Contains(attr.Value.Emit(), "private") {
							t.Errorf("metric %s contains query data in %s", metric.Name, attr.Key)
						}
					}
				}
			}
			for _, name := range []string{"db.client.operation.duration", "db.client.operation.errors"} {
				if !seen[name] {
					t.Errorf("missing metric %s", name)
				}
			}
			if len(telemetry.exporter.GetSpans()) != 0 {
				t.Fatal("query recorded a span without a parent")
			}
		})
	}
}

func TestPGXTracerOmitsSQLAndParameters(t *testing.T) {
	for _, tt := range []struct {
		name, sql, want string
	}{
		{"parameterized", "SELECT 'private-literal', $1", "SELECT"},
		{"literal without whitespace", "SELECT('private-literal')", "SELECT"},
		{"comment without whitespace", "SELECT/*private-comment*/1", "SELECT"},
		{"semicolon", "SELECT;", "SELECT"},
		{"nested comment", "/* /* inner */ private-secret */ SELECT 1", "UNKNOWN"},
		{"invalid statement", "private_secret", "UNKNOWN"},
		{"empty", "", "UNKNOWN"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			telemetry := newTestTelemetry(t, true, false)
			ctx, parent := telemetry.service.Tracer("test-parent").Start(t.Context(), "parent")
			defer parent.End()
			tracer := telemetry.service.PGXTracer()
			query := pgx.TraceQueryStartData{SQL: tt.sql, Args: []any{"private-argument"}}
			queryCtx := tracer.TraceQueryStart(ctx, nil, query)
			tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{})
			prepareCtx := tracer.TracePrepareStart(ctx, nil, pgx.TracePrepareStartData{Name: "lookup", SQL: query.SQL})
			tracer.TracePrepareEnd(prepareCtx, nil, pgx.TracePrepareEndData{})
			batch := &pgx.Batch{}
			batch.Queue(query.SQL, query.Args...)
			batchCtx := tracer.TraceBatchStart(ctx, nil, pgx.TraceBatchStartData{Batch: batch})
			tracer.TraceBatchQuery(batchCtx, nil, pgx.TraceBatchQueryData{SQL: query.SQL, Args: query.Args})
			tracer.TraceBatchEnd(batchCtx, nil, pgx.TraceBatchEndData{})

			spans := telemetry.exporter.GetSpans()
			if len(spans) != 4 {
				t.Fatalf("expected query, prepare, batch query and batch spans, got %v", spans)
			}
			for i, want := range []string{"query " + tt.want, "prepare " + tt.want, "batch query " + tt.want, "batch start"} {
				if spans[i].Name != want {
					t.Errorf("span name = %q, want %q", spans[i].Name, want)
				}
				attrs := attribute.NewSet(spans[i].Attributes...)
				for _, key := range []attribute.Key{"db.query.text", "db.statement", otelpgx.QueryParametersKey} {
					if attrs.HasValue(key) {
						t.Errorf("span %q contains SQL or parameters in %q", spans[i].Name, key)
					}
				}
			}
		})
	}
}

func TestRecordPGXPoolStatsUsesServiceProviderAndAttributes(t *testing.T) {
	telemetry := newTestTelemetry(t, false, true)
	foreign := newTestTelemetry(t, true, true)
	installTestGlobals(t, foreign.service)
	cfg, err := pgxpool.ParseConfig("postgres://test@127.0.0.1:1/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MinConns, cfg.MaxConns = 0, 2
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := telemetry.service.RecordPGXPoolStats(pool,
		otelpgx.WithStatsMeterProvider(foreign.service.meterProvider),
		otelpgx.WithStatsAttributes(attribute.String("db.client.connection.pool.name", "primary")),
	); err != nil {
		t.Fatalf("register pool stats: %v", err)
	}

	metrics := collectPGXTestMetrics(t, telemetry.reader)
	for _, name := range []string{
		"pgxpool.acquires", "pgxpool.acquire_duration", "pgxpool.acquired_connections",
		"pgxpool.canceled_acquires", "pgxpool.constructing_connections", "pgxpool.empty_acquire",
		"pgxpool.idle_connections", "pgxpool.max_connections", "pgxpool.max_idle_destroys",
		"pgxpool.max_lifetime_destroys", "pgxpool.new_connections", "pgxpool.total_connections",
		"pgxpool.empty_acquire_wait_time",
	} {
		if !hasMetric(metrics, name) {
			t.Errorf("missing pool metric %q", name)
		}
	}
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			var points []metricdata.DataPoint[int64]
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				points = data.DataPoints
				if metric.Name == "pgxpool.acquire_duration" || metric.Name == "pgxpool.empty_acquire_wait_time" {
					if !data.IsMonotonic || metric.Unit != "ns" {
						t.Errorf("cumulative duration must be a monotonic sum in ns: %+v", metric)
					}
				}
			case metricdata.Gauge[int64]:
				points = data.DataPoints
			default:
				t.Fatalf("unexpected pool metric data: %+v", metric)
			}
			if len(points) != 1 {
				t.Fatalf("metric %s has %d points, want 1", metric.Name, len(points))
			}
			if name, ok := points[0].Attributes.Value("db.client.connection.pool.name"); !ok || name.AsString() != "primary" {
				t.Errorf("metric %s did not receive pool name: %v", metric.Name, points[0].Attributes)
			}
			if metric.Name == "pgxpool.max_connections" && points[0].Value != 2 {
				t.Errorf("max connections = %d, want 2", points[0].Value)
			}
		}
	}
	if len(collectPGXTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
		t.Fatal("pool stats used the global provider or provider from options")
	}
	t.Run("registration error", func(t *testing.T) {
		want := errors.New("register callback failed")
		svc := Service{meterProvider: pgxTestFailingMeterProvider{noopmetric.NewMeterProvider(), want}}
		if err := svc.RecordPGXPoolStats(pool); !errors.Is(err, want) {
			t.Fatalf("registration error = %v, want %v", err, want)
		}
	})
}

func TestRecordPGXPoolStatsDisabledDoesNotUseGlobalProvider(t *testing.T) {
	foreign := newTestTelemetry(t, true, true)
	installTestGlobals(t, foreign.service)
	cfg, err := pgxpool.ParseConfig("postgres://test@127.0.0.1:1/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, svc := range []Service{newTestTelemetry(t, true, false).service, {}} {
		if err := svc.RecordPGXPoolStats(pool, otelpgx.WithStatsMeterProvider(foreign.service.meterProvider)); err != nil {
			t.Fatal(err)
		}
	}
	if len(collectPGXTestMetrics(t, foreign.reader).ScopeMetrics) != 0 {
		t.Fatal("disabled pool metrics used the global provider or provider from options")
	}
}

func collectPGXTestMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return metrics
}

func pgxTestDurationCount(t *testing.T, reader *sdkmetric.ManualReader) uint64 {
	t.Helper()
	var count uint64
	for _, scope := range collectPGXTestMetrics(t, reader).ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "db.client.operation.duration" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok || metric.Unit != "s" {
				t.Fatalf("unexpected operation duration: %+v", metric)
			}
			for _, point := range histogram.DataPoints {
				count += point.Count
			}
		}
	}
	return count
}

type pgxTestFailingMeterProvider struct {
	metric.MeterProvider
	err error
}

func (p pgxTestFailingMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return pgxTestFailingMeter{p.MeterProvider.Meter(name, opts...), p.err}
}

type pgxTestFailingMeter struct {
	metric.Meter
	err error
}

func (m pgxTestFailingMeter) RegisterCallback(metric.Callback, ...metric.Observable) (metric.Registration, error) {
	return nil, m.err
}
