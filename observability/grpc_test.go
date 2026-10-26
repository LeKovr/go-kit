package observability

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestGRPCHandlersPropagateContextAndRecordTelemetry(t *testing.T) {
	clientTelemetry := newTestTelemetry(t, true, true)
	serverTelemetry := newTestTelemetry(t, true, true)
	impl := &grpcTestHealthServer{}
	client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(), serverTelemetry.service.GRPCServerHandler(), impl)
	ctx, parent := clientTelemetry.service.Tracer("test-parent").Start(grpcTestContext(t), "parent")
	defer parent.End()
	member, err := baggage.NewMember("test-key", "test-value")
	if err != nil {
		t.Fatal(err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatal(err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)

	response, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("check: response=%v, err=%v", response, err)
	}
	wait(ctx)
	clientSpan := grpcTestSpan(t, clientTelemetry.exporter, trace.SpanKindClient)
	serverSpan := grpcTestSpan(t, serverTelemetry.exporter, trace.SpanKindServer)
	if clientSpan.Parent.SpanID() != parent.SpanContext().SpanID() || serverSpan.Parent.SpanID() != clientSpan.SpanContext.SpanID() {
		t.Fatalf("unexpected parent chain: parent=%s client=%s server=%s", parent.SpanContext().SpanID(), clientSpan.Parent.SpanID(), serverSpan.Parent.SpanID())
	}
	if clientSpan.SpanContext.TraceID() != parent.SpanContext().TraceID() || serverSpan.SpanContext.TraceID() != parent.SpanContext().TraceID() || !serverSpan.Parent.IsRemote() {
		t.Fatal("trace context did not propagate from parent through client to server")
	}
	if got := baggage.FromContext(impl.receivedContext).Member("test-key").Value(); got != "test-value" {
		t.Fatalf("server baggage = %q, want test-value", got)
	}
	if got := trace.SpanContextFromContext(impl.receivedContext).SpanID(); got != serverSpan.SpanContext.SpanID() {
		t.Fatalf("handler span = %s, want %s", got, serverSpan.SpanContext.SpanID())
	}
	assertGRPCTestDuration(t, clientTelemetry.reader, "client", healthpb.Health_Check_FullMethodName, "OK", 1)
	assertGRPCTestDuration(t, serverTelemetry.reader, "server", healthpb.Health_Check_FullMethodName, "OK", 1)
}

func TestGRPCHandlersRecordInternalError(t *testing.T) {
	clientTelemetry := newTestTelemetry(t, true, true)
	serverTelemetry := newTestTelemetry(t, true, true)
	client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(), serverTelemetry.service.GRPCServerHandler(), &grpcTestHealthServer{checkError: status.Error(codes.Internal, "test failure")})
	ctx := grpcTestContext(t)
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Internal {
		t.Fatalf("check error = %v, want Internal", err)
	}
	wait(ctx)
	for _, tc := range []struct {
		telemetry testTelemetry
		kind      trace.SpanKind
		side      string
	}{
		{clientTelemetry, trace.SpanKindClient, "client"},
		{serverTelemetry, trace.SpanKindServer, "server"},
	} {
		span := grpcTestSpan(t, tc.telemetry.exporter, tc.kind)
		attrs := attribute.NewSet(span.Attributes...)
		code, _ := attrs.Value("rpc.response.status_code")
		if span.Status.Code != otelcodes.Error || code.AsString() != "INTERNAL" {
			t.Fatalf("%s span: status=%v, rpc status=%v", tc.side, span.Status, code)
		}
		assertGRPCTestDuration(t, tc.telemetry.reader, tc.side, healthpb.Health_Check_FullMethodName, "INTERNAL", 1)
	}
}

