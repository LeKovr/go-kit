// Package grpcserver owns the lifecycle of a gRPC server.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
)

// Config holds listener settings; defaults are applied by config.Open.
type Config struct {
	Listen string `long:"listen" env:"LISTEN" default:":9090" description:"Addr and port which gRPC server listens at"`
}

// Service owns a gRPC server and its listener. Configure and register before Run.
type Service struct {
	config        Config
	server        *grpc.Server
	listener      net.Listener
	started       atomic.Bool
	runCalled     atomic.Bool
	stopOnce      sync.Once
	stopRequested chan struct{}
	stopCtx       context.Context
	stopCancel    context.CancelFunc
	stopImmediate bool
	done          chan struct{}
	stopErr       error
}

// New creates a server without opening resources. Native options configure TLS,
// interceptors, stats handlers and other gRPC behavior.
func New(cfg Config, opts ...grpc.ServerOption) *Service {
	return &Service{config: cfg, server: grpc.NewServer(opts...), stopRequested: make(chan struct{}), done: make(chan struct{})}
}

// Server exposes the native server for service registration before Run.
func (srv *Service) Server() *grpc.Server { return srv.server }

// WithListener supplies a listener owned by the first Run or Shutdown call.
func (srv *Service) WithListener(listener net.Listener) *Service {
	srv.listener = listener
	return srv
}

// Run serves once. Context cancellation forces a stop; use Shutdown to drain RPCs.
func (srv *Service) Run(ctx context.Context) (err error) {
	if !srv.runCalled.CompareAndSwap(false, true) {
		return errors.New("gRPC service already run")
	}

	if !srv.started.CompareAndSwap(false, true) {
		<-srv.done
		return srv.stopErr
	}

	var listener *ownedListener

	if srv.listener != nil {
		listener = &ownedListener{Listener: srv.listener}
	}
	defer func() {
		srv.server.Stop()

		if listener != nil {
			srv.stopErr = errors.Join(srv.stopErr, listener.Close())
		}

		srv.beginShutdown(context.Background(), true)
		srv.stopCancel()
		err = errors.Join(err, srv.stopErr)
		close(srv.done)
	}()

	if ctx.Err() != nil {
		return nil
	}

	select {
	case <-srv.stopRequested:
		return nil
	default:
	}

	if listener == nil {
		ln, listenErr := net.Listen("tcp", srv.config.Listen)
		if listenErr != nil {
			return fmt.Errorf("gRPC listen: %w", listenErr)
		}

		listener = &ownedListener{Listener: ln}
	}

	select {
	case <-srv.stopRequested:
		return nil
	default:
	}

	var stopping atomic.Bool

	served := make(chan error, 1)
	go func() {
		serveErr := srv.server.Serve(listener)
		if stopping.Load() && (errors.Is(serveErr, grpc.ErrServerStopped) || errors.Is(serveErr, net.ErrClosed)) {
			serveErr = nil
		}
		if serveErr != nil {
			serveErr = fmt.Errorf("gRPC serve: %w", serveErr)
		}
		served <- serveErr
	}()
	select {
	case serveErr := <-served:
		return serveErr
	case <-ctx.Done():
	case <-srv.stopRequested:
	}

	stopping.Store(true)
	srv.beginShutdown(context.Background(), true)
	// Run cancellation also interrupts an explicit Shutdown already in progress.
	stopCancel := context.AfterFunc(ctx, srv.stopCancel)
	defer stopCancel()

	srv.stopErr = srv.shutdown()

	return <-served
}

func (srv *Service) shutdown() error {
	if srv.stopImmediate {
		srv.server.Stop()
		return nil
	}

	if err := srv.stopCtx.Err(); err != nil {
		srv.server.Stop()
		return fmt.Errorf("gRPC shutdown: %w", err)
	}

	stopped := make(chan struct{})

	go func() { srv.server.GracefulStop(); close(stopped) }()

	select {
	case <-stopped:
		return nil
	case <-srv.stopCtx.Done():
		srv.server.Stop()
		// GracefulStop may still be waiting for handlers that ignore cancellation.
		<-stopped

		return fmt.Errorf("gRPC shutdown: %w", srv.stopCtx.Err())
	}
}

// Shutdown stops admission and drains active work using ctx, without a new timeout.
// It is safe before Run, during startup and on repeated calls. A call before Run
// closes a supplied listener and prevents Run from starting. An expired context
// requests forced closure; Run still owns and waits for resource cleanup.
func (srv *Service) Shutdown(ctx context.Context) error {
	select {
	case <-srv.done:
		return srv.stopErr
	default:
	}

	srv.beginShutdown(ctx, false)

	if srv.started.CompareAndSwap(false, true) {
		srv.server.Stop()

		if srv.listener != nil {
			srv.stopErr = srv.listener.Close()
		}

		srv.stopCancel()
		close(srv.done)
	}

	select {
	case <-srv.done:
		return srv.stopErr
	default:
	}

	select {
	case <-srv.done:
		return srv.stopErr
	case <-ctx.Done():
		srv.stopCancel()
		return ctx.Err()
	}
}

func (srv *Service) beginShutdown(ctx context.Context, immediate bool) {
	srv.stopOnce.Do(func() {
		srv.stopCtx, srv.stopCancel = context.WithCancel(ctx)
		srv.stopImmediate = immediate
		close(srv.stopRequested)
	})
}

// ownedListener preserves the first Close error without closing twice.
type ownedListener struct {
	net.Listener
	once sync.Once
	err  error
}

func (ln *ownedListener) Close() error {
	ln.once.Do(func() { ln.err = ln.Listener.Close() })
	return ln.err
}
