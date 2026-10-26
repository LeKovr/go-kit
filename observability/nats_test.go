package observability

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type natsTestExporter chan sdktrace.ReadOnlySpan

func (e natsTestExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		e <- span
	}
	return nil
}

func (e natsTestExporter) Shutdown(context.Context) error { return nil }

func newNATSTestService(t *testing.T) (Service, natsTestExporter) {
	t.Helper()
	exporter := make(natsTestExporter, 32)
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return Service{config: Config{EnableTraces: true}, serviceName: "test-nats", tracerProvider: provider}, exporter
}

func connectNATSTest(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	t.Cleanup(func() { srv.Shutdown(); srv.WaitForShutdown() })
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server did not start")
	}
	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return conn
}

func natsTestTraceHeader(traceID, spanID byte) string {
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{traceID}, SpanID: trace.SpanID{spanID}, TraceFlags: trace.FlagsSampled,
	})
	headers := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(context.Background(), parent), headers)
	return headers.Get("traceparent")
}

func natsTestParent(ctx context.Context) context.Context {
	return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}).Extract(ctx,
		propagation.MapCarrier{"traceparent": natsTestTraceHeader(1, 2), "baggage": "tenant=acme"})
}

func takeNATSSpans(t *testing.T, exporter natsTestExporter, count int) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	spans := make(map[string]sdktrace.ReadOnlySpan, count)
	for range count {
		select {
		case span := <-exporter:
			if span.InstrumentationScope().Name != "test-nats" {
				t.Errorf("span %q scope = %q, want service name", span.Name(), span.InstrumentationScope().Name)
			}
			spans[span.Name()] = span
		case <-timer.C:
			t.Fatalf("received %d spans, want %d", len(spans), count)
		}
	}
	return spans
}

func poisonNATSGlobals(t *testing.T) natsTestExporter {
	t.Helper()
	globalService, exporter := newNATSTestService(t)
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(globalService.tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	return exporter
}

func TestNATSSpanAttributes(t *testing.T) {
	for _, tt := range []struct {
		name, operation, operationType, subject, spanName, template string
		kind                                                        trace.SpanKind
	}{
		{"publish", "publish", "send", "jobs.created", "publish jobs.created", "", trace.SpanKindProducer},
		{"request", "request", "send", "jobs.calculate", "request jobs.calculate", "", trace.SpanKindProducer},
		{"process", "process", "process", "jobs.created", "process jobs.created", "", trace.SpanKindConsumer},
		{"reply", "publish", "send", "_INBOX.unique", "publish _INBOX.*", "_INBOX.*", trace.SpanKindProducer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, exporter := newNATSTestService(t)
			_, span := svc.natsTracing().start(t.Context(), tt.operation, tt.subject, 3, tt.kind)
			span.End()
			got := takeNATSSpans(t, exporter, 1)[tt.spanName]
			if got == nil {
				t.Fatalf("span %q missing", tt.spanName)
			}
			attrs := attribute.NewSet(got.Attributes()...)
			for _, want := range []attribute.KeyValue{
				attribute.String("messaging.system", "nats"),
				attribute.String("messaging.operation.name", tt.operation),
				attribute.String("messaging.operation.type", tt.operationType),
				attribute.String("messaging.destination.name", tt.subject),
				attribute.Int("messaging.message.body.size", 3),
			} {
				if value, ok := attrs.Value(want.Key); !ok || value != want.Value {
					t.Errorf("%s = %v, present = %t; want %v", want.Key, value, ok, want.Value)
				}
			}
			value, ok := attrs.Value("messaging.destination.template")
			if ok != (tt.template != "") || (ok && value.AsString() != tt.template) {
				t.Errorf("destination template = %v, present = %t; want %q", value, ok, tt.template)
			}
			for _, key := range []attribute.Key{"error.type", "messaging.batch.message_count"} {
				if attrs.HasValue(key) {
					t.Errorf("successful single-message span must not set %s", key)
				}
			}
		})
	}
}