func TestGRPCHandlersFinishStreams(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		name := "EOF"
		if cancelStream {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			clientTelemetry := newTestTelemetry(t, true, true)
			serverTelemetry := newTestTelemetry(t, true, true)
			client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(), serverTelemetry.service.GRPCServerHandler(), &grpcTestHealthServer{waitForCancel: cancelStream})
			ctx := grpcTestContext(t)
			streamCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			stream, err := client.Watch(streamCtx, &healthpb.HealthCheckRequest{})
			if err != nil {
				t.Fatalf("watch: %v", err)
			}
			if response, err := stream.Recv(); err != nil || response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
				t.Fatalf("watch response=%v, err=%v", response, err)
			}
			if cancelStream {
				cancel()
			}
			_, err = stream.Recv()
			if cancelStream && status.Code(err) != codes.Canceled || !cancelStream && err != io.EOF {
				t.Fatalf("watch ended with %v", err)
			}
			wait(ctx)
			grpcTestSpan(t, clientTelemetry.exporter, trace.SpanKindClient)
			grpcTestSpan(t, serverTelemetry.exporter, trace.SpanKindServer)
			code := "OK"
			if cancelStream {
				code = "CANCELLED"
			}
			assertGRPCTestDuration(t, clientTelemetry.reader, "client", healthpb.Health_Watch_FullMethodName, code, 1)
			assertGRPCTestDuration(t, serverTelemetry.reader, "server", healthpb.Health_Watch_FullMethodName, code, 1)
		})
	}
}

func TestGRPCHandlersKeepSignalsIndependentOfGlobals(t *testing.T) {
	for _, tc := range []struct {
		name      string
		traces    bool
		metrics   bool
		zeroValue bool
	}{
		{name: "traces", traces: true},
		{name: "metrics", metrics: true},
		{name: "disabled"},
		{name: "zero value", zeroValue: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			globals := newTestTelemetry(t, true, true)
			installTestGlobals(t, globals.service)
			clientTelemetry := newTestTelemetry(t, tc.traces, tc.metrics)
			serverTelemetry := newTestTelemetry(t, tc.traces, tc.metrics)
			if !tc.traces && !tc.metrics {
				var svc Service
				if !tc.zeroValue {
					disabled, err := New(context.Background(), Config{}, "test", "test")
					if err != nil {
						t.Fatal(err)
					}
					svc = *disabled
				}
				clientTelemetry.service, serverTelemetry.service = svc, svc
			}
			client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(), serverTelemetry.service.GRPCServerHandler(), &grpcTestHealthServer{})
			ctx := grpcTestContext(t)
			if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
				t.Fatal(err)
			}
			wait(ctx)
			wantSpans, wantCount := 0, uint64(0)
			if tc.traces {
				wantSpans = 1
			}
			if tc.metrics {
				wantCount = 1
			}
			for side, telemetry := range map[string]testTelemetry{"client": clientTelemetry, "server": serverTelemetry} {
				if got := len(telemetry.exporter.GetSpans()); got != wantSpans {
					t.Fatalf("%s spans = %d, want %d", side, got, wantSpans)
				}
				assertGRPCTestDuration(t, telemetry.reader, side, healthpb.Health_Check_FullMethodName, "OK", wantCount)
			}
			assertNoGRPCTestTelemetry(t, globals)
		})
	}
}

