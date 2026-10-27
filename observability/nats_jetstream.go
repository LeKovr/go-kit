package observability

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/trace"
)

// JetStreamPublisher instruments synchronous publishes on an existing JetStream client.
type JetStreamPublisher struct {
	publisher jetstream.JetStream
	tracing   natsTracing
}

// JetStreamPublisher uses this Service's tracer and propagator without OTel globals.
func (svc Service) JetStreamPublisher(js jetstream.JetStream) *JetStreamPublisher {
	return &JetStreamPublisher{publisher: js, tracing: svc.natsTracing()}
}

// PublishMsg traces publication through PubAck or error, using a copy of msg's
// headers. PubAck confirms broker acceptance, not completion by a consumer.
func (p *JetStreamPublisher) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (ack *jetstream.PubAck, err error) {
	if ctx == nil {
		return nil, nats.ErrInvalidContext
	}
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	ctx, span := p.tracing.start(ctx, "publish", msg.Subject, len(msg.Data), trace.SpanKindProducer)
	defer endNATSSpan(span, &err)
	return p.publisher.PublishMsg(ctx, p.tracing.message(ctx, msg), opts...)
}

// JetStreamHandler adapts a context-aware callback for Consumer.Consume.
// Like NATSHandler it preserves base's local lifecycle and records returned errors.
// Ack, Nak and Term remain the handler's responsibility; their errors are recorded
// only if returned by the handler. The span covers the callback, including ACKs
// made before it returns. A new invocation creates a span for each redelivery.
func (svc Service) JetStreamHandler(base context.Context, handler func(context.Context, jetstream.Msg) error, opts ...NATSOption) jetstream.MessageHandler {
	cfg := newNATSHandlerConfig(base, handler == nil, opts)
	tracing := svc.natsTracing()
	return func(msg jetstream.Msg) {
		tracing.process(base, msg.Headers(), msg.Subject(), len(msg.Data()), cfg, func(ctx context.Context) error {
			return handler(ctx, msg)
		})
	}
}
