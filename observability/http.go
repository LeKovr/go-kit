package observability

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
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

// WithSpanNameFormatter replaces the default method and optional route span name.
func WithSpanNameFormatter(formatter func(string, *http.Request) string) HTTPOption {
	return func(cfg *httpConfig) {
		if formatter != nil {
			cfg.spanNameFormatter = formatter
		}
	}
}

// HTTPMiddleware returns a net/http middleware compatible with server.Handler.
func (svc Service) HTTPMiddleware(opts ...HTTPOption) func(http.Handler) http.Handler {
	cfg := defaultHTTPConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	otelOpts := []otelhttp.Option{
		otelhttp.WithServerName(svc.serviceName),
		otelhttp.WithSpanNameFormatter(cfg.spanNameFormatter),
		otelhttp.WithFilter(func(r *http.Request) bool {
			_, ignored := cfg.ignoredPaths[r.URL.Path]
			return !ignored
		}),
		otelhttp.WithTracerProvider(svc.tracerProviderOrNoop()),
		otelhttp.WithMeterProvider(svc.meterProviderOrNoop()),
		otelhttp.WithPropagators(svc.propagator()),
	}

	otelMiddleware := otelhttp.NewMiddleware(svc.serviceName, otelOpts...)

	return func(next http.Handler) http.Handler {
		return otelMiddleware(next)
	}
}

// HTTPTransport returns client instrumentation for base; nil uses http.DefaultTransport.
// Service providers and propagation take precedence over opts; OTel globals are not used.
// The caller retains ownership of base, including its idle connections.
func (svc Service) HTTPTransport(base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper {
	options := make([]otelhttp.Option, 0, len(opts)+3)
	options = append(options, opts...)
	options = append(options,
		otelhttp.WithTracerProvider(svc.tracerProviderOrNoop()),
		otelhttp.WithMeterProvider(svc.meterProviderOrNoop()),
		otelhttp.WithPropagators(svc.propagator()),
	)

	return otelhttp.NewTransport(base, options...)
}

// HTTPClientTrace returns optional HTTPTransport details for DNS, connections and TLS.
// The Service tracer provider takes precedence over opts; globals are not used.
func (svc Service) HTTPClientTrace(opts ...otelhttptrace.ClientTraceOption) otelhttp.Option {
	options := make([]otelhttptrace.ClientTraceOption, 0, len(opts)+1)
	options = append(options, opts...)
	options = append(options, otelhttptrace.WithTracerProvider(svc.tracerProviderOrNoop()))

	return otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
		return otelhttptrace.NewClientTrace(ctx, options...)
	})
}

func defaultHTTPConfig() httpConfig {
	ignored := make(map[string]struct{}, len(defaultIgnoredPaths))
	for _, path := range defaultIgnoredPaths {
		ignored[path] = struct{}{}
	}

	return httpConfig{
		ignoredPaths: ignored,
		spanNameFormatter: func(_ string, r *http.Request) string {
			if r.Pattern == "" {
				return r.Method
			}
			route := r.Pattern
			if i := strings.IndexAny(route, " \t"); i >= 0 {
				route = strings.TrimLeft(route[i+1:], " \t")
			}
			return r.Method + " " + route
		},
	}
}
