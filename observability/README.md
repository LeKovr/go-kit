# go-kit/observability

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/observability.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/observability
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/observability
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/observability
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=observability/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=observability&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет добавляет observability в Go-сервис через OpenTelemetry:

* traces и metrics с экспортом в OTLP endpoint
* W3C propagation (`traceparent`, `baggage`)
* Go runtime metrics
* HTTP middleware на базе `otelhttp`
* `Meter` и `Tracer` для бизнес-метрик и прикладных span

## Использование

```go
obs, err := observability.New(ctx, cfg.Observability, application, version)
if err != nil {
	return err
}
defer obs.Shutdown(ctx)

srv := server.New(cfg.Server)
srv.Use(obs.HTTPMiddleware())
```

Полный пример запуска с OpenTelemetry Collector и OpenObserve см. в [example](example)

## Архитектура

Общая схема передачи observability-сигналов от Go-сервиса до Collector и backend-ов показана в [схеме observability-сигналов](architecture.md).

## Настройки

`Traces`, `metrics` и `Go runtime metrics` выключены по умолчанию и включаются отдельными флагами. 

Использование пакета добавляет в сервис следующие опции:

```
OpenTelemetry Options:
      --otel.exporter_otlp_endpoint=    OTLP gRPC endpoint URL (default: http://localhost:4317) [$OTEL_EXPORTER_OTLP_ENDPOINT]
      --otel.service_instance_id=       Unique service instance id, e.g. pod name, pod UID, hostname, or container id [$OTEL_SERVICE_INSTANCE_ID]
      --otel.metric_interval=           OTLP metrics export interval (default: 10s) [$OTEL_METRIC_INTERVAL]
      --otel.shutdown_timeout=          OpenTelemetry shutdown timeout (default: 5s) [$OTEL_SHUTDOWN_TIMEOUT]
      --otel.enable_traces              Enable traces export [$OTEL_ENABLE_TRACES]
      --otel.enable_metrics             Enable metrics export [$OTEL_ENABLE_METRICS]
      --otel.enable_go_runtime_metrics  Enable Go runtime metrics collection [$OTEL_ENABLE_GO_RUNTIME_METRICS]
```
