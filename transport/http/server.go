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

	mu          sync.Mutex
	runCalled   bool
	stopping    bool
	httpServer  *http.Server // Published after startup, before Serve.
	cancelDrain context.CancelFunc
	drained     chan error
	done        chan struct{}
	stopErr     error
}

// AccessLogDisabled holds access_log value for access logging disabling.
const AccessLogDisabled = "-"

// New returns *Service.
func New(cfg Config) *Service {
	return &Service{
		config: cfg,
		mux:    http.NewServeMux(),
		done:   make(chan struct{}),
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
	srv.mu.Lock()
	if srv.runCalled {
		srv.mu.Unlock()
		return errors.New("HTTP service already run")
	}

	srv.runCalled = true
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
	defer func() {
		if listener != nil {
			srv.stopErr = errors.Join(srv.stopErr, listener.Close())
		}

		srv.stopErr = errors.Join(srv.stopErr, srv.closeAccessLog())
		err = errors.Join(err, srv.stopErr)
		close(srv.done)
	}()

	if ctx.Err() != nil {
		return nil
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
	closeServer := sync.OnceValue(func() error {
		cancelRequests()
		return hs.Close()
	})
	defer func() { srv.stopErr = errors.Join(srv.stopErr, closeServer()) }()

	srv.mu.Lock()
	if srv.stopping {
		srv.mu.Unlock()
		return nil
	}

	srv.httpServer = hs
	srv.mu.Unlock()

	stop := context.AfterFunc(ctx, func() {
		srv.mu.Lock()
		srv.stopping = true
		cancel := srv.cancelDrain
		srv.mu.Unlock()

		if cancel != nil {
			cancel()
		} else {
			closeServer()
		}
	})
	defer stop()

	if srv.config.TLS.CertFile != "" {
		err = hs.ServeTLS(listener, srv.config.TLS.CertFile, srv.config.TLS.KeyFile)
	} else {
		err = hs.Serve(listener)
	}

	srv.mu.Lock()
	stopping = srv.stopping
	srv.httpServer = nil
	srv.stopping = true
	drained := srv.drained
	srv.mu.Unlock()

	// Serve returns as soon as Shutdown closes the listener. Keep requests
	// and their access log alive until Shutdown finishes.
	if drained != nil {
		if drainErr := <-drained; drainErr != nil {
			srv.stopErr = fmt.Errorf("HTTP shutdown: %w", drainErr)
		}
	}

	if stopping && (errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)) {
		return nil
	}

	return fmt.Errorf("HTTP serve: %w", err)
}

// Shutdown stops admission and drains active requests within ctx. Before Run it
// closes a supplied listener and prevents serving. Later calls only wait for cleanup within their own context.
func (srv *Service) Shutdown(ctx context.Context) error {
	srv.mu.Lock()
	if !srv.stopping {
		srv.stopping = true

		if !srv.runCalled {
			srv.mu.Unlock()
			if srv.listener != nil {
				srv.stopErr = srv.listener.Close()
			}

			close(srv.done)
			return srv.stopErr
		}

		if hs := srv.httpServer; hs != nil {
			drainCtx, cancel := context.WithCancel(ctx)
			srv.cancelDrain = cancel
			srv.drained = make(chan error, 1)

			go func() {
				defer cancel()
				srv.drained <- hs.Shutdown(drainCtx)
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
