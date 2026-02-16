package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/go-http-utils/etag"
	"golang.org/x/sync/errgroup"
)

// TLSConfig holds TLS config options.
type TLSConfig struct {
	CertFile           string `long:"cert" description:"CertFile for serving HTTPS instead HTTP" env:"CERT"`
	KeyFile            string `long:"key"  description:"KeyFile for serving HTTPS instead HTTP" env:"KEY"`
	NoCheckCertificate bool   `long:"no-check" description:"disable tls certificate validation"`
}

// VersionResponseConfig holds settings for HTTP version response.
type VersionResponseConfig struct {
	Prefix string `long:"prefix" default:"/js/version.js" description:"URL for version response"`
	Format string `long:"format" default:"document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n" description:"Format string for version response"`
	CType  string `long:"ctype"  default:"text/javascript" description:"js code Content-Type header"`
}

// Config holds all config vars.
type Config struct {
	Listen string `long:"listen" default:":8080" description:"Addr and port which server listens at" env:"LISTEN"`

	MaxHeaderBytes    int           `long:"maxheader" description:"MaxHeaderBytes"`
	ReadTimeout       time.Duration `long:"rto" default:"10s" description:"HTTP read timeout"`
	WriteTimeout      time.Duration `long:"wto" default:"60s" description:"HTTP write timeout, '0' means disable"`
	ReadHeaderTimeout time.Duration `long:"rhto" default:"10s" description:"HTTP read header timeout"`
	IdleTimeout       time.Duration `long:"ito" default:"10s" description:"HTTP idle timeout"`
	GracePeriod       time.Duration `long:"grace" default:"10s" description:"Stop grace period"`

	IPHeader   string `long:"ip_header" env:"IP_HEADER" default:"X-Real-IP" description:"HTTP Request Header for remote IP"`
	UserHeader string `long:"user_header" env:"USER_HEADER" default:"X-Username" description:"HTTP Request Header for username"`
	AccessLog  string `long:"access_log" env:"ACCESS_LOG" description:"HTTP access log filename (default: STDOUT, '-' means disable)"`
	UseETag    bool   `long:"etag" env:"ETAG" description:"Add ETAG in HTTP response"`

	TLS     TLSConfig             `group:"HTTPS Options"            namespace:"tls"  env-namespace:"TLS"`
	Version VersionResponseConfig `group:"Version response Options" namespace:"vr"`
}

// Handler is a http midleware handler.
type Handler func(http.Handler) http.Handler

// Worker is a server goroutene worker.
type Worker func(ctx context.Context) error

// Service holds service attributes.
type Service struct {
	config          Config
	listener        net.Listener
	server          *http.Server
	mux             *http.ServeMux
	handlers        []Handler
	workers         []Worker
	onShutdown      *Worker
	accessLogWriter io.Writer
}

// AccessLogDisabled holds access_log value for access logging disabling.
const AccessLogDisabled = "-"

// New returns *Service.
func New(cfg Config) *Service {
	return &Service{
		config: cfg,
		mux:    http.NewServeMux(),
	}
}

// WithListener sets service listener.
func (srv *Service) WithListener(listener net.Listener) *Service {
	srv.listener = listener
	return srv
}

// WithStatic sets static filesystem for serve via http.
func (srv *Service) WithStatic(fSystem fs.FS) *Service {
	httpFileServer := http.FileServer(http.FS(fSystem))
	srv.mux.Handle("/", httpFileServer)
	return srv
}

// WithVersion sets hanler returning source code version as js.
func (srv *Service) WithVersion(version string) *Service {
	srv.mux.HandleFunc(srv.config.Version.Prefix, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", srv.config.Version.CType)
		if _, err := fmt.Fprintf(w, srv.config.Version.Format, version); err != nil {
			slog.Error("Version response", "err", err)
		}
	})
	return srv
}

// Use adds handler for muxer.
func (srv *Service) Use(handler Handler) *Service {
	srv.handlers = append(srv.handlers, handler)
	return srv
}

// ServeMuxWithHandlers return mux joined with defined by Use handlers.
func (srv Service) ServeMuxWithHandlers() http.Handler {
	var mux http.Handler = srv.mux
	for _, handler := range srv.handlers {
		mux = handler(mux)
	}
	if srv.config.UseETag {
		mux = etag.Handler(mux, false)
	}
	return mux
}

// ServeMux returns service muxer.
func (srv Service) ServeMux() *http.ServeMux {
	return srv.mux
}

