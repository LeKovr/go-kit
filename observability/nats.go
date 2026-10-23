package observability

import (
	"context"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NATSOption configures Core NATS and JetStream message handlers.
type NATSOption func(*natsHandlerConfig)

type natsHandlerConfig struct {
	newTrace bool
}

// WithNATSNewTrace starts a new trace and links it to the incoming message.
// Baggage and the handler's local context are preserved. With tracing disabled,
// the incoming context is propagated unchanged by the noop tracer.
func WithNATSNewTrace() NATSOption {
	return func(cfg *natsHandlerConfig) { cfg.newTrace = true }
}

// NATSClient instruments sends on an existing connection. The caller owns its lifecycle.
type NATSClient struct {
	conn    *nats.Conn
	tracing natsTracing
}

// NATSClient uses this Service's tracer and propagator without OTel globals.
func (svc Service) NATSClient(conn *nats.Conn) *NATSClient {
	return &NATSClient{conn: conn, tracing: svc.natsTracing()}
}

// PublishMsg injects context into a copy of msg's headers and traces the publish call.
// ctx supplies telemetry context; Core NATS publish does not wait for delivery or
// support context cancellation. Data must not be mutated during the call.
func (c *NATSClient) PublishMsg(ctx context.Context, msg *nats.Msg) (err error) {
	if ctx == nil {
		return nats.ErrInvalidContext
	}
	if msg == nil {
		return nats.ErrInvalidMsg
	}
	ctx, span := c.tracing.start(ctx, "publish", msg.Subject, len(msg.Data), trace.SpanKindProducer)
	defer endNATSSpan(span, &err)
	return c.conn.PublishMsg(c.tracing.message(ctx, msg))
}

// RequestMsg traces the entire request until a reply, timeout or error. The reply
// does not replace the caller's context or create a separate receive span.
func (c *NATSClient) RequestMsg(ctx context.Context, msg *nats.Msg) (reply *nats.Msg, err error) {
	if ctx == nil {
		return nil, nats.ErrInvalidContext
	}
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	ctx, span := c.tracing.start(ctx, "request", msg.Subject, len(msg.Data), trace.SpanKindClient)
	defer endNATSSpan(span, &err)
	return c.conn.RequestMsgWithContext(ctx, c.tracing.message(ctx, msg))
}

// NATSHandler adapts a context-aware handler for Subscribe or QueueSubscribe.
// base supplies local cancellation, deadlines and values. Incoming trace context
// and baggage are extracted for each invocation. Returned errors affect only the
// span; replies, logging and retry policy belong to the handler. Panics are recorded
// and rethrown. The process span ends when the handler returns.
func (svc Service) NATSHandler(base context.Context, handler func(context.Context, *nats.Msg) error, opts ...NATSOption) nats.MsgHandler {
	cfg := newNATSHandlerConfig(base, handler == nil, opts)
	tracing := svc.natsTracing()
	return func(msg *nats.Msg) {
		tracing.process(base, msg.Header, msg.Subject, len(msg.Data), cfg, func(ctx context.Context) error {
			return handler(ctx, msg)
		})
	}
}

type natsTracing struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

func (svc Service) natsTracing() natsTracing {
	return natsTracing{tracer: svc.Tracer(svc.serviceName), propagator: svc.propagator()}
}

func newNATSHandlerConfig(base context.Context, nilHandler bool, opts []NATSOption) natsHandlerConfig {
	if base == nil || nilHandler {
		panic("observability: NATS handler requires a context and callback")
	}
	var cfg natsHandlerConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func (t natsTracing) start(ctx context.Context, operation, subject string, size int, kind trace.SpanKind, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts, trace.WithSpanKind(kind), trace.WithAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.operation.name", operation),
		attribute.String("messaging.destination.name", subject),
		attribute.Int("messaging.message.body.size", size),
	))
	// Reply inboxes are unique per request; keep span names stable.
	name := subject
	if strings.HasPrefix(subject, "_INBOX.") {
		name = "_INBOX.*"
	}
	return t.tracer.Start(ctx, operation+" "+name, opts...)
}

func (t natsTracing) message(ctx context.Context, msg *nats.Msg) *nats.Msg {
	copyMsg := *msg
	copyMsg.Header = make(nats.Header, len(msg.Header)+3)
	for key, values := range msg.Header {
		copyMsg.Header[key] = append([]string(nil), values...)
	}
	// A reused message must not send stale context when the new context is empty.
	for _, field := range t.propagator.Fields() {
		for key := range copyMsg.Header {
			if strings.EqualFold(key, field) {
				delete(copyMsg.Header, key)
			}
		}
	}
	t.propagator.Inject(ctx, natsHeaderCarrier(copyMsg.Header))
	return &copyMsg
}

func (t natsTracing) process(base context.Context, headers nats.Header, subject string, size int, cfg natsHandlerConfig, handler func(context.Context) error) {
	ctx := t.propagator.Extract(base, natsHeaderCarrier(headers))
	var opts []trace.SpanStartOption
	incoming := trace.SpanContextFromContext(ctx)
	local := trace.SpanContextFromContext(base)
	if cfg.newTrace {
		opts = append(opts, trace.WithNewRoot())
		if incoming.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: incoming}))
		}
	}
	// Preserve correlation if extraction replaced an active local span.
	if local.IsValid() && (local.TraceID() != incoming.TraceID() || local.SpanID() != incoming.SpanID()) {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: local}))
	}
	ctx, span := t.start(ctx, "process", subject, size, trace.SpanKindConsumer, opts...)
	var err error
	defer endNATSSpan(span, &err)
	err = handler(ctx)
}

func endNATSSpan(span trace.Span, err *error) {
	if recovered := recover(); recovered != nil {
		span.RecordError(fmt.Errorf("panic: %v", recovered))
		span.SetStatus(codes.Error, "panic in NATS operation")
		span.End()
		panic(recovered)
	}
	if *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	}
	span.End()
}
