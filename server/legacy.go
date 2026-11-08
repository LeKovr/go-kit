package server

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"sync/atomic"

	httpserver "github.com/LeKovr/go-kit/server/http"
)

// Config holds the legacy HTTP service configuration.
//
// Deprecated: use httpserver.Config from github.com/LeKovr/go-kit/server/http.
type Config = httpserver.Config

// TLSConfig holds TLS configuration options.
//
// Deprecated: use httpserver.TLSConfig from github.com/LeKovr/go-kit/server/http.
type TLSConfig = httpserver.TLSConfig

// VersionResponseConfig holds settings for the HTTP version response.
//
// Deprecated: use httpserver.VersionResponseConfig from github.com/LeKovr/go-kit/server/http.
type VersionResponseConfig = httpserver.VersionResponseConfig

// Handler is an HTTP middleware handler.
//
// Deprecated: use httpserver.Handler from github.com/LeKovr/go-kit/server/http.
type Handler = httpserver.Handler

// AccessLogDisabled disables HTTP access logging.
//
// Deprecated: use httpserver.AccessLogDisabled from github.com/LeKovr/go-kit/server/http.
const AccessLogDisabled = httpserver.AccessLogDisabled

// Service adapts the old object API to the worker runner and HTTP transport.
// Configure it before running. HTTP can run only once and owns its listener.
//
// Deprecated: use httpserver.New and the package-level Run function.
type Service struct {
	config         Config
	httpService    *httpserver.Service
	listener       net.Listener
	workers        []Worker
	onShutdown     Worker
	httpRegistered bool
	httpStarted    *atomic.Bool
}

// New returns a legacy service without opening HTTP resources.
//
// Deprecated: use httpserver.New and the package-level Run function.
func New(cfg Config) *Service {
	return &Service{
		config:      cfg,
		httpService: httpserver.New(cfg),
		httpStarted: new(atomic.Bool),
	}
}

// WithListener sets the listener that the HTTP worker will own when run.
//
// Deprecated: use httpserver.Service.WithListener.
func (srv *Service) WithListener(listener net.Listener) *Service {
	srv.listener = listener
	srv.httpService.WithListener(listener)
	return srv
}

// WithStatic registers a static filesystem at the root of the HTTP mux.
//
// Deprecated: use httpserver.Service.WithStatic.
func (srv *Service) WithStatic(fSystem fs.FS) *Service {
	srv.httpService.WithStatic(fSystem)
	return srv
}

// WithVersion registers the configured HTTP version response.
//
// Deprecated: use httpserver.Service.WithVersion.
func (srv *Service) WithVersion(version string) *Service {
	srv.httpService.WithVersion(version)
	return srv
}

// Use adds middleware to the HTTP mux.
//
// Deprecated: use httpserver.Service.Use.
func (srv *Service) Use(handler Handler) *Service {
	srv.httpService.Use(handler)
	return srv
}

// ServeMuxWithHandlers returns the mux wrapped with configured middleware.
//
// Deprecated: use httpserver.Service.ServeMuxWithHandlers.
func (srv Service) ServeMuxWithHandlers() http.Handler {
	return srv.httpService.ServeMuxWithHandlers()
}

// ServeMux returns the HTTP mux.
//
// Deprecated: use httpserver.Service.ServeMux.
func (srv Service) ServeMux() *http.ServeMux {
	return srv.httpService.ServeMux()
}

// WithWorkers appends workers to the service.
//
// Deprecated: pass workers to the package-level Run function.
func (srv *Service) WithWorkers(workers ...Worker) *Service {
	srv.workers = append(srv.workers, workers...)
	return srv
}

// WithNWorkers appends n copies of worker. Non-positive n adds nothing.
//
// Deprecated: pass repeated workers to the package-level Run function.
func (srv *Service) WithNWorkers(n int, worker Worker) *Service {
	for range n {
		srv.workers = append(srv.workers, worker)
	}
	return srv
}

// WithShutdown replaces the callback run on shared-context cancellation.
// Its context retains values and has a fresh GracePeriod timeout. The callback
// runs alongside worker cleanup and must cooperate with its timeout to finish.
//
// Deprecated: perform application cleanup after the package-level Run returns,
// or pass a worker that waits for cancellation when concurrent cleanup is needed.
func (srv *Service) WithShutdown(worker Worker) *Service {
	srv.onShutdown = worker
	return srv
}

// WithHTTPWorkers registers the HTTP worker once, without opening resources.
//
// Deprecated: pass httpserver.Service.Run to the package-level Run function.
func (srv *Service) WithHTTPWorkers() *Service {
	if !srv.httpRegistered {
		srv.httpRegistered = true
		srv.WithWorkers(func(ctx context.Context) error {
			srv.httpStarted.Store(true)
			return srv.httpService.Run(ctx)
		})
	}
	return srv
}

// Run appends workers and runs HTTP plus all registered workers. The listener
// is bound before workers start. Access log and TLS preparation happen inside
// the HTTP worker, so their errors may follow the start of background workers.
//
// Deprecated: use httpserver.New and the package-level Run function.
func (srv *Service) Run(ctx context.Context, workers ...Worker) error {
	srv.WithHTTPWorkers()
	// RunWorkers may already have started HTTP with its own listener. Reject a
	// subsequent Run before binding a listener the used HTTP service cannot own.
	if srv.httpStarted.Load() {
		return errors.New("HTTP service already run")
	}
	if srv.listener == nil {
		listener, err := net.Listen("tcp", srv.config.Listen)
		if err != nil {
			return err
		}
		srv.WithListener(listener)
	}
	return srv.WithWorkers(workers...).run(ctx)
}

// RunWorkers appends workers without adding HTTP. The legacy service waits for
// cancellation even when all finite workers succeed or no workers are given.
//
// Deprecated: use the package-level Run function, adding a worker that waits
// for cancellation if the service must stay alive after finite workers finish.
func (srv *Service) RunWorkers(ctx context.Context, workers ...Worker) error {
	return srv.WithWorkers(workers...).run(ctx)
}

func (srv *Service) run(ctx context.Context) error {
	// This ordinary worker preserves the old wait-for-cancellation behavior.
	shutdown := func(ctx context.Context) error {
		<-ctx.Done()
		if srv.onShutdown != nil {
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), srv.config.GracePeriod)
			defer cancel()
			return srv.onShutdown(shutdownCtx)
		}
		return nil
	}
	workers := make([]Worker, 0, len(srv.workers)+1)
	workers = append(workers, shutdown)
	workers = append(workers, srv.workers...)
	for i, worker := range workers {
		workers[i] = func(ctx context.Context) error {
			err := worker(ctx)
			if ctx.Err() != nil {
				// The old API accepts cooperative cancellation, including
				// ordinary wrappers. Unwrap stops at joined errors so another
				// cause cannot be discarded. Adapt before errgroup records it.
				for cause := err; cause != nil; cause = errors.Unwrap(cause) {
					if cause == context.Canceled {
						return nil
					}
				}
			}
			return err
		}
	}
	return Run(ctx, workers...)
}