// WithShutdown registers worker for call on shutdown.
func (srv *Service) WithShutdown(worker Worker) *Service {
	srv.onShutdown = &worker
	return srv
}

// WithWorkers registers workers.
func (srv *Service) WithWorkers(workers ...Worker) *Service {
	srv.workers = append(srv.workers, workers...)
	return srv
}

// WithNWorkers registers N copies of worker.
func (srv *Service) WithNWorkers(n int, worker Worker) *Service {
	for range n {
		srv.workers = append(srv.workers, worker)
	}
	return srv
}

// WithHTTPWorkers registers HTTP workers.
func (srv *Service) WithHTTPWorkers() *Service {

	cfg := srv.config
	//mux := srv.ServeMuxWithHandlers()

	// Creating a normal HTTP server
	server := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}
	if cfg.WriteTimeout != 0 {
		server.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.TLS.NoCheckCertificate {
		server.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	workers := make([]Worker, 2)
	if srv.config.TLS.CertFile != "" {
		workers[0] = func(_ context.Context) error {
			slog.Debug("Start HTTPS service")
			return server.ServeTLS(srv.listener, srv.config.TLS.CertFile, srv.config.TLS.KeyFile)
		}
	} else {
		workers[0] = func(_ context.Context) error {
			slog.Debug("Start HTTP service")
			return server.Serve(srv.listener)
		}
	}
	workers[1] = func(ctx context.Context) error {
		<-ctx.Done()
		timedCtx, cancel := context.WithTimeout(ctx, cfg.GracePeriod)
		defer cancel()
		return server.Shutdown(timedCtx)
	}
	srv.server = server
	srv.WithWorkers(workers...)
	return srv
}

// Run runs HTTP(s) service and workers. HTTP Workers will be registered if none.
func (srv *Service) Run(ctx context.Context, workers ...Worker) error {
	cfg := srv.config
	if srv.listener == nil {
		slog.Debug("Start Listener", "addr", cfg.Listen)
		listener, err := net.Listen("tcp", cfg.Listen)
		if err != nil {
			return err
		}
		srv.listener = listener
	}
	if srv.server == nil {
		srv.WithHTTPWorkers()
	}
	server := srv.server
	server.Handler = srv.ServeMuxWithHandlers() // Use aclual handlers list.
	server.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	if cfg.AccessLog != "" && cfg.AccessLog != AccessLogDisabled {
		writer, err := os.OpenFile(cfg.AccessLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return err
		}
		srv.accessLogWriter = writer
		defer writer.Close()
		server.Handler = srv.accessLogHandler(server.Handler)
	}

	return srv.WithWorkers(workers...).run(ctx)
}

// RunWorkers runs workers without HTTP service.
func (srv *Service) RunWorkers(ctx context.Context, workers ...Worker) error {
	return srv.WithWorkers(workers...).run(ctx)
}

func (srv *Service) run(ctxParent context.Context) error {
	ctx, stop := signal.NotifyContext(ctxParent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// start servers
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return srv.shutdownWorker(gCtx)
	})
	for _, worker := range srv.workers {
		w := worker
		g.Go(func() error {
			return w(gCtx)
		})
	}
	if er := g.Wait(); er != nil && !errors.Is(er, http.ErrServerClosed) && !errors.Is(er, net.ErrClosed) {
		return er
	}
	slog.Info("Exit")
	return nil
}

func (srv *Service) shutdownWorker(ctx context.Context) error {
	<-ctx.Done()
	slog.Debug("Shutdown")
	timedCtx, cancel := context.WithTimeout(context.Background(), srv.config.GracePeriod)
	defer cancel()
	var err error
	if srv.onShutdown != nil {
		w := *srv.onShutdown
		err = w(timedCtx)
	}
	return err
}

// WithAccessLog calculates estimate and prints HTTP request log.
func (srv Service) accessLogHandler(handler http.Handler) http.Handler {
	var writer io.Writer = os.Stdout
	if srv.accessLogWriter != nil {
		writer = srv.accessLogWriter
	}
	cfg := srv.config
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, w, r)
		ip := r.Header.Get(cfg.IPHeader)
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		user := r.Header.Get(cfg.UserHeader)
		if user == "" {
			user = "-"
		}
		fmt.Fprintf(writer, `%s - %s [%s] "%s %s" %d %s %d %s%s`,
			ip,
			user,
			time.Now().Format(time.DateTime),
			r.Method,
			r.URL.RequestURI(),
			m.Code,
			m.Duration,
			m.Written,
			r.Header.Get("Referer"),
			"\n",
		)
	})
}
