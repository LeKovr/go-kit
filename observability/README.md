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

Пакет включает базовую observability для Go-сервисов:

* traces в OTLP endpoint
* metrics в OTLP endpoint
* W3C propagation (`traceparent`, `baggage`)
* Go runtime metrics через `go.opentelemetry.io/contrib/instrumentation/runtime`
* HTTP server traces/metrics через `otelhttp` middleware
* конструкторы `Meter` и `Tracer` для бизнес-метрик и прикладных span
* logs остаются stdout/stderr, storage и delivery делает инфраструктура

Архитектурная схема observability-сигналов: [architecture.md](architecture.md).

`observability.New` создает providers, но не меняет глобальное состояние OpenTelemetry. Если приложению нужна поддержка сторонней instrumentation, которая берет providers через `otel.GetTracerProvider()` или `otel.GetMeterProvider()`, вызовите `obs.InstallGlobal()` в `main`.

`OTEL_EXPORTER_OTLP_ENDPOINT` задается только URL-форматом, например `http://127.0.0.1:4317` для Collector на том же сервере или `https://collector.example.com:4317` для удаленного TLS endpoint.

Traces и metrics выключены по умолчанию, чтобы простой локальный запуск не пытался отправлять telemetry в Collector. Их можно включить независимо через `--otel.enable_traces` / `OTEL_ENABLE_TRACES=true` и `--otel.enable_metrics` / `OTEL_ENABLE_METRICS=true`.

Metrics экспортируются периодически. Интервал задается `--otel.metric_interval` и по умолчанию равен `10s`.
Go runtime metrics включаются через `--otel.enable_go_runtime_metrics` или `OTEL_ENABLE_GO_RUNTIME_METRICS=true`. Этот флаг работает только если metrics включены.

## Контракт

Приложение не знает про `OpenObserve`, `Loki`, `Tempo`, `VictoriaMetrics`, `Prometheus` или `Datadog`. Оно отправляет `traces`/`metrics` в OTLP endpoint и пишет structured logs в stdout/stderr.

OpenTelemetry Collector принимает telemetry, добавляет infra attributes, батчит, ретраит, фильтрует, семплирует и отправляет данные в выбранное хранилище.

`slogger` добавляет `trace_id` и `span_id` в JSON logs, если лог пишется через `slog.InfoContext`, `slog.ErrorContext` и в context есть активный span.
