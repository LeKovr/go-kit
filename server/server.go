package server

import (
	"context"
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

// Config holds all config vars.
type Config struct {
	Listen string `long:"listen" default:":8080" description:"Addr and port which server listens at"`

	MaxHeaderBytes int           `long:"maxheader" description:"MaxHeaderBytes"`
	ReadTimeout    time.Duration `long:"rto" default:"10s" description:"HTTP read timeout"`
	WriteTimeout   time.Duration `long:"wto" default:"60s" description:"HTTP write timeout"`
	GracePeriod    time.Duration `long:"grace" default:"10s" description:"Stop grace period"`

	IPHeader   string `long:"ip_header" env:"IP_HEADER" default:"X-Real-IP" description:"HTTP Request Header for remote IP"`
	UserHeader string `long:"user_header" env:"USER_HEADER" default:"X-Username" description:"HTTP Request Header for username"`

	VersionPrefix string `long:"ver_prefix" default:"/js/version.js" description:"URL for version response"`
	VersionFormat string `long:"ver_format" default:"document.addEventListener('DOMContentLoaded', () => { appVersion.innerHTML = '%s'; });\n" description:"Format string for version response"`
}

type Handler func(http.Handler) http.Handler

type Worker func(ctx context.Context) error

type Service struct {
	config     Config
	listener   net.Listener
	mux        *http.ServeMux
	handlers   []Handler
	onShutdown *Worker
}

func New(cfg Config) *Service {
	return &Service{
		config: cfg,
		mux:    http.NewServeMux(),
	}
}

func (srv *Service) WithListener(listener net.Listener) *Service {
	srv.listener = listener
	return srv
}

func (srv *Service) WithStatic(fSystem fs.FS) *Service {
	httpFileServer := http.FileServer(http.FS(fSystem))
	srv.mux.Handle("/", httpFileServer)
	return srv
}

func (srv *Service) WithVersion(version string) *Service {
	srv.mux.HandleFunc(srv.config.VersionPrefix, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, err := fmt.Fprintf(w, srv.config.VersionFormat, version)
		if err != nil {
			slog.Error("Verion response", "err", err)
		}
	})
	return srv
}

func (srv *Service) Use(handler Handler) *Service {
	srv.handlers = append(srv.handlers, handler)
	return srv
}

func (srv Service) ServeMux() *http.ServeMux {
	return srv.mux
}

func (srv *Service) WithShutdown(worker Worker) *Service {
	srv.onShutdown = &worker
	return srv
}

func (srv Service) Run(ctxParent context.Context, workers ...Worker) error {

	ctx, stop := signal.NotifyContext(ctxParent, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg := srv.config

	var mux http.Handler = srv.mux
	for _, handler := range srv.handlers {
		mux = handler(mux)
	}
	listener := srv.listener
	if listener == nil {
		var err error
		if listener, err = net.Listen("tcp", cfg.Listen); err != nil {
			return err
		}
	}
	// Creating a normal HTTP server
	server := &http.Server{
		//		Addr:    cfg.Listen,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	// start servers
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		slog.Debug("Start HTTP service", "addr", cfg.Listen)
		return server.Serve(listener)
	})
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
		return errors.Join(err, server.Shutdown(timedCtx))
	})
	for _, worker := range workers {
		w := worker
		g.Go(func() error {
			return w(gCtx)
		})
	}
	if er := g.Wait(); er != nil && !errors.Is(er, http.ErrServerClosed) {
		return er
	}
	slog.Info("Exit")
	return nil
}

// func(http.Handler) *http.ServeMux
// withReqLogger prints HTTP request log.
func (cfg Config) WithAccessLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, w, r)
		ip := r.Header.Get(cfg.IPHeader)
		if ip == "" {
			ip = r.RemoteAddr[:strings.Index(r.RemoteAddr, ":")]
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

// WithETag addr ETAG to response.
func (cfg Config) WithETag(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag.Handler(handler, false).ServeHTTP(w, r)
	})
}
