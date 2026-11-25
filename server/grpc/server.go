// Package grpcserver owns the lifecycle of a gRPC server.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
)

// Config holds listener settings; defaults are applied by config.Open.
type Config struct {
	Listen string `long:"listen" env:"LISTEN" default:":9090" description:"Addr and port which gRPC server listens at"`
}

// Service owns a gRPC server and its listener. Configure and register before Run.
type Service struct {
	config   Config
	server   *grpc.Server
	listener net.Listener

	mu       sync.Mutex
	runCtx   context.Context // Set once, by the first Run.
	stopping bool
	drained  chan struct{}
	done     chan struct{}
	stopErr  error
}

// New creates a server without opening resources. Native options configure TLS,
// interceptors, stats handlers and other gRPC behavior.
func New(cfg Config, opts ...grpc.ServerOption) *Service {
	return &Service{config: cfg, server: grpc.NewServer(opts...), done: make(chan struct{})}
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
	srv.mu.Lock()
	if srv.runCtx != nil {
		srv.mu.Unlock()
		return errors.New("gRPC service already run")
	}

	srv.runCtx = ctx
	stopping := srv.stopping
	srv.mu.Unlock()

	if stopping {
		<-srv.done
		return srv.stopErr
	}

	var listener *ownedListener
	if srv.listener != nil {
		listener = &ownedListener{Listener: srv.listener}
	}

	// Keep cancellation active while cleanup waits for an in-flight Shutdown.
	stop := context.AfterFunc(ctx, srv.server.Stop)
	defer stop()

	defer func() {
		srv.mu.Lock()
		drained := srv.drained
		srv.stopping = true
		srv.mu.Unlock()

		if drained != nil {
			<-drained
		} else {
			srv.server.Stop()
		}

		if errors.Is(err, grpc.ErrServerStopped) && (drained != nil || ctx.Err() != nil) {
			err = nil // Shutdown or cancellation won the race with Serve.
		}

		if listener != nil {
			srv.stopErr = errors.Join(srv.stopErr, listener.Close())
		}

		err = errors.Join(err, srv.stopErr)
		close(srv.done)
	}()

	if ctx.Err() != nil {
		return nil
	}

	if listener == nil {
		ln, err := net.Listen("tcp", srv.config.Listen)
		if err != nil {
			return fmt.Errorf("gRPC listen: %w", err)
		}

		listener = &ownedListener{Listener: ln}
	}

	if err := srv.server.Serve(listener); err != nil {
		return fmt.Errorf("gRPC serve: %w", err)
	}

	return nil
}

// Shutdown stops admission and drains active RPCs within ctx. Before Run it
// closes a supplied listener and prevents serving.
// Later calls only wait for cleanup within their own context.
func (srv *Service) Shutdown(ctx context.Context) error {
	srv.mu.Lock()
	if !srv.stopping {
		srv.stopping = true

		if srv.runCtx == nil {
			srv.mu.Unlock()
			srv.server.Stop()

			if srv.listener != nil {
				srv.stopErr = srv.listener.Close()
			}

			close(srv.done)
			return srv.stopErr
		}

		if srv.runCtx.Err() == nil {
			srv.drained = make(chan struct{})

			go func() {
				defer close(srv.drained)
				srv.stopErr = srv.drain(ctx)
			}()
		}
	}

	srv.mu.Unlock()

	select {
	case <-srv.done:
		return srv.stopErr
	default:
	}

	select {
	case <-srv.done:
		return srv.stopErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drain lets native gRPC finish active RPCs until either context is canceled.
func (srv *Service) drain(ctx context.Context) error {
	if ctx.Err() != nil {
		srv.server.Stop()
	} else {
		force := context.AfterFunc(ctx, srv.server.Stop)
		srv.server.GracefulStop()
		force()
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("gRPC shutdown: %w", err)
	}

	if srv.runCtx.Err() != nil {
		return fmt.Errorf("gRPC shutdown: %w", context.Canceled)
	}

	return nil
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