func TestGRPCHandlersPassFiltersWithoutBreakingPropagation(t *testing.T) {
	for _, filteredSide := range []string{"client", "server", "both"} {
		t.Run(filteredSide, func(t *testing.T) {
			clientTelemetry := newTestTelemetry(t, true, true)
			serverTelemetry := newTestTelemetry(t, true, true)
			filter := otelgrpc.WithFilter(func(*stats.RPCTagInfo) bool { return false })
			var clientOptions, serverOptions []otelgrpc.Option
			if filteredSide != "server" {
				clientOptions = append(clientOptions, filter)
			}
			if filteredSide != "client" {
				serverOptions = append(serverOptions, filter)
			}
			impl := &grpcTestHealthServer{}
			client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(clientOptions...), serverTelemetry.service.GRPCServerHandler(serverOptions...), impl)
			parent := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
			})
			ctx := trace.ContextWithSpanContext(grpcTestContext(t), parent)
			if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
				t.Fatal(err)
			}
			wait(ctx)
			if got := trace.SpanContextFromContext(impl.receivedContext).TraceID(); got != parent.TraceID() {
				t.Fatalf("filtered RPC lost trace: got %s, want %s", got, parent.TraceID())
			}
			for side, telemetry := range map[string]testTelemetry{"client": clientTelemetry, "server": serverTelemetry} {
				if filteredSide == side || filteredSide == "both" {
					assertNoGRPCTestTelemetry(t, telemetry)
				} else {
					if got := len(telemetry.exporter.GetSpans()); got != 1 {
						t.Fatalf("%s spans = %d, want 1", side, got)
					}
					assertGRPCTestDuration(t, telemetry.reader, side, healthpb.Health_Check_FullMethodName, "OK", 1)
				}
			}
		})
	}
}

func TestGRPCHandlersKeepServiceProvidersAndPropagator(t *testing.T) {
	globals := newTestTelemetry(t, true, true)
	installTestGlobals(t, globals.service)
	override := newTestTelemetry(t, true, true)
	clientTelemetry := newTestTelemetry(t, true, true)
	serverTelemetry := newTestTelemetry(t, true, true)
	opts := []otelgrpc.Option{
		otelgrpc.WithTracerProvider(override.service.tracerProvider),
		otelgrpc.WithMeterProvider(override.service.meterProvider),
		otelgrpc.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelgrpc.WithSpanAttributes(attribute.String("test.attribute", "passed")),
	}
	impl := &grpcTestHealthServer{}
	client, wait := newGRPCTestClient(t, clientTelemetry.service.GRPCClientHandler(opts...), serverTelemetry.service.GRPCServerHandler(opts...), impl)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(grpcTestContext(t), parent)
	member, err := baggage.NewMember("test-key", "test-value")
	if err != nil {
		t.Fatal(err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatal(err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	wait(ctx)
	clientSpan := grpcTestSpan(t, clientTelemetry.exporter, trace.SpanKindClient)
	serverSpan := grpcTestSpan(t, serverTelemetry.exporter, trace.SpanKindServer)
	if clientSpan.Parent.SpanID() != parent.SpanID() || serverSpan.Parent.SpanID() != clientSpan.SpanContext.SpanID() || serverSpan.SpanContext.TraceID() != parent.TraceID() {
		t.Fatal("option or global propagator replaced service propagation")
	}
	if got := baggage.FromContext(impl.receivedContext).Member("test-key").Value(); got != "test-value" {
		t.Fatalf("server baggage = %q, want test-value", got)
	}
	for _, span := range []tracetest.SpanStub{clientSpan, serverSpan} {
		attrs := attribute.NewSet(span.Attributes...)
		if value, _ := attrs.Value("test.attribute"); value.AsString() != "passed" {
			t.Fatal("upstream span attribute option was not passed through")
		}
	}
	assertGRPCTestDuration(t, clientTelemetry.reader, "client", healthpb.Health_Check_FullMethodName, "OK", 1)
	assertGRPCTestDuration(t, serverTelemetry.reader, "server", healthpb.Health_Check_FullMethodName, "OK", 1)
	assertNoGRPCTestTelemetry(t, globals)
	assertNoGRPCTestTelemetry(t, override)
}

type grpcTestHealthServer struct {
	healthpb.UnimplementedHealthServer
	receivedContext context.Context
	checkError      error
	waitForCancel   bool
}

func (s *grpcTestHealthServer) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.receivedContext = ctx
	if s.checkError != nil {
		return nil, s.checkError
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *grpcTestHealthServer) Watch(_ *healthpb.HealthCheckRequest, stream grpc.ServerStreamingServer[healthpb.HealthCheckResponse]) error {
	if err := stream.Send(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}); err != nil {
		return err
	}
	if s.waitForCancel {
		<-stream.Context().Done()
		return status.FromContextError(stream.Context().Err()).Err()
	}
	return nil
}

type grpcTestEndHandler struct {
	stats.Handler
	ended chan struct{}
}

func (h grpcTestEndHandler) HandleRPC(ctx context.Context, event stats.RPCStats) {
	h.Handler.HandleRPC(ctx, event)
	if _, ok := event.(*stats.End); ok {
		// otelgrpc ends its span before recording metrics; wait for the whole callback.
		h.ended <- struct{}{}
	}
}

func newGRPCTestClient(t *testing.T, clientHandler, serverHandler stats.Handler, impl healthpb.HealthServer) (healthpb.HealthClient, func(context.Context)) {
	t.Helper()
	clientEnded, serverEnded := make(chan struct{}, 1), make(chan struct{}, 1)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.StatsHandler(grpcTestEndHandler{Handler: serverHandler, ended: serverEnded}))
	healthpb.RegisterHealthServer(server, impl)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(grpcTestEndHandler{Handler: clientHandler, ended: clientEnded}),
	)
	t.Cleanup(func() {
		stopped := make(chan struct{})
		go func() {
			if conn != nil {
				_ = conn.Close()
			}
			server.Stop()
			_ = listener.Close()
			close(stopped)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		select {
		case <-stopped:
		case <-ctx.Done():
			t.Error("gRPC cleanup timed out")
			return
		}
		select {
		case err := <-served:
			if err != nil && err != grpc.ErrServerStopped {
				t.Errorf("serve: %v", err)
			}
		case <-ctx.Done():
			t.Error("gRPC Serve did not exit")
		}
	})
	if err != nil {
		t.Fatalf("new gRPC client: %v", err)
	}
	wait := func(ctx context.Context) {
		t.Helper()
		for side, ended := range map[string]<-chan struct{}{"client": clientEnded, "server": serverEnded} {
			select {
			case <-ended:
			case <-ctx.Done():
				t.Fatalf("waiting for %s stats.End: %v", side, ctx.Err())
			}
		}
	}
	return healthpb.NewHealthClient(conn), wait
}

func grpcTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func grpcTestSpan(t *testing.T, exporter *tracetest.InMemoryExporter, kind trace.SpanKind) tracetest.SpanStub {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].SpanKind != kind {
		t.Fatalf("expected one %s span, got %v", kind, spans)
	}
	return spans[0]
}

