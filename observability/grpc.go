package observability

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/stats"
)

// GRPCServerHandler returns instrumentation for grpc.StatsHandler, including streams.
// Service providers and propagation take precedence over opts; globals are not used.
func (svc Service) GRPCServerHandler(opts ...otelgrpc.Option) stats.Handler {
	options := make([]otelgrpc.Option, 0, len(opts)+3)
	options = append(options, opts...)
	options = append(options,
		otelgrpc.WithTracerProvider(svc.tracerProviderOrNoop()),
		otelgrpc.WithMeterProvider(svc.meterProviderOrNoop()),
		otelgrpc.WithPropagators(svc.propagator()),
	)

	return otelgrpc.NewServerHandler(options...)
}

// GRPCClientHandler returns instrumentation for grpc.WithStatsHandler, including streams.
// Service providers and propagation take precedence over opts; globals are not used.
func (svc Service) GRPCClientHandler(opts ...otelgrpc.Option) stats.Handler {
	options := make([]otelgrpc.Option, 0, len(opts)+3)
	options = append(options, opts...)
	options = append(options,
		otelgrpc.WithTracerProvider(svc.tracerProviderOrNoop()),
		otelgrpc.WithMeterProvider(svc.meterProviderOrNoop()),
		otelgrpc.WithPropagators(svc.propagator()),
	)

	return otelgrpc.NewClientHandler(options...)
}
