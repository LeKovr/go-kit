package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/observability/example/api"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type flowTraceReceiver struct {
	collectortrace.UnimplementedTraceServiceServer
	mu        sync.Mutex
	resources []*tracepb.ResourceSpans
}

func (r *flowTraceReceiver) Export(_ context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	r.mu.Lock()
	r.resources = append(r.resources, req.ResourceSpans...)
	r.mu.Unlock()
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func TestCalculationFlow(t *testing.T) {
	dsn := os.Getenv("PGX_TEST_DSN")
	if dsn == "" {
		t.Skip("set PGX_TEST_DSN to run the PostgreSQL calculation flow")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	otlpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	receiver := &flowTraceReceiver{}
	otlpServer := grpc.NewServer()
	collectortrace.RegisterTraceServiceServer(otlpServer, receiver)
	go otlpServer.Serve(otlpListener)
	t.Cleanup(otlpServer.Stop)

	services := make(map[string]*observability.Service)
	for _, role := range []string{"http-client", "http-gateway", "grpc-server", "nats-worker"} {
		obs, err := observability.New(ctx, observability.Config{
			EnableTraces: true, OTLPEndpoint: "http://" + otlpListener.Addr().String(), ShutdownTimeout: 5 * time.Second,
		}, serviceName(role), "test")
		if err != nil {
			t.Fatal(err)
		}
		services[role] = obs
		t.Cleanup(func() {
			if err := obs.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.Tracer = services["nats-worker"].PGXTracer()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := services["nats-worker"].RecordPGXPoolStats(pool); err != nil {
		t.Fatal(err)
	}

	broker, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go broker.Start()
	t.Cleanup(func() { broker.Shutdown(); broker.WaitForShutdown() })
	if !broker.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server did not start")
	}
	requestConn, err := nats.Connect(broker.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(requestConn.Close)
	workerConn, err := nats.Connect(broker.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workerConn.Close)
	requests, err := services["nats-worker"].Meter(application + "/nats-worker").Int64Counter("demo.custom.requests")
	if err != nil {
		t.Fatal(err)
	}
	workerBaggage := make(chan baggage.Baggage, 1)
	workerDone := make(chan struct{}, 1)
	worker := natsWorkerHandler(services["nats-worker"].NATSClient(workerConn), pool, 5*time.Second, logger, requests)
	handle := services["nats-worker"].NATSHandler(ctx, func(ctx context.Context, msg *nats.Msg) error {
		workerBaggage <- baggage.FromContext(ctx)
		return worker(ctx, msg)
	})
	sub, err := workerConn.QueueSubscribe(natsSubject, "demo-workers", func(msg *nats.Msg) {
		handle(msg)
		workerDone <- struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := workerConn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}

	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer(grpc.StatsHandler(services["grpc-server"].GRPCServerHandler()))
	api.RegisterCalculatorServer(grpcServer, &calculatorServer{
		client: services["grpc-server"].NATSClient(requestConn), timeout: 5 * time.Second, logger: logger,
	})
	go grpcServer.Serve(grpcListener)
	t.Cleanup(grpcServer.Stop)
	grpcConn, err := grpc.NewClient(grpcListener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(services["http-gateway"].GRPCClientHandler()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grpcConn.Close() })
	httpDone := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/demo", calculatorHTTPHandler(api.NewCalculatorClient(grpcConn), 5*time.Second, logger))
	httpHandler := services["http-gateway"].HTTPMiddleware()(mux)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHandler.ServeHTTP(w, r)
		httpDone <- struct{}{}
	}))
	t.Cleanup(httpServer.Close)
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	httpClient := &http.Client{Transport: services["http-client"].HTTPTransport(transport), Timeout: 5 * time.Second}
	request := func(ctx context.Context, wantStatus int, wantBody string) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/demo", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != wantStatus || string(body) != wantBody {
			t.Fatalf("HTTP response = (%d, %q), want (%d, %q)", resp.StatusCode, body, wantStatus, wantBody)
		}
		select {
		case <-httpDone:
		case <-ctx.Done():
			t.Fatal("HTTP handler did not finish")
		}
	}

	parent := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}).Extract(ctx,
		propagation.MapCarrier{
			"traceparent": "00-11111111111111111111111111111111-2222222222222222-01",
			"baggage":     "tenant=acme,job.id=flow-42",
		})
	request(parent, http.StatusOK, "ok")
	select {
	case <-workerDone:
	case <-ctx.Done():
		t.Fatal("NATS worker did not finish")
	}
	bag := <-workerBaggage
	if bag.Member("tenant").Value() != "acme" || bag.Member("job.id").Value() != "flow-42" {
		t.Fatalf("worker baggage = %q, want tenant=acme and job.id=flow-42", bag.String())
	}

	if err := sub.Unsubscribe(); err != nil {
		t.Fatal(err)
	}
	if err := workerConn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	noWorker := propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{
		"traceparent": "00-33333333333333333333333333333333-4444444444444444-01",
	})
	request(noWorker, http.StatusServiceUnavailable, "Service Unavailable\n")

	// Finish server callbacks before Shutdown flushes every provider to the receiver.
	grpcServer.GracefulStop()
	pool.Close()
	for _, obs := range services {
		if err := obs.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	receiver.mu.Lock()
	resources := append([]*tracepb.ResourceSpans(nil), receiver.resources...)
	receiver.mu.Unlock()

	parentSpan := trace.SpanContextFromContext(parent)
	traceID := parentSpan.TraceID()
	noWorkerID := trace.SpanContextFromContext(noWorker).TraceID()
	spans := make(map[string]*tracepb.Span)
	var failedRPC, failedNATS *tracepb.Span
	var dbSpans []*tracepb.Span
	for _, resource := range resources {
		serviceName := flowAttribute(resource.Resource.Attributes, "service.name")
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if bytes.Equal(span.TraceId, traceID[:]) {
					if serviceName == application+"-nats-worker" && flowAttribute(span.Attributes, "db.system.name") == "postgresql" {
						dbSpans = append(dbSpans, span)
						continue
					}
					key := serviceName + ":" + span.Name
					if spans[key] != nil {
						t.Fatalf("duplicate span %q", key)
					}
					spans[key] = span
				}
				if bytes.Equal(span.TraceId, noWorkerID[:]) && serviceName == application+"-grpc-server" {
					switch span.Kind {
					case tracepb.Span_SPAN_KIND_SERVER:
						failedRPC = span
					case tracepb.Span_SPAN_KIND_PRODUCER:
						failedNATS = span
					}
				}
			}
		}
	}
	method := strings.TrimPrefix(api.Calculator_Calculate_FullMethodName, "/")
	want := []struct {
		role, name string
		kind       tracepb.Span_SpanKind
	}{
		{"http-client", "HTTP GET", tracepb.Span_SPAN_KIND_CLIENT},
		{"http-gateway", "GET /demo", tracepb.Span_SPAN_KIND_SERVER},
		{"http-gateway", method, tracepb.Span_SPAN_KIND_CLIENT},
		{"grpc-server", method, tracepb.Span_SPAN_KIND_SERVER},
		{"grpc-server", "request " + natsSubject, tracepb.Span_SPAN_KIND_PRODUCER},
		{"nats-worker", "process " + natsSubject, tracepb.Span_SPAN_KIND_CONSUMER},
		{"nats-worker", "publish _INBOX.*", tracepb.Span_SPAN_KIND_PRODUCER},
	}
	parentID := parentSpan.SpanID()
	previous := parentID[:]
	for _, tt := range want {
		key := application + "-" + tt.role + ":" + tt.name
		span := spans[key]
		if span == nil {
			t.Fatalf("span %q missing", key)
		}
		if span.Kind != tt.kind || !bytes.Equal(span.ParentSpanId, previous) {
			t.Errorf("span %q kind/parent = (%v, %x), want (%v, %x)", key, span.Kind, span.ParentSpanId, tt.kind, previous)
		}
		previous = span.SpanId
	}
	workerSpan := spans[application+"-nats-worker:process "+natsSubject]
	var query, acquire *tracepb.Span
	for _, span := range dbSpans {
		for _, attr := range span.Attributes {
			switch attr.Key {
			case "db.query.text", "db.statement", "pgx.query.parameters":
				t.Errorf("DB span %q contains SQL or parameters in %q", span.Name, attr.Key)
			}
		}
		if !bytes.Equal(span.ParentSpanId, workerSpan.SpanId) {
			continue
		}
		switch span.Name {
		case "query SELECT":
			if flowAttribute(span.Attributes, "pgx.prepare_stmt.name") == "" {
				query = span
			}
		case "pool.acquire":
			acquire = span
		}
	}
	for name, span := range map[string]*tracepb.Span{"query SELECT": query, "pool.acquire": acquire} {
		if span == nil || span.Kind != tracepb.Span_SPAN_KIND_CLIENT {
			t.Errorf("worker is missing a DB client child %q: %v", name, span)
		}
	}
	if failedRPC == nil || flowAttribute(failedRPC.Attributes, "rpc.response.status_code") != "UNAVAILABLE" {
		t.Errorf("missing worker did not produce gRPC UNAVAILABLE: %v", failedRPC)
	}
	if failedNATS == nil || flowAttribute(failedNATS.Attributes, "error.type") != "no_responders" {
		t.Errorf("missing worker did not produce NATS no_responders: %v", failedNATS)
	}
}

func flowAttribute(attrs []*commonpb.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}