func assertGRPCTestDuration(t *testing.T, reader *sdkmetric.ManualReader, side, method, code string, want uint64) {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(grpcTestContext(t), &metrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var count uint64
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "rpc."+side+".call.duration" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok || metric.Unit != "s" {
				t.Fatalf("unexpected duration metric: %v", metric)
			}
			for _, point := range histogram.DataPoints {
				gotMethod, _ := point.Attributes.Value("rpc.method")
				gotCode, _ := point.Attributes.Value("rpc.response.status_code")
				if gotMethod.AsString() != strings.TrimPrefix(method, "/") || gotCode.AsString() != code {
					t.Fatalf("unexpected duration attributes: %v", point.Attributes)
				}
				count += point.Count
			}
		}
	}
	if count != want {
		t.Fatalf("%s duration count = %d, want %d", side, count, want)
	}
}

func assertNoGRPCTestTelemetry(t *testing.T, telemetry testTelemetry) {
	t.Helper()
	if spans := telemetry.exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("unexpected spans: %v", spans)
	}
	var metrics metricdata.ResourceMetrics
	if err := telemetry.reader.Collect(grpcTestContext(t), &metrics); err != nil {
		t.Fatal(err)
	}
	if len(metrics.ScopeMetrics) != 0 {
		t.Fatalf("unexpected metrics: %v", metrics)
	}
}
