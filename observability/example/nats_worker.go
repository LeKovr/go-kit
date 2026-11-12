package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/slogger"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/metric"
)

const natsSubject = "demo.calculate"

// NATSConfig holds connection and request settings for the demo.
type NATSConfig struct {
	GracePeriod time.Duration `default:"10s" description:"NATS stop grace period" env:"GRACE" long:"grace"`
	URL         string        `default:"nats://127.0.0.1:4222" description:"NATS server URL"                  env:"URL"     long:"url"`
	Timeout     time.Duration `default:"3s"                    description:"NATS connection and request timeout" env:"TIMEOUT" long:"timeout"`
}

// DBConfig holds PostgreSQL connection and query settings for the demo.
type DBConfig struct {
	DSN     string        `description:"PostgreSQL connection string" env:"DSN" long:"dsn"`
	Timeout time.Duration `default:"3s" description:"PostgreSQL connection and query timeout" env:"TIMEOUT" long:"timeout"`
}

func runNATSWorker(ctx context.Context, cfg Config, obs *observability.Service) error {
	if cfg.NATS.GracePeriod < 0 {
		return errors.New("NATS grace period must not be negative")
	}
	if ctx.Err() != nil {
		return nil
	}
	logger := slogger.WithScope(slog.Default(), application+"/nats-worker")
	requests, err := obs.Meter(application+"/nats-worker").Int64Counter("demo.custom.requests",
		metric.WithDescription("Number of custom demo handler calls."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DB.DSN)
	if err != nil {
		return err
	}
	poolCfg.ConnConfig.Tracer = obs.PGXTracer()
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := obs.RecordPGXPoolStats(pool,
		otelpgx.WithStatsAttributes(attribute.String("db.client.connection.pool.name", "primary")),
	); err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, cfg.DB.Timeout)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		return err
	}

	return runNATSSubscription(ctx, cfg.NATS, func(callbackCtx context.Context, conn *nats.Conn) nats.MsgHandler {
		return obs.NATSHandler(callbackCtx, natsWorkerHandler(obs.NATSClient(conn), pool, cfg.DB.Timeout, logger, requests))
	})
}

// runNATSSubscription owns callbacks until they finish, before the caller closes its pool.
func runNATSSubscription(ctx context.Context, cfg NATSConfig, handler func(context.Context, *nats.Conn) nats.MsgHandler) error {
	logger := slogger.WithScope(slog.Default(), application+"/nats-worker")
	if cfg.GracePeriod < 0 {
		return errors.New("NATS grace period must not be negative")
	}
	if ctx.Err() != nil {
		return nil
	}
	callbackCtx, cancelCallbacks := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelCallbacks()
	closed := make(chan struct{})
	drainErrors := make(chan error, 1)
	var draining atomic.Bool
	conn, err := nats.Connect(cfg.URL,
		nats.Name(application+"-nats-worker"), nats.Timeout(cfg.Timeout),
		nats.DrainTimeout(cfg.GracePeriod),
		nats.ClosedHandler(func(*nats.Conn) { close(closed) }),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS asynchronous error", "err", err)
			if draining.Load() {
				if errors.Is(err, nats.ErrDrainTimeout) {
					err = errors.Join(err, context.DeadlineExceeded)
				}
				select {
				case drainErrors <- err:
				default:
				}
			}
		}),
	)
	if err != nil {
		return err
	}
	var callbacks sync.WaitGroup
	var mu sync.Mutex
	stopping := false
	defer func() {
		// Exclude new callbacks before Wait, including ones queued by the client.
		mu.Lock()
		stopping = true
		mu.Unlock()
		cancelCallbacks()
		conn.Close()
		callbacks.Wait()
	}()
	handle := handler(callbackCtx, conn)
	_, err = conn.QueueSubscribe(natsSubject, "demo-workers", func(msg *nats.Msg) {
		mu.Lock()
		if stopping {
			mu.Unlock()
			return
		}
		callbacks.Add(1)
		mu.Unlock()
		defer callbacks.Done()
		handle(msg)
	})
	if err != nil {
		return err
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		return err
	}
	logger.Info("NATS worker ready", "subject", natsSubject)
	select {
	case <-ctx.Done():
	case <-closed:
		return errors.New("NATS connection closed")
	}
	if cfg.GracePeriod == 0 {
		return nil
	}
	draining.Store(true)
	timer := time.NewTimer(cfg.GracePeriod)
	defer timer.Stop()
	if err := conn.Drain(); err != nil {
		return fmt.Errorf("NATS drain: %w", err)
	}
	select {
	case <-closed:
		select {
		case err := <-drainErrors:
			return fmt.Errorf("NATS drain: %w", err)
		default:
			return nil
		}
	case <-timer.C:
		return fmt.Errorf("NATS drain: %w", context.DeadlineExceeded)
	}
}

func natsWorkerHandler(client *observability.NATSClient, pool *pgxpool.Pool, timeout time.Duration, logger *slog.Logger, requests metric.Int64Counter) func(context.Context, *nats.Msg) error {
	return func(ctx context.Context, msg *nats.Msg) error {
		logger.InfoContext(ctx, "NATS message received", "subject", msg.Subject, "job_id", baggage.FromContext(ctx).Member("job.id").Value())
		requests.Add(ctx, 1, metric.WithAttributes(attribute.String("demo.operation", "request")))
		if msg.Reply == "" {
			return nil
		}
		queryCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		var result int
		if err := pool.QueryRow(queryCtx, "SELECT $1::int + $2::int", 20, 22).Scan(&result); err != nil {
			logger.ErrorContext(ctx, "PostgreSQL calculation failed", "err", err)
			return err
		}
		err := client.PublishMsg(ctx, &nats.Msg{Subject: msg.Reply, Data: []byte(strconv.Itoa(result) + "\n")})
		if err != nil {
			logger.ErrorContext(ctx, "NATS reply failed", "err", err)
		}
		return err
	}
}
