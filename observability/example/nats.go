package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/baggage"
)

const natsSubject = "demo.calculate"

// NATSConfig holds connection and request settings for the demo.
type NATSConfig struct {
	URL     string        `default:"nats://127.0.0.1:4222" description:"NATS server URL"                  env:"URL"     long:"url"`
	Timeout time.Duration `default:"3s"                    description:"NATS connection and request timeout" env:"TIMEOUT" long:"timeout"`
}

func natsHTTPHandler(request func(context.Context, *nats.Msg) (*nats.Msg, error), timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		msg, err := request(ctx, &nats.Msg{Subject: natsSubject, Data: []byte("calculate")})
		if err != nil {
			writeNATSError(w, ctx, logger, err)
			return
		}

		_, _ = w.Write(msg.Data)
	}
}

func natsPublishHTTPHandler(publish func(context.Context, *nats.Msg) error, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		for _, payload := range []string{"calculate1", "calculate2", "calculate3"} {
			if err := publish(ctx, &nats.Msg{Subject: natsSubject, Data: []byte(payload)}); err != nil {
				writeNATSError(w, ctx, logger, err)
				return
			}
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted\n"))
	}
}

func writeNATSError(w http.ResponseWriter, ctx context.Context, logger *slog.Logger, err error) {
	logger.ErrorContext(ctx, "NATS operation failed", "err", err)
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, nats.ErrNoResponders), errors.Is(err, nats.ErrConnectionClosed):
		status = http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, http.StatusText(status), status)
}

func runNATSWorker(ctx context.Context, cfg Config, obs *observability.Service) error {
	telemetry := newTelemetry(slog.Default(), obs, application+"/worker")
	handler, err := NewDemoHandler(telemetry)
	if err != nil {
		return err
	}

	closed := make(chan struct{})
	conn, err := nats.Connect(cfg.NATS.URL,
		nats.Name(application+"-worker"), nats.Timeout(cfg.NATS.Timeout),
		nats.DrainTimeout(cfg.Server.GracePeriod),
		nats.ClosedHandler(func(*nats.Conn) { close(closed) }),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			telemetry.Logger.Error("NATS asynchronous error", "err", err)
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := obs.NATSClient(conn)
	_, err = conn.QueueSubscribe(natsSubject, "demo-workers", obs.NATSHandler(ctx, func(ctx context.Context, msg *nats.Msg) error {
		telemetry.Logger.InfoContext(ctx, "NATS message received", "subject", msg.Subject, "job_id", baggage.FromContext(ctx).Member("job.id").Value())
		handler.RecordRequest(ctx)
		handler.Calculate(ctx)
		if msg.Reply == "" {
			return nil
		}
		err := client.PublishMsg(ctx, &nats.Msg{Subject: msg.Reply, Data: []byte("ok\n")})
		if err != nil {
			telemetry.Logger.ErrorContext(ctx, "NATS reply failed", "err", err)
		}
		return err
	}))
	if err != nil {
		return err
	}
	consume, err := startJetStreamWorker(ctx, conn, cfg.NATS.Timeout, obs, handler, telemetry.Logger)
	if err != nil {
		return err
	}
	defer consume.Stop()

	if err := conn.FlushTimeout(cfg.NATS.Timeout); err != nil {
		return err
	}
	telemetry.Logger.Info("NATS worker ready", "subject", natsSubject)

	return server.New(cfg.Server).WithShutdown(func(ctx context.Context) error {
		consume.Drain()
		select {
		case <-consume.Closed():
		case <-ctx.Done():
			return ctx.Err()
		}
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
