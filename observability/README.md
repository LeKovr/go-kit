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
* Core NATS и JetStream: отправка, request/reply и обработчики с передачей контекста
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

## NATS

Подключение создаётся обычным `nats.Connect`. `Service` предоставляет адаптеры
отправителя и обработчиков с собственными tracer и W3C propagator;
`InstallGlobal()` не требуется. Instrumentation scope равен имени сервиса.

```go
conn, err := nats.Connect(url, nats.Name(application))
if err != nil {
    return err
}
defer conn.Close()

client := obs.NATSClient(conn)
_, err = conn.QueueSubscribe("jobs.created", "workers", obs.NATSHandler(workerCtx,
    func(ctx context.Context, msg *nats.Msg) error {
        return process(ctx, msg.Data)
    },
))
if err != nil {
    return err
}
if err := client.PublishMsg(ctx, &nats.Msg{Subject: "jobs.created", Data: payload}); err != nil {
    return err
}
```

Для request/reply используется `client.RequestMsg(ctx, msg)`: один client span
измеряет весь вызов до ответа, ошибки или таймаута. Ответ публикуется через
`client.PublishMsg(ctx, &nats.Msg{Subject: request.Reply, Data: result})` из контекста
обработчика. Полученный ответ не заменяет контекст вызывающей стороны.

JetStream создаётся через `jetstream.New(conn)`. Для синхронной публикации
используется `obs.JetStreamPublisher(js).PublishMsg(ctx, msg, opts...)`, а обработчик
подключается к существующему consumer:

```go
consume, err := consumer.Consume(obs.JetStreamHandler(workerCtx,
    func(ctx context.Context, msg jetstream.Msg) error {
        if err := process(ctx, msg.Data()); err != nil {
            return errors.Join(err, msg.Nak())
        }
        return msg.Ack()
    },
    observability.WithNATSNewTrace(),
))
```

По умолчанию process span продолжает входящий trace. `WithNATSNewTrace()` создаёт
новый trace со ссылкой на входящий span. Baggage сохраняется в обоих режимах.
`workerCtx` задаёт локальные values, deadline и cancellation; удалённая отмена и
deadline через заголовки не передаются. Context обработчика действует до возврата
callback; фоновую работу и завершение подписок организует приложение.

Обвязка записывает возвращённые ошибки в span. Panic записывается и пробрасывается
дальше. Логирование, ответы, Ack/Nak/Term и повторную обработку определяет приложение.
ACK внутри callback входит в длительность process span; отдельного ACK span нет.

| Операция | Граница span |
| --- | --- |
| Core `PublishMsg` | Вызов публикации в клиенте, без подтверждения доставки. `ctx` используется для telemetry, не для отмены native publish. |
| Core `RequestMsg` | Вызов до ответа, ошибки или таймаута. |
| JetStream `PublishMsg` | Публикация до PubAck или ошибки. PubAck не означает завершение обработки consumer. |
| Handler | Выполнение callback, включая явные ответ или ACK внутри него. |

Отправка копирует сообщение и его заголовки, заменяя поля propagation данными из
переданного `ctx`. Исходные заголовки не изменяются; payload не копируется, его нельзя
менять во время вызова. Заголовки читаются без учёта регистра, несколько значений
baggage сохраняются. Конфликтующие traceparent не выбираются произвольно: такой
входящий trace context считается невалидным.

При выключенных traces доступны только propagation и выполнение исходных операций:
новые spans не экспортируются, входящий trace context и baggage сохраняются, включая
режим `WithNATSNewTrace()`. Глобальные OTel providers на эти адаптеры не влияют.

Подписки, `Drain`, `Close`, TLS, credentials и reconnect остаются в `nats.go`.
Первая версия покрывает Core send/request и JetStream sync publish/callback.
Вызовы через исходные клиенты, async publish и batch/fetch автоматически не
инструментируются. Собственные метрики NATS в эту обвязку не входят.

Полный запуск Core и JetStream с Docker NATS, Collector и OpenObserve описан в
[example](example).

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
