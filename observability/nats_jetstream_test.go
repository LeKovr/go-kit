package observability

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestJetStreamPublishAndProcess(t *testing.T) {
	conn := connectNATSTest(t)
	js, err := jetstream.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = natsTestParent(ctx)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVENTS", Subjects: []string{"js.events"}, Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "worker", AckPolicy: jetstream.AckExplicitPolicy, AckWait: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, exporter := newNATSTestService(t)
	type delivery struct {
		ctx context.Context
		msg jetstream.Msg
	}
	received := make(chan delivery, 1)
	gate, release := context.WithCancel(t.Context())
	defer release()
	handlerErr := errors.New("consumer failed")
	consumption, err := consumer.Consume(svc.JetStreamHandler(t.Context(), func(ctx context.Context, msg jetstream.Msg) error {
		received <- delivery{ctx, msg}
		<-gate.Done()
		return handlerErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumption.Stop)
	originalHeaders := nats.Header{"Traceparent": {"stale"}, "Baggage": {"stale=value"}, "X-Test": {"untouched"}}
	msg := &nats.Msg{Subject: "js.events", Data: []byte("event"), Header: nats.Header{
		"Traceparent": {"stale"}, "Baggage": {"stale=value"}, "X-Test": {"untouched"},
	}}
	publisher := svc.JetStreamPublisher(js)
	ack, err := publisher.PublishMsg(ctx, msg, jetstream.WithMsgID("event-1"))
	if err != nil {
		t.Fatal(err)
	}
	if ack.Stream != "EVENTS" || ack.Sequence != 1 || ack.Duplicate {
		t.Fatalf("unexpected PubAck: %+v", ack)
	}
	if !reflect.DeepEqual(msg.Header, originalHeaders) || msg.Reply != "" || string(msg.Data) != "event" {
		t.Fatalf("JetStream publish mutated original message: %+v", msg)
	}
	var got delivery
	select {
	case got = <-received:
	case <-ctx.Done():
		t.Fatal("JetStream consumer did not receive the message")
	}
	if string(got.msg.Data()) != "event" || baggage.FromContext(got.ctx).Member("tenant").Value() != "acme" {
		t.Fatal("consumer lost payload or baggage")
	}
	publish := takeNATSSpans(t, exporter, 1)["publish js.events"]
	if publish == nil || publish.SpanKind() != trace.SpanKindProducer || publish.Parent().SpanID() != trace.SpanContextFromContext(ctx).SpanID() {
		t.Fatal("publish span must end at PubAck while the handler is still running")
	}
	if len(exporter) != 0 {
		t.Fatal("process span ended before handler return")
	}
	release()
	process := takeNATSSpans(t, exporter, 1)["process js.events"]
	if process == nil || process.SpanKind() != trace.SpanKindConsumer || process.Parent().SpanID() != publish.SpanContext().SpanID() {
		t.Fatal("process span did not continue the published message")
	}
	if process.Status().Code != codes.Error || len(process.Events()) == 0 {
		t.Fatal("consumer error was not recorded")
	}
	info, err := consumer.Info(ctx)
	if err != nil || info.NumAckPending != 1 {
		t.Fatalf("handler must leave ACK ownership to caller: info=%+v, err=%v", info, err)
	}
	if err := got.msg.DoubleAck(ctx); err != nil {
		t.Fatal(err)
	}
	info, err = consumer.Info(ctx)
	if err != nil || info.NumAckPending != 0 {
		t.Fatalf("explicit ACK did not complete: info=%+v, err=%v", info, err)
	}
	ack, err = publisher.PublishMsg(ctx, msg, jetstream.WithMsgID("event-1"))
	if err != nil || !ack.Duplicate {
		t.Fatalf("publish options were not forwarded: ack=%+v, err=%v", ack, err)
	}
	takeNATSSpans(t, exporter, 1)
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := publisher.PublishMsg(cancelled, msg); err != context.Canceled {
		t.Fatalf("native cancellation error changed: %v", err)
	}
	failed := takeNATSSpans(t, exporter, 1)["publish js.events"]
	if failed == nil || failed.Status().Code != codes.Error || len(failed.Events()) == 0 {
		t.Fatal("JetStream publish error was not recorded")
	}
}
