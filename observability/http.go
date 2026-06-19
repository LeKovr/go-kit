package observability

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var defaultIgnoredPaths = []string{"/health", "/ready", "/metrics"}

type httpConfig struct {
	ignoredPaths      map[string]struct{}
	spanNameFormatter func(string, *http.Request) string
}

// HTTPOption changes HTTP middleware behavior.
type HTTPOption func(*httpConfig)

// WithIgnoredPaths replaces the default untraced paths.
func WithIgnoredPaths(paths ...string) HTTPOption {
	return func(cfg *httpConfig) {
		cfg.ignoredPaths = make(map[string]struct{}, len(paths))
		for _, path := range paths {
			cfg.ignoredPaths[path] = struct{}{}
		}
	}
}

// WithSpanNameFormatter replaces the default "METHOD /path" span name.
func WithSpanNameFormatter(formatter func(string, *http.Request) string) HTTPOption {
	return func(cfg *httpConfig) {
		if formatter != nil {
			cfg.spanNameFormatter = formatter
		}
	}
}

// HTTPMiddleware returns a net/http middleware compatible with server.Handler.
func (srv Service) HTTPMiddleware(opts ...HTTPOption) func(http.Handler) http.Handler {
	cfg := defaultHTTPConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	otelOpts := []otelhttp.Option{
		otelhttp.WithServerName(srv.serviceName),
		otelhttp.WithSpanNameFormatter(cfg.spanNameFormatter),
		otelhttp.WithFilter(func(r *http.Request) bool {
			_, ignored := cfg.ignoredPaths[r.URL.Path]
			return !ignored
		}),
	}
	if srv.tracerProvider != nil {
		otelOpts = append(otelOpts, otelhttp.WithTracerProvider(srv.tracerProvider))
	}
	if srv.meterProvider != nil {
		otelOpts = append(otelOpts, otelhttp.WithMeterProvider(srv.meterProvider))
	}

	otelMiddleware := otelhttp.NewMiddleware(srv.serviceName, otelOpts...)

	return func(next http.Handler) http.Handler {
		return otelMiddleware(next)
	}
}

func defaultHTTPConfig() httpConfig {
	ignored := make(map[string]struct{}, len(defaultIgnoredPaths))
	for _, path := range defaultIgnoredPaths {
		ignored[path] = struct{}{}
	}

	return httpConfig{
		ignoredPaths: ignored,
		spanNameFormatter: func(_ string, r *http.Request) string {
			if r.Pattern != "" {
				return r.Pattern
			}

			return r.Method + " " + r.URL.Path
		},
	}
}
