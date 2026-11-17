package httpserver

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
	"sync"
	"sync/atomic"
	"time"
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

	IPHeader         string `long:"ip_header" env:"IP_HEADER" default:"X-Real-IP" description:"HTTP Request Header for remote IP"`
	UserHeader       string `long:"user_header" env:"USER_HEADER" default:"X-Username" description:"HTTP Request Header for username"`
	AccessLog        string `long:"access_log" env:"ACCESS_LOG" description:"HTTP access log filename (default: STDOUT, '-' means disable)"`
	UseETag          bool   `long:"etag" env:"ETAG" description:"Add ETAG in HTTP response"`
	ETagMaxBodyBytes int    `long:"etag_max_body_bytes" env:"ETAG_MAX_BODY_BYTES" default:"1048576" description:"Maximum response body size buffered for ETag; non-positive means 1 MiB"`

	TLS     TLSConfig             `group:"HTTPS Options"            namespace:"tls"  env-namespace:"TLS"`
	Version VersionResponseConfig `group:"Version response Options" namespace:"vr"`
}

// Service owns an HTTP server and its listener. Configure it before Run.
type Service struct {
	config          Config
	listener        net.Listener
	mux             *http.ServeMux
	handlers        []Handler
	accessLogWriter io.Writer
	started         atomic.Bool
	runCalled       atomic.Bool
	stopOnce        sync.Once
	stopRequested   chan struct{}
	stopCtx         context.Context
	stopCancel      context.CancelFunc
	stopImmediate   bool
	done            chan struct{}
	stopErr         error
}

// AccessLogDisabled holds access_log value for access logging disabling.
const AccessLogDisabled = "-"

// New returns *Service.
func New(cfg Config) *Service {
	return &Service{
		config:        cfg,
		mux:           http.NewServeMux(),
		stopRequested: make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// WithListener supplies a listener owned by the first Run or Shutdown call.
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

// ServeMux returns service muxer.
func (srv *Service) ServeMux() *http.ServeMux {
	return srv.mux
}

// Run serves HTTP(S) once. Context cancellation forces closure; use Shutdown to drain requests.
// The listener passed to WithListener is closed on return, including on error.
func (srv *Service) Run(ctx context.Context) (err error) {
	if !srv.runCalled.CompareAndSwap(false, true) {
		return errors.New("HTTP service already run")
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
		if listener != nil {
			srv.stopErr = errors.Join(srv.stopErr, listener.Close())
		}

		srv.stopErr = errors.Join(srv.stopErr, srv.closeAccessLog())
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
			return fmt.Errorf("HTTP listen: %w", listenErr)
		}
		listener = &ownedListener{Listener: ln}
	}
	handler := srv.ServeMuxWithHandlers()
	if srv.config.AccessLog != AccessLogDisabled {
		if srv.config.AccessLog != "" {
			file, openErr := os.OpenFile(srv.config.AccessLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if openErr != nil {
				return fmt.Errorf("open access log: %w", openErr)
			}
			srv.accessLogWriter = file
		}
		handler = srv.accessLogHandler(handler)
	}
	// Keep request values while letting active requests finish during drain.
	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	hs := &http.Server{
		Handler:           handler,
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		ReadTimeout:       srv.config.ReadTimeout,
		WriteTimeout:      srv.config.WriteTimeout,
		ReadHeaderTimeout: srv.config.ReadHeaderTimeout,
		IdleTimeout:       srv.config.IdleTimeout,
		MaxHeaderBytes:    srv.config.MaxHeaderBytes,
	}
	if srv.config.TLS.NoCheckCertificate {
		hs.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	defer func() {
		cancelRequests()

		srv.stopErr = errors.Join(srv.stopErr, hs.Close())
	}()

	select {
	case <-srv.stopRequested:
		return nil
	default:
	}

	var stopping atomic.Bool

	served := make(chan error, 1)
	go func() {
		var serveErr error
		if srv.config.TLS.CertFile != "" {
			serveErr = hs.ServeTLS(listener, srv.config.TLS.CertFile, srv.config.TLS.KeyFile)
		} else {
			serveErr = hs.Serve(listener)
		}
		if stopping.Load() && (errors.Is(serveErr, http.ErrServerClosed) || errors.Is(serveErr, net.ErrClosed)) {
			serveErr = nil
		}
		if serveErr != nil {
			serveErr = fmt.Errorf("HTTP serve: %w", serveErr)
		}
		served <- serveErr
	}()
	var serveErr error
	serveDone := false
	select {
	case serveErr = <-served:
		serveDone = true
	case <-ctx.Done():
	case <-srv.stopRequested:
	}

	stopping.Store(true)
	srv.beginShutdown(context.Background(), true)
	// Run cancellation also interrupts an explicit Shutdown already in progress.
	stopCancel := context.AfterFunc(ctx, srv.stopCancel)
	defer stopCancel()

	srv.stopErr = srv.shutdown(hs, cancelRequests)

	if !serveDone {
		serveErr = <-served
	}

	return serveErr
}

func (srv *Service) shutdown(hs *http.Server, cancelRequests context.CancelFunc) error {
	if srv.stopImmediate {
		cancelRequests()
		return hs.Close()
	}

	if err := hs.Shutdown(srv.stopCtx); err != nil {
		cancelRequests()
		return errors.Join(fmt.Errorf("HTTP shutdown: %w", err), hs.Close())
	}

	return nil
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

func (srv *Service) closeAccessLog() error {
	f, ok := srv.accessLogWriter.(*os.File)
	if !ok {
		return nil
	}
	srv.accessLogWriter = nil
	if err := f.Close(); err != nil {
		return fmt.Errorf("close access log: %w", err)
	}
	slog.Info("Log closed")
	return nil
}
