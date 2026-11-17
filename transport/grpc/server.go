// Package grpcserver owns the lifecycle of a gRPC server.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
)

// Config holds listener and shutdown settings; defaults are applied by config.Open.
type Config struct {
	Listen      string        `long:"listen" env:"LISTEN" default:":9090" description:"Addr and port which gRPC server listens at"`
	GracePeriod time.Duration `long:"grace" env:"GRACE" default:"10s" description:"gRPC stop grace period"`
}

// Service owns a gRPC server and its listener. Configure and register before Run.
type Service struct {
	config   Config
	server   *grpc.Server
	listener net.Listener
	started  atomic.Bool
}

// New creates a server without opening resources. Native options configure TLS,
// interceptors, stats handlers and other gRPC behavior.
func New(cfg Config, opts ...grpc.ServerOption) *Service {
	return &Service{config: cfg, server: grpc.NewServer(opts...)}
}

// Server exposes the native server for service registration before Run.
func (srv *Service) Server() *grpc.Server { return srv.server }

// WithListener supplies a listener whose ownership transfers on entry to Run.
func (srv *Service) WithListener(listener net.Listener) *Service {
	srv.listener = listener
	return srv
}

// Run serves once and drains active RPCs when ctx is canceled. A zero grace
// period forces an immediate stop; a negative grace period is invalid.
func (srv *Service) Run(ctx context.Context) (err error) {
	if !srv.started.CompareAndSwap(false, true) {
		return errors.New("gRPC service already run")
	}
	var listener *ownedListener
	if srv.listener != nil {
		listener = &ownedListener{Listener: srv.listener}
	}
	defer func() {
		srv.server.Stop()
		if listener != nil {
			err = errors.Join(err, listener.Close())
		}
	}()
	if srv.config.GracePeriod < 0 {
		return errors.New("gRPC grace period must not be negative")
	}
	if ctx.Err() != nil {
		return nil
	}
	if listener == nil {
		ln, listenErr := net.Listen("tcp", srv.config.Listen)
		if listenErr != nil {
			return fmt.Errorf("gRPC listen: %w", listenErr)
		}
		listener = &ownedListener{Listener: ln}
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
	}
	stopping.Store(true)
	if srv.config.GracePeriod == 0 {
		srv.server.Stop()
	} else {
		stopped := make(chan struct{})
		go func() { srv.server.GracefulStop(); close(stopped) }()
		timer := time.NewTimer(srv.config.GracePeriod)
		defer timer.Stop()
		select {
		case <-stopped:
		case <-timer.C:
			err = fmt.Errorf("gRPC shutdown: %w", context.DeadlineExceeded)
			srv.server.Stop()
			<-stopped
		}
	}
	return errors.Join(err, <-served)
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
