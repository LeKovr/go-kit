package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/slogger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/metric"
)

const natsSubject = "demo.calculate"

// NATSConfig holds connection and request settings for the demo.
type NATSConfig struct {
	URL     string        `default:"nats://127.0.0.1:4222" description:"NATS server URL"                  env:"URL"     long:"url"`
	Timeout time.Duration `default:"3s"                    description:"NATS connection and request timeout" env:"TIMEOUT" long:"timeout"`
}

func runNATSWorker(ctx context.Context, cfg Config, obs *observability.Service) error {
	logger := slogger.WithScope(slog.Default(), application+"/nats-worker")
	requests, err := obs.Meter(application+"/nats-worker").Int64Counter("demo.custom.requests",
		metric.WithDescription("Number of custom demo handler calls."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	closed := make(chan struct{})
	conn, err := nats.Connect(cfg.NATS.URL,
		nats.Name(application+"-nats-worker"), nats.Timeout(cfg.NATS.Timeout),
		nats.DrainTimeout(cfg.Server.GracePeriod),
		nats.ClosedHandler(func(*nats.Conn) { close(closed) }),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS asynchronous error", "err", err)
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := obs.NATSClient(conn)
	_, err = conn.QueueSubscribe(natsSubject, "demo-workers", obs.NATSHandler(ctx, natsWorkerHandler(client, logger, requests)))
	if err != nil {
		return err
	}

	if err := conn.FlushTimeout(cfg.NATS.Timeout); err != nil {
		return err
	}
	logger.Info("NATS worker ready", "subject", natsSubject)

	return server.New(cfg.Server).WithShutdown(func(ctx context.Context) error {
		if err := conn.Drain(); err != nil {
			return err
		}
		select {
		case <-closed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}).RunWorkers(ctx, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case <-closed:
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("NATS connection closed")
		}
	})
}

func natsWorkerHandler(client *observability.NATSClient, logger *slog.Logger, requests metric.Int64Counter) func(context.Context, *nats.Msg) error {
	return func(ctx context.Context, msg *nats.Msg) error {
		logger.InfoContext(ctx, "NATS message received", "subject", msg.Subject, "job_id", baggage.FromContext(ctx).Member("job.id").Value())
		requests.Add(ctx, 1, metric.WithAttributes(attribute.String("demo.operation", "request")))
		if msg.Reply == "" {
			return nil
		}
		err := client.PublishMsg(ctx, &nats.Msg{Subject: msg.Reply, Data: []byte("ok\n")})
		if err != nil {
			logger.ErrorContext(ctx, "NATS reply failed", "err", err)
		}
		return err
	}
}
