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
* PostgreSQL: трассировка операций pgx и метрики пула
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

Клиент:

```go
client := &http.Client{
	Transport: obs.HTTPTransport(http.DefaultTransport),
	Timeout:   cfg.Client.Timeout,
}
```

`HTTPTransport` поддерживает [опции `otelhttp`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp#Option).
Для трассировки DNS, подключения и TLS добавьте `HTTPClientTrace`:

```go
client.Transport = obs.HTTPTransport(http.DefaultTransport, obs.HTTPClientTrace())
```

`HTTPClientTrace` использует provider из `obs`. Настройки передаются через
[опции `otelhttptrace`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace#ClientTraceOption):

- `WithoutHeaders` отключает запись исходящих заголовков в trace.
- `WithoutSubSpans` заменяет дочерние spans событиями HTTP span.

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

### PostgreSQL (pgx)

```go
poolCfg, err := pgxpool.ParseConfig(dsn)
if err != nil {
	return err
}
poolCfg.ConnConfig.Tracer = obs.PGXTracer()
pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
if err != nil {
	return err
}
defer pool.Close()

if err := obs.RecordPGXPoolStats(pool); err != nil {
	return err
}
```

Методы используют [`otelpgx`](https://github.com/exaring/otelpgx) с providers из `obs`.
`RecordPGXPoolStats` принимает его опции. Например, чтобы различать несколько пулов
к одной БД, при регистрации метрик можно задать каждому постоянное имя:

```go
if err := obs.RecordPGXPoolStats(pool,
	otelpgx.WithStatsAttributes(attribute.String("db.client.connection.pool.name", "primary")),
); err != nil {
	return err
}
```

Этот вызов заменяет регистрацию без опций выше. По умолчанию имя имеет вид
`host:port/database`. `WithStatsAttributes` относится только к метрикам пула;
`WithMinimumReadDBStatsInterval` задаёт минимальный интервал чтения статистики.

Запросам передавайте `ctx` обработчика, чтобы сохранить связь traces. В фоновых
задачах сначала создайте span через `obs.Tracer`: DB spans требуют записываемого родителя.
Метрики работают и без него; атрибут `db.operation.name` позволяет различать `SELECT`, `INSERT` и другие SQL-операции.

Метрики подключайте один раз на пул. При завершении приложения закройте пул до
`obs.Shutdown`. SQL и параметры не записываются, но сообщения ошибок могут содержать
данные запроса.

### NATS

Обёртки добавляют трассировку отправки и обработки сообщений, передают trace context
и baggage, записывают ошибки в spans. Соединениями и подписками управляет приложение.

#### Core NATS

Отправка через существующее соединение `conn`:

```go
client := obs.NATSClient(conn)
if err := client.PublishMsg(ctx, &nats.Msg{Subject: "jobs.created", Data: payload}); err != nil {
	return err
}
```

Обработка:

```go
_, err = conn.QueueSubscribe("jobs.created", "workers", obs.NATSHandler(workerCtx,
	func(ctx context.Context, msg *nats.Msg) error {
		return process(ctx, msg.Data)
	},
))
if err != nil {
	return err
}
```

`PublishMsg` не ждёт подтверждения доставки. Для запроса с ответом используйте
`client.RequestMsg(ctx, msg)`, а в обработчике отправьте ответ через
`client.PublishMsg(ctx, reply)` с `reply.Subject = msg.Reply`.

#### JetStream

Здесь `js` получен через `jetstream.New(conn)`, а stream и `consumer` для
`jobs.created` уже созданы.

Отправка с ожиданием подтверждения брокера:

```go
publisher := obs.JetStreamPublisher(js)
if _, err := publisher.PublishMsg(ctx, &nats.Msg{Subject: "jobs.created", Data: payload}); err != nil {
	return err
}
```

Обработка с явным ACK при успехе и NAK при ошибке:

```go
consume, err := consumer.Consume(obs.JetStreamHandler(workerCtx,
	func(ctx context.Context, msg jetstream.Msg) error {
		if err := process(ctx, msg.Data()); err != nil {
			return errors.Join(err, msg.Nak())
		}
		return msg.Ack()
	},
))
if err != nil {
	return err
}
defer consume.Stop()
```

PubAck подтверждает приём сообщения брокером. Обработку подтверждает сам consumer
вызовом `Ack()`.

Оба обработчика продолжают входящий trace. Для нового trace со ссылкой на входящий
передайте `observability.WithNATSNewTrace()` в `NATSHandler` или `JetStreamHandler`.
`workerCtx` задаёт локальные deadline и отмену; через NATS они не передаются.

Полный пример Core NATS request/reply см. в [example](example).

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
