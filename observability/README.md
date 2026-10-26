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
* HTTP server/client instrumentation на базе `otelhttp`
* gRPC client/server instrumentation на базе `otelgrpc`, включая streaming
* Core NATS и JetStream: отправка и обработка сообщений с передачей контекста
* `Meter` и `Tracer` для бизнес-метрик и прикладных span

## Использование

### Инициализация

```go
obs, err := observability.New(ctx, cfg.Observability, application, version)
if err != nil {
	return err
}
defer obs.Shutdown(context.Background())
```

### InstallGlobal

Для сторонних библиотек, использующих глобальные OTel providers и propagator:

```go
restore := obs.InstallGlobal()
defer restore()
```

### HTTP

Сервер:

```go
srv := server.New(cfg.Server)
srv.Use(obs.HTTPMiddleware())
```

Имя server span — `METHOD route`, где `route` — шаблон из `Request.Pattern`.
Без шаблона используется только метод, например `GET`.

Клиент:

```go
client := &http.Client{
	Transport: obs.HTTPTransport(http.DefaultTransport),
	Timeout:   cfg.Client.Timeout,
}
```

Полный HTTP-пример с OpenTelemetry Collector и OpenObserve см. в [example](example).

### gRPC

Сервер:

```go
srv := grpc.NewServer(
	grpc.StatsHandler(obs.GRPCServerHandler()),
)
```

Клиент:

```go
conn, err := grpc.NewClient(
	target,
	grpc.WithTransportCredentials(creds),
	grpc.WithStatsHandler(obs.GRPCClientHandler()),
)
if err != nil {
	return err
}
defer conn.Close()
```

Handlers поддерживают unary и streaming RPC. Вызовам клиента передавайте текущий
`ctx` обработчика, чтобы сохранить связь traces.

Оба метода принимают [опции `otelgrpc`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc#Option),
например `WithFilter` для исключения RPC. Providers и propagator всегда берутся
из `obs` и имеют приоритет над options.

### NATS

Core NATS:

```go
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

Для request/reply используйте `client.RequestMsg(ctx, msg)`. Ответ отправляется через
`client.PublishMsg` на subject `msg.Reply` из контекста обработчика.

Для JetStream используйте `obs.JetStreamPublisher(js).PublishMsg(ctx, msg, opts...)`.
Обработчик подключается к существующему consumer:

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
if err != nil {
	return err
}
defer consume.Stop()
```

| Вызов | Граница span |
| --- | --- |
| Core `PublishMsg` | Возврат из публикации в клиенте, без подтверждения доставки и отмены через `ctx`. |
| Core `RequestMsg` | Получение reply, ошибка или таймаут, включая ожидание ответа. |
| JetStream `PublishMsg` | Получение PubAck или ошибки. PubAck подтверждает приём брокером, не завершение обработки. |
| Handler | Возврат callback, включая ответ или ACK внутри него. |

Отправка создаёт spans `Producer` с `messaging.operation.type=send`, обработка —
`Consumer` с типом `process`. У request сохраняется `messaging.operation.name=request`.
Исходный subject записывается в `messaging.destination.name`; для `_INBOX.…`
используется шаблон `_INBOX.*` в имени span и `messaging.destination.template`.

По умолчанию обработчик продолжает входящий trace. `WithNATSNewTrace()` создаёт
новый trace со ссылкой на входящий span. Baggage сохраняется в обоих случаях.
`workerCtx` задаёт локальные values, deadline и cancellation; отмена и deadline
отправителя через NATS не передаются. Возврат callback завершает span, срок жизни
context определяется `workerCtx`.

Ошибки записываются в span с `error.type`: `timeout`, `canceled`, `no_responders`,
`connection_closed` или `_OTHER`. Классификация через `errors.Is` учитывает
обёрнутые ошибки. Panic получает тип `panic` и пробрасывается дальше.
При успехе `error.type` отсутствует.

Заголовки читаются без учёта регистра: используется первое значение `traceparent`
и все значения baggage. Отправка копирует заголовки; payload нельзя менять во время
вызова. При выключенных traces передаются trace context и baggage без новых spans.

Соединение, подписки, ACK, retry и shutdown остаются у приложения. Async publish
и batch/fetch не инструментируются. Пример запуска см. в [example](example).

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