func TestNATSClientRequestReply(t *testing.T) {
	globalSpans := poisonNATSGlobals(t)
	conn := connectNATSTest(t)
	svc, exporter := newNATSTestService(t)
	client := svc.NATSClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = natsTestParent(ctx)
	received := make(chan context.Context, 1)
	_, err := conn.Subscribe("core.request", svc.NATSHandler(t.Context(), func(ctx context.Context, msg *nats.Msg) error {
		received <- ctx
		return client.PublishMsg(ctx, &nats.Msg{Subject: msg.Reply, Data: []byte("worker reply")})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	originalHeaders := nats.Header{"Traceparent": {"stale"}, "Baggage": {"stale=value"}, "X-Test": {"untouched"}}
	msg := &nats.Msg{Subject: "core.request", Data: []byte("question"), Header: nats.Header{
		"Traceparent": {"stale"}, "Baggage": {"stale=value"}, "X-Test": {"untouched"},
	}}
	reply, err := client.RequestMsg(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if string(reply.Data) != "worker reply" {
		t.Fatalf("reply = %q", reply.Data)
	}
	if !reflect.DeepEqual(msg.Header, originalHeaders) || msg.Reply != "" || string(msg.Data) != "question" {
		t.Fatalf("request message mutated: %+v", msg)
	}
	handlerCtx := <-received
	if bag := baggage.FromContext(handlerCtx); bag.Member("tenant").Value() != "acme" || bag.Member("stale").Value() != "" {
		t.Fatalf("handler baggage = %s", bag)
	}
	spans := takeNATSSpans(t, exporter, 3)
	request, process, publish := spans["request core.request"], spans["process core.request"], spans["publish _INBOX.*"]
	if request == nil || process == nil || publish == nil {
		t.Fatalf("unexpected span names: %v", spans)
	}
	if request.SpanKind() != trace.SpanKindProducer || request.Parent().SpanID() != trace.SpanContextFromContext(ctx).SpanID() {
		t.Error("request span did not continue caller context as a producer span")
	}
	attrs := attribute.NewSet(request.Attributes()...)
	for key, want := range map[attribute.Key]string{"messaging.operation.name": "request", "messaging.operation.type": "send"} {
		if value, ok := attrs.Value(key); !ok || value.AsString() != want {
			t.Errorf("request %s = %v, want %s", key, value, want)
		}
	}
	for _, span := range spans {
		attrs := attribute.NewSet(span.Attributes()...)
		if attrs.HasValue("error.type") {
			t.Errorf("successful span %q must not set error.type", span.Name())
		}
	}
	if process.Parent().SpanID() != request.SpanContext().SpanID() || process.SpanKind() != trace.SpanKindConsumer {
		t.Error("process span did not continue request span")
	}
	if publish.Parent().SpanID() != process.SpanContext().SpanID() || publish.SpanKind() != trace.SpanKindProducer {
		t.Error("reply publish did not continue process span")
	}
	wireCtx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": reply.Header.Get("traceparent")})
	if trace.SpanContextFromContext(wireCtx).SpanID() != publish.SpanContext().SpanID() {
		t.Error("reply did not carry its publish span")
	}
	if len(exporter) != 0 || len(globalSpans) != 0 {
		t.Fatal("unexpected extra span or global provider used")
	}
}

func TestNATSClientPublishWithoutReply(t *testing.T) {
	conn := connectNATSTest(t)
	svc, exporter := newNATSTestService(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = natsTestParent(ctx)
	gate, release := context.WithCancel(t.Context())
	defer release()
	received := make(chan context.Context, 1)
	_, err := conn.Subscribe("core.publish", svc.NATSHandler(t.Context(), func(ctx context.Context, msg *nats.Msg) error {
		if msg.Reply != "" || string(msg.Data) != "event" {
			t.Errorf("unexpected published message: %+v", msg)
		}
		received <- ctx
		<-gate.Done()
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	published := make(chan error, 1)
	go func() {
		published <- svc.NATSClient(conn).PublishMsg(ctx, &nats.Msg{Subject: "core.publish", Data: []byte("event")})
	}()
	select {
	case err := <-published:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("publish waited for handler completion or a reply")
	}
	var handlerCtx context.Context
	select {
	case handlerCtx = <-received:
	case <-ctx.Done():
		t.Fatal("consumer did not receive the message")
	}
	publish := takeNATSSpans(t, exporter, 1)["publish core.publish"]
	if publish == nil || publish.Parent().SpanID() != trace.SpanContextFromContext(ctx).SpanID() {
		t.Fatal("publish did not continue the caller's span")
	}
	if trace.SpanContextFromContext(handlerCtx).TraceID() != publish.SpanContext().TraceID() || baggage.FromContext(handlerCtx).Member("tenant").Value() != "acme" {
		t.Fatal("consumer did not preserve the published trace and baggage")
	}
	if len(exporter) != 0 {
		t.Fatal("process span ended before handler completion")
	}
	release()
	process := takeNATSSpans(t, exporter, 1)["process core.publish"]
	if process == nil || process.Parent().SpanID() != publish.SpanContext().SpanID() || process.Status().Code == codes.Error {
		t.Fatal("process did not successfully continue the publish span")
	}
}

func TestNATSHandlerContext(t *testing.T) {
	for _, tt := range []struct {
		name      string
		newTrace  bool
		duplicate bool
		localSpan bool
	}{
		{"parent", false, false, false},
		{"new trace", true, false, false},
		{"duplicate traceparent", false, true, false},
		{"same local span", false, false, true},
		{"new trace with same local span", true, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, exporter := newNATSTestService(t)
			type key struct{}
			base, cancel := context.WithTimeout(context.WithValue(t.Context(), key{}, "local"), time.Minute)
			defer cancel()
			if tt.localSpan {
				incoming := trace.SpanContextFromContext(natsTestParent(context.Background()))
				base = trace.ContextWithSpanContext(base, incoming.WithRemote(false))
			}
			deadline, _ := base.Deadline()
			headers := nats.Header{
				"TRACEPARENT": {natsTestTraceHeader(1, 2)}, "baggage": {"tenant=acme", "region=eu"}, "Baggage": {"tier=gold"},
			}
			if tt.duplicate {
				other := natsTestTraceHeader(3, 4)
				headers["TRACEPARENT"] = append(headers["TRACEPARENT"], other)
				headers["traceparent"] = []string{other}
			}
			var options []NATSOption
			if tt.newTrace {
				options = append(options, WithNATSNewTrace())
			}
			var got context.Context
			svc.NATSHandler(base, func(ctx context.Context, _ *nats.Msg) error { got = ctx; return nil }, options...)(
				&nats.Msg{Subject: "events", Header: headers})
			if got == nil || got.Value(key{}) != "local" {
				t.Fatal("handler lost local context")
			}
			if gotDeadline, ok := got.Deadline(); !ok || gotDeadline != deadline {
				t.Fatal("handler lost local deadline")
			}
			bag := baggage.FromContext(got)
			if bag.Member("tenant").Value() != "acme" || bag.Member("region").Value() != "eu" || bag.Member("tier").Value() != "gold" {
				t.Fatalf("multi-value baggage lost: %s", bag)
			}
			cancel()
			if got.Err() != context.Canceled {
				t.Fatal("handler context did not inherit cancellation")
			}
			span := takeNATSSpans(t, exporter, 1)["process events"]
			if span == nil {
				t.Fatal("process span missing")
			}
			incoming := trace.SpanContextFromContext(natsTestParent(context.Background()))
			switch {
			case tt.newTrace:
				if span.Parent().IsValid() || span.SpanContext().TraceID() == incoming.TraceID() || len(span.Links()) != 1 || span.Links()[0].SpanContext.SpanID() != incoming.SpanID() {
					t.Fatal("new trace must have no parent and one link to incoming span")
				}
			default:
				if span.Parent().SpanID() != incoming.SpanID() || span.SpanContext().TraceID() != incoming.TraceID() || len(span.Links()) != 0 {
					t.Fatal("default handler must continue incoming trace")
				}
			}
		})
	}
}

func TestNATSPropagationWithoutTracing(t *testing.T) {
	globalSpans := poisonNATSGlobals(t)
	conn := connectNATSTest(t)
	svc, err := New(t.Context(), Config{}, "disabled", "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = natsTestParent(ctx)
	received := make(chan context.Context, 1)
	_, err = conn.Subscribe("disabled", svc.NATSHandler(t.Context(), func(ctx context.Context, msg *nats.Msg) error {
		received <- ctx
		return msg.Respond([]byte("ok"))
	}, WithNATSNewTrace()))
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NATSClient(conn).RequestMsg(ctx, &nats.Msg{Subject: "disabled"}); err != nil {
		t.Fatal(err)
	}
	plain, err := conn.SubscribeSync("without.context")
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	stale := &nats.Msg{Subject: "without.context", Header: nats.Header{
		"traceparent": {natsTestTraceHeader(1, 2)}, "Traceparent": {natsTestTraceHeader(1, 2)},
		"baggage": {"tenant=stale"}, "Baggage": {"other=stale"}, "X-Test": {"keep"},
	}}
	if err := svc.NATSClient(conn).PublishMsg(t.Context(), stale); err != nil {
		t.Fatal(err)
	}
	wire, err := plain.NextMsgWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire.Header, nats.Header{"X-Test": {"keep"}}) || len(stale.Header) != 5 {
		t.Fatalf("stale propagation was forwarded or original headers changed: wire=%v, original=%v", wire.Header, stale.Header)
	}
	got := <-received
	if trace.SpanContextFromContext(got).SpanID() != trace.SpanContextFromContext(ctx).SpanID() || baggage.FromContext(got).Member("tenant").Value() != "acme" {
		t.Fatal("disabled tracing or new-trace option discarded propagation")
	}
	if len(globalSpans) != 0 {
		t.Fatal("disabled service used the global recording provider")
	}
}

func TestNATSClientErrors(t *testing.T) {
	conn := connectNATSTest(t)
	svc, exporter := newNATSTestService(t)
	client := svc.NATSClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := client.RequestMsg(ctx, &nats.Msg{Subject: "no.responders"}); err != nats.ErrNoResponders {
		t.Fatalf("no responders error = %v", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := client.RequestMsg(cancelled, &nats.Msg{Subject: "cancelled"}); err != context.Canceled {
		t.Fatalf("cancelled request error = %v", err)
	}
	if _, err := conn.Subscribe("deadline", func(*nats.Msg) {}); err != nil {
		t.Fatal(err)
	}
	if err := conn.FlushWithContext(ctx); err != nil {
		t.Fatal(err)
	}
	deadline, expire := context.WithTimeout(ctx, 20*time.Millisecond)
	defer expire()
	if _, err := client.RequestMsg(deadline, &nats.Msg{Subject: "deadline"}); err != context.DeadlineExceeded {
		t.Fatalf("deadline error = %v", err)
	}
	conn.Close()
	if err := client.PublishMsg(ctx, &nats.Msg{Subject: "closed"}); err != nats.ErrConnectionClosed {
		t.Fatalf("closed publish error = %v", err)
	}
	if _, err := client.RequestMsg(ctx, &nats.Msg{Subject: "closed"}); err != nats.ErrConnectionClosed {
		t.Fatalf("closed request error = %v", err)
	}
	spans := takeNATSSpans(t, exporter, 5)
	for name, want := range map[string]string{
		"request no.responders": "no_responders",
		"request cancelled":     "canceled",
		"request deadline":      "timeout",
		"publish closed":        "connection_closed",
		"request closed":        "connection_closed",
	} {
		span := spans[name]
		if span == nil {
			t.Fatalf("span %q missing", name)
		}
		attrs := attribute.NewSet(span.Attributes()...)
		if value, ok := attrs.Value("error.type"); !ok || value.AsString() != want {
			t.Errorf("error.type on %q = %v, want %s", name, value, want)
		}
		if span.Status().Code != codes.Error || len(span.Events()) == 0 {
			t.Errorf("transport error was not recorded on %q", span.Name())
		}
	}
	if err := client.PublishMsg(ctx, nil); err != nats.ErrInvalidMsg {
		t.Fatalf("nil publish message error = %v", err)
	}
	if _, err := client.RequestMsg(nil, &nats.Msg{Subject: "invalid"}); err != nats.ErrInvalidContext {
		t.Fatalf("nil request context error = %v", err)
	}
}

func TestNATSHandlerErrorAndPanic(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		errorType string
		panics    bool
	}{
		{"success", nil, "", false},
		{"other", errors.New("handler failed"), "_OTHER", false},
		{"deadline", context.DeadlineExceeded, "timeout", false},
		{"timeout", nats.ErrTimeout, "timeout", false},
		{"canceled", context.Canceled, "canceled", false},
		{"no responders", nats.ErrNoResponders, "no_responders", false},
		{"closed", nats.ErrConnectionClosed, "connection_closed", false},
		{"wrapped deadline", fmt.Errorf("handle: %w", context.DeadlineExceeded), "timeout", false},
		{"wrapped timeout", fmt.Errorf("handle: %w", nats.ErrTimeout), "timeout", false},
		{"wrapped canceled", fmt.Errorf("handle: %w", context.Canceled), "canceled", false},
		{"wrapped no responders", fmt.Errorf("handle: %w", nats.ErrNoResponders), "no_responders", false},
		{"wrapped closed", fmt.Errorf("handle: %w", nats.ErrConnectionClosed), "connection_closed", false},
		{"panic", errors.New("handler panicked"), "panic", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, exporter := newNATSTestService(t)
			base, cancel := context.WithCancel(t.Context())
			cancel()
			handler := svc.NATSHandler(base, func(ctx context.Context, _ *nats.Msg) error {
				if ctx.Err() != context.Canceled {
					t.Error("handler lost cancelled lifecycle context")
				}
				if tt.panics {
					panic(tt.err)
				}
				return tt.err
			})
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				handler(&nats.Msg{Subject: "result"})
			}()
			if (tt.panics && recovered != tt.err) || (!tt.panics && recovered != nil) {
				t.Fatalf("panic = %v, panics = %t", recovered, tt.panics)
			}
			span := takeNATSSpans(t, exporter, 1)["process result"]
			if span == nil {
				t.Fatal("handler span missing")
			}
			if tt.err != nil && (span.Status().Code != codes.Error || len(span.Events()) == 0) {
				t.Fatal("handler failure was not recorded")
			}
			if tt.err == nil && (span.Status().Code == codes.Error || len(span.Events()) != 0) {
				t.Fatal("successful handler recorded an error")
			}
			attrs := attribute.NewSet(span.Attributes()...)
			if value, ok := attrs.Value("error.type"); ok != (tt.errorType != "") || (ok && value.AsString() != tt.errorType) {
				t.Errorf("error.type = %v, present = %t; want %q", value, ok, tt.errorType)
			}
		})
	}
}
