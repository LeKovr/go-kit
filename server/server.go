package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/go-http-utils/etag"
	"golang.org/x/sync/errgroup"
)

// TLSConfig holds TLS config options
type TLSConfig struct {
	CertFile           string `long:"cert" description:"CertFile for serving HTTPS instead HTTP" env:"CERT"`
	KeyFile            string `long:"key"  description:"KeyFile for serving HTTPS instead HTTP" env:"KEY"`
	NoCheckCertificate bool   `long:"no-check" description:"disable tls certificate validation"`
}

// VersionResponseConfig
type VersionResponseConfig struct {
	Prefix string `long:"prefix" default:"/js/version.js" description:"URL for version response"`
	Format string `long:"format" default:"document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n" description:"Format string for version response"`
	CType  string `long:"ctype"  default:"text/javascript" description:"js code Content-Type header"`
}

// Config holds all config vars.
type Config struct {
	Listen string `long:"listen" default:":8080" description:"Addr and port which server listens at" env:"LISTEN"`

	MaxHeaderBytes int           `long:"maxheader" description:"MaxHeaderBytes"`
	ReadTimeout    time.Duration `long:"rto" default:"10s" description:"HTTP read timeout"`
	WriteTimeout   time.Duration `long:"wto" default:"60s" description:"HTTP write timeout"`
	GracePeriod    time.Duration `long:"grace" default:"10s" description:"Stop grace period"`

	IPHeader   string `long:"ip_header" env:"IP_HEADER" default:"X-Real-IP" description:"HTTP Request Header for remote IP"`
	UserHeader string `long:"user_header" env:"USER_HEADER" default:"X-Username" description:"HTTP Request Header for username"`

	TLS     TLSConfig             `group:"HTTPS Options"            namespace:"tls"  env-namespace:"TLS"`
	Version VersionResponseConfig `group:"Version response Options" namespace:"vr"`
}

// Handler is a http midleware handler.
type Handler func(http.Handler) http.Handler

// Worker is a server goroutene worker.
type Worker func(ctx context.Context) error

// Service holds service attributes.
type Service struct {
	config     Config
	listener   net.Listener
	mux        *http.ServeMux
	handlers   []Handler
	onShutdown *Worker
}

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
		_, err := fmt.Fprintf(w, srv.config.Version.Format, version)
		if err != nil {
			slog.Error("Verion response", "err", err)
		}
	})
	return srv
}

// Use adds handler for muxer.
func (srv *Service) Use(handler Handler) *Service {
	srv.handlers = append(srv.handlers, handler)
	return srv
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

// RunWorkers runs workers without HTTP servece.
func (srv Service) RunWorkers(ctxParent context.Context, workers ...Worker) error {
	ctx, stop := signal.NotifyContext(ctxParent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := srv.config

	// start servers
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gCtx.Done()
		slog.Debug("Shutdown")
		stop()
		timedCtx, cancel := context.WithTimeout(ctx, cfg.GracePeriod)
		defer cancel()
		var err error
		if srv.onShutdown != nil {
			w := *srv.onShutdown
			err = w(timedCtx)
		}
		return err
	})
	for _, worker := range workers {
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

// Run runs HTTP(s) service and workers.
func (srv Service) Run(ctxParent context.Context, workers ...Worker) error {

	cfg := srv.config

	var mux http.Handler = srv.mux
	for _, handler := range srv.handlers {
		mux = handler(mux)
	}
	listener := srv.listener
	if listener == nil {
		var err error
		slog.Debug("Start Listener", "addr", cfg.Listen)
		if listener, err = net.Listen("tcp", cfg.Listen); err != nil {
			return err
		}
	}
	// Creating a normal HTTP server
	server := &http.Server{
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctxParent
		},
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}
	if cfg.TLS.NoCheckCertificate {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	httpWorkers := make([]Worker, 2+len(workers))
	if srv.config.TLS.CertFile != "" {
		httpWorkers[0] = func(_ context.Context) error {
			slog.Debug("Start HTTPS service")
			return server.ServeTLS(listener, srv.config.TLS.CertFile, srv.config.TLS.KeyFile)
		}
	} else {
		httpWorkers[0] = func(_ context.Context) error {
			slog.Debug("Start HTTP service")
			return server.Serve(listener)
		}
	}
	httpWorkers[1] = func(ctx context.Context) error {
		<-ctx.Done()
		timedCtx, cancel := context.WithTimeout(ctx, cfg.GracePeriod)
		defer cancel()
		return server.Shutdown(timedCtx)
	}

	return srv.RunWorkers(ctxParent, append(httpWorkers, workers...)...)
}

// WithAccessLog calculates estimate and prints HTTP request log.
func (cfg Config) WithAccessLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, w, r)
		ip := r.Header.Get(cfg.IPHeader)
		if ip == "" {
			if colonPos := strings.Index(r.RemoteAddr, ":"); colonPos == -1 {
				ip = r.RemoteAddr
			} else {
				ip = r.RemoteAddr[:colonPos]
			}
		}
		user := r.Header.Get(cfg.UserHeader)
		if user == "" {
			user = "-"
		}
		fmt.Printf(`%s - %s [%s] "%s %s" %d %s %d %s%s`,
			ip,
			user,
			time.Now().Format(time.DateTime),
			r.Method,
			r.URL,
			m.Code,
			m.Duration,
			m.Written,
			r.Header.Get("Referer"),
			"\n",
		)

	})
}

// WithETag adds ETAG to response.
func (cfg Config) WithETag(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag.Handler(handler, false).ServeHTTP(w, r)
	})
}
