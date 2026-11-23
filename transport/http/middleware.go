package httpserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/felixge/httpsnoop"
)

// Handler is a http midleware handler.
type Handler func(http.Handler) http.Handler

// Use adds handler for muxer.
func (srv *Service) Use(handler Handler) *Service {
	srv.handlers = append(srv.handlers, handler)
	return srv
}

// ServeMuxWithHandlers return mux joined with defined by Use handlers.
func (srv *Service) ServeMuxWithHandlers() http.Handler {
	var mux http.Handler = srv.mux
	for _, handler := range srv.handlers {
		mux = handler(mux)
	}
	if srv.config.UseETag {
		mux = withETag(mux, srv.config.ETagMaxBodyBytes)
	}
	return mux
}

// WithAccessLog calculates estimate and prints HTTP request log.
func (srv *Service) accessLogHandler(handler http.Handler) http.Handler {
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
