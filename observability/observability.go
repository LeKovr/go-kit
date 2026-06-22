package observability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// Config holds OpenTelemetry bootstrap options.
type Config struct {
	OTLPEndpoint string `default:"http://localhost:4317" long:"exporter_otlp_endpoint" description:"OTLP gRPC endpoint URL" env:"EXPORTER_OTLP_ENDPOINT"`
	InstanceID   string `long:"service_instance_id" description:"Unique service instance id, e.g. pod name, pod UID, hostname, or container id" env:"SERVICE_INSTANCE_ID"`

	MetricInterval  time.Duration `default:"10s" description:"OTLP metrics export interval" long:"metric_interval" env:"METRIC_INTERVAL"`
	ShutdownTimeout time.Duration `default:"5s" description:"OpenTelemetry shutdown timeout" long:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`

	EnableTraces           bool `description:"Enable traces export" long:"enable_traces" env:"ENABLE_TRACES"`
	EnableMetrics          bool `description:"Enable metrics export" long:"enable_metrics" env:"ENABLE_METRICS"`
	EnableGoRuntimeMetrics bool `description:"Enable Go runtime metrics collection" long:"enable_go_runtime_metrics" env:"ENABLE_GO_RUNTIME_METRICS"`
}

// Service holds configured observability providers.
type Service struct {
	config Config

	serviceName    string
	serviceVersion string

	tracerProvider trace.TracerProvider
	meterProvider  otelmetric.MeterProvider

	shutdowns []func(context.Context) error
}

// New creates OpenTelemetry providers for traces and metrics.
func New(ctx context.Context, cfg Config, serviceName, serviceVersion string) (*Service, error) {
	svc := &Service{
		serviceName:    serviceName,
		serviceVersion: serviceVersion,
		config:         cfg,
		tracerProvider: nooptrace.NewTracerProvider(),
		meterProvider:  noopmetric.NewMeterProvider(),
	}

	if !cfg.EnableTraces && !cfg.EnableMetrics {
		return svc, nil
	}

	r, err := newResource(cfg, serviceName, serviceVersion)
	if err != nil {
		return nil, err
	}

	if cfg.EnableMetrics {
		meterProvider, err := newMeterProvider(ctx, cfg, r)
		if err != nil {
			return nil, err
		}

		svc.meterProvider = meterProvider
		svc.shutdowns = append(svc.shutdowns, meterProvider.Shutdown)
	}

	if cfg.EnableTraces {
		tracerProvider, err := newTracerProvider(ctx, cfg, r)
		if err != nil {
			return nil, errors.Join(err, svc.Shutdown(ctx))
		}

		svc.tracerProvider = tracerProvider
		svc.shutdowns = append(svc.shutdowns, tracerProvider.Shutdown)
	}

	if cfg.EnableMetrics && cfg.EnableGoRuntimeMetrics {
		if err := otelruntime.Start(otelruntime.WithMeterProvider(svc.meterProvider)); err != nil {
			return nil, errors.Join(err, svc.Shutdown(ctx))
		}
	}

	return svc, nil
}

func newMeterProvider(ctx context.Context, cfg Config, r *resource.Resource) (*sdkmetric.MeterProvider, error) {
	meterExporter, err := newMetricExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	readerOpts := []sdkmetric.PeriodicReaderOption{
		sdkmetric.WithInterval(cfg.MetricInterval),
	}

	if cfg.EnableGoRuntimeMetrics {
		readerOpts = append(readerOpts, sdkmetric.WithProducer(otelruntime.NewProducer()))
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(r),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(meterExporter, readerOpts...)),
	), nil
}

func newTracerProvider(ctx context.Context, cfg Config, r *resource.Resource) (*sdktrace.TracerProvider, error) {
	traceExporter, err := newTraceOTLPExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(r),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(traceExporter),
	), nil
}

// InstallGlobal configures this service as the process-wide OpenTelemetry
// provider set. It returns a restore function useful for tests and embedded
// applications.
func (svc *Service) InstallGlobal() func() {
	previousTracerProvider := otel.GetTracerProvider()
	previousMeterProvider := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()

	otel.SetTracerProvider(svc.tracerProviderOrNoop())
	otel.SetMeterProvider(svc.meterProviderOrNoop())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		otel.SetTracerProvider(previousTracerProvider)
		otel.SetMeterProvider(previousMeterProvider)
		otel.SetTextMapPropagator(previousPropagator)
	}
}

// Shutdown flushes and stops OpenTelemetry providers.
func (svc Service) Shutdown(ctx context.Context) error {
	if svc.config.ShutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, svc.config.ShutdownTimeout)
		defer cancel()
	}

	return shutdownAll(ctx, svc.shutdowns...)
}

func shutdownAll(ctx context.Context, funcs ...func(context.Context) error) error {
	var errs []error

	for _, fn := range funcs {
		if fn == nil {
			continue
		}

		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Meter returns a meter for application-specific instruments.
func (svc Service) Meter(name string, opts ...otelmetric.MeterOption) otelmetric.Meter {
	return svc.meterProviderOrNoop().Meter(name, opts...)
}

// Tracer returns a tracer for application-specific spans.
func (svc Service) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return svc.tracerProviderOrNoop().Tracer(name, opts...)
}

func (svc Service) meterProviderOrNoop() otelmetric.MeterProvider {
	if svc.meterProvider != nil {
		return svc.meterProvider
	}

	return noopmetric.NewMeterProvider()
}

func (svc Service) tracerProviderOrNoop() trace.TracerProvider {
	if svc.tracerProvider != nil {
		return svc.tracerProvider
	}

	return nooptrace.NewTracerProvider()
}

func newResource(cfg Config, serviceName, serviceVersion string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	}

	if cfg.InstanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceID(cfg.InstanceID))
	}

	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes("", attrs...),
	)
}

func newTraceExporterOptions(cfg Config) ([]otlptracegrpc.Option, error) {
	opts := make([]otlptracegrpc.Option, 0, 1)
	if cfg.OTLPEndpoint != "" {
		if err := validateOTLPEndpointURL(cfg.OTLPEndpoint); err != nil {
			return nil, err
		}

		opts = append(opts, otlptracegrpc.WithEndpointURL(cfg.OTLPEndpoint))
	}

	return opts, nil
}

func newMetricExporterOptions(cfg Config) ([]otlpmetricgrpc.Option, error) {
	opts := make([]otlpmetricgrpc.Option, 0, 1)
	if cfg.OTLPEndpoint != "" {
		if err := validateOTLPEndpointURL(cfg.OTLPEndpoint); err != nil {
			return nil, err
		}

		opts = append(opts, otlpmetricgrpc.WithEndpointURL(cfg.OTLPEndpoint))
	}

	return opts, nil
}

func validateOTLPEndpointURL(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse OTLP endpoint URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("OTLP endpoint must use http or https URL scheme: %q", endpoint)
	}

	if u.Host == "" {
		return fmt.Errorf("OTLP endpoint must include host: %q", endpoint)
	}

	return nil
}

func newTraceOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	opts, err := newTraceExporterOptions(cfg)
	if err != nil {
		return nil, err
	}

	return otlptracegrpc.New(ctx, opts...)
}

func newMetricExporter(ctx context.Context, cfg Config) (sdkmetric.Exporter, error) {
	opts, err := newMetricExporterOptions(cfg)
	if err != nil {
		return nil, err
	}

	return otlpmetricgrpc.New(ctx, opts...)
}
