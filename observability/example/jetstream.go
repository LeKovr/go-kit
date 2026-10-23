package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/baggage"
)

const (
	jetStreamName     = "DEMO_CALCULATE"
	jetStreamSubject  = "demo.calculate.js"
	jetStreamConsumer = "demo-workers"
)

func jetStreamHTTPHandler(publish func(context.Context, *nats.Msg, ...jetstream.PublishOpt) (*jetstream.PubAck, error), timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		ack, err := publish(ctx, &nats.Msg{Subject: jetStreamSubject, Data: []byte("calculate")})
		if err != nil {
			writeNATSError(w, ctx, logger, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, "accepted stream=%s sequence=%d\n", ack.Stream, ack.Sequence)
	}
}

func startJetStreamWorker(ctx context.Context, conn *nats.Conn, timeout time.Duration, obs *observability.Service, handler *DemoHandler, logger *slog.Logger) (jetstream.ConsumeContext, error) {
	js, err := jetstream.New(conn)
	if err != nil {
		return nil, err
	}
	setupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err = js.CreateOrUpdateStream(setupCtx, jetstream.StreamConfig{
		Name:      jetStreamName,
		Subjects:  []string{jetStreamSubject},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.WorkQueuePolicy,
	})
	if err != nil {
		return nil, err
	}
	consumer, err := js.CreateOrUpdateConsumer(setupCtx, jetStreamName, jetstream.ConsumerConfig{
		Durable:       jetStreamConsumer,
		FilterSubject: jetStreamSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, err
	}

	return consumer.Consume(obs.JetStreamHandler(ctx, func(ctx context.Context, msg jetstream.Msg) error {
		logger.InfoContext(ctx, "JetStream message received", "subject", msg.Subject(), "job_id", baggage.FromContext(ctx).Member("job.id").Value())
		handler.RecordRequest(ctx)
		handler.Calculate(ctx)
		if err := msg.Ack(); err != nil {
			logger.ErrorContext(ctx, "JetStream acknowledgement failed", "err", err)
			return err
		}
		return nil
	}, observability.WithNATSNewTrace()), jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		logger.Error("JetStream asynchronous error", "err", err)
	}))
}
