# observability example

Минимальный пример HTTP server, HTTP client и NATS worker с observability:

* `server` mode показывает HTTP middleware для server spans и HTTP metrics;
* `handler` добавляет custom metric и выполняет расчёт в текущем span;
* `client` передает W3C trace context через `otelhttp.NewTransport`, а server продолжает trace через HTTP middleware;
* `worker` получает Core NATS запросы и JetStream сообщения, выполняет расчет, отправляет reply или явно подтверждает обработку.

## Запуск

```sh
make up
make run
```

Обычный запрос:

```sh
curl -v http://localhost:8080/demo
```

Запрос с trace context:

```sh
make run MODE=client
```

## NATS request/reply

`make up` запускает NATS, Collector и OpenObserve. Приложения запускаются на хосте
из каталога `observability/example` в отдельных терминалах.
`make run` отправляет telemetry в `http://localhost:4317`, опубликованный Compose.
Другой Collector можно задать через `OTEL_EXPORTER_OTLP_ENDPOINT`.

Терминал 1, worker:

```sh
make run MODE=worker
```

Терминал 2, HTTP server:

```sh
make run
```

Терминал 3, запрос через NATS:

```sh
curl -v http://localhost:8080/demo/nats
```

Для того же запроса с HTTP client span:

```sh
make run MODE=client CLIENT_URL=http://localhost:8080/demo/nats
```

Успешный ответ: HTTP `200`, тело `ok`. Старый `/demo` выполняет расчет прямо в HTTP server.

```text
HTTP client → server /demo/nats → NATS demo.calculate → worker
HTTP client ← server           ← NATS reply          ← worker
```

Worker подписан на subject `demo.calculate` в queue group `demo-workers`.
При нескольких workers каждый запрос обрабатывает один из них.

| Настройка | По умолчанию | Назначение |
|-----------|---------------|------------|
| `NATS_URL` | `nats://127.0.0.1:4222` | Адрес брокера для server и worker. |
| `NATS_TIMEOUT` | `3s` | Таймаут подключения к NATS и ожидания ответа. |

Путь `/demo/nats` использует Core request/reply без хранения сообщений. Порты
`4222` (клиенты) и `8222` (monitoring) доступны только на localhost.
Проверить готовность брокера можно командой `curl -f http://localhost:8222/healthz`
или через `docker compose ps nats`.

При работающем брокере и остановленном worker запрос возвращает HTTP `503`
сразу после ответа NATS об отсутствии подписчиков. Если подписчик есть, но
не отвечает, после `NATS_TIMEOUT` server возвращает HTTP `504`.

## NATS publish без ответа

При запущенных server и worker отправьте сообщение без ожидания результата:

```sh
curl -v -H 'baggage: job.id=calculation-42' http://localhost:8080/demo/nats/publish
```

Server трижды вызывает Core `PublishMsg` с данными `calculate1`, `calculate2`,
`calculate3` и возвращает HTTP `202` с телом `accepted`.
Это означает принятие публикации клиентской библиотекой, без подтверждения
доставки или выполнения расчёта. Если worker не подписан, Core NATS сообщение
не сохраняет для последующей обработки.

Используется тот же subject `demo.calculate`. Worker выполняет расчёт и отправляет
reply только при наличии `msg.Reply`, поэтому поддерживает оба Core сценария.
Входящий trace продолжается по умолчанию, отдельный trace не создаётся:

```text
HTTP /demo/nats/publish → publish demo.calculate → process demo.calculate
```

HTTP-запрос завершается без ожидания worker; завершение HTTP span не завершает
остальные spans этого trace.

## JetStream

Тот же worker при запуске создаёт stream `DEMO_CALCULATE` для subject
`demo.calculate.js` и durable consumer `demo-workers`. Сначала запустите worker,
затем отправьте запрос:

```sh
curl -v -H 'baggage: job.id=calculation-42' http://localhost:8080/demo/jetstream
```

Или используйте клиент с HTTP span:

```sh
make run MODE=client CLIENT_URL=http://localhost:8080/demo/jetstream
```

Server возвращает HTTP `202` и `accepted stream=DEMO_CALCULATE sequence=...`
после PubAck. Это подтверждает сохранение сообщения брокером, а не завершение
расчёта. Worker обрабатывает сообщение и сам вызывает `msg.Ack()`; ошибка ACK
возвращается из обработчика и попадает в span. При добавлении бизнес-ошибок
приложение должно само выбрать `Nak`, `Term` или другую политику. Middleware
ничего не подтверждает автоматически.

Stream использует `WorkQueuePolicy`: сообщение удаляется после ACK. Данные
JetStream хранятся в Docker volume `nats-data`. После первого запуска worker
можно остановить, отправить сообщение и запустить снова: сохранённое сообщение
будет обработано при возобновлении consumer.

## Просмотр telemetry

OpenObserve UI:

```text
http://localhost:5080
```

Логин и пароль лежат в `.env`:

```sh
grep ZO_ROOT_USER_EMAIL .env
grep ZO_ROOT_USER_PASSWORD .env
```

Посмотреть поток telemetry без UI:

```sh
docker compose logs -f otel-collector
```

## Traces

Spans создают HTTP instrumentation и NATS-обвязка. В коде example нет ручных spans;
расчёт и его логи используют context текущего HTTP или NATS process span.

Обычный `curl` создает trace на server:

```text
curl -> server:
observability-example-server /demo
```

`client` mode создает trace на client и продолжает его на server:

```text
demo client -> server:
localhost HTTP GET
└── observability-example-server /demo
```

`localhost HTTP GET` добавляет `traceparent`, а server middleware читает его и связывает `/demo` с client trace.

В `client` mode `obs.InstallGlobal()` делает providers доступными для `otelhttp.NewTransport`.

### NATS traces

Соединение создаётся через `nats.Connect(...)`. Server и worker подключают
`obs.NATSClient(conn)` для Core отправки, `obs.NATSHandler(...)` для подписки,
`obs.JetStreamPublisher(js)` и `obs.JetStreamHandler(...)` для JetStream.
Provider и propagator принадлежат своему `observability.Service`;
`InstallGlobal()` для NATS не нужен.

Core handler по умолчанию продолжает trace отправителя:

```text
observability-example-server /demo/nats
└── request demo.calculate
    └── observability-example-worker process demo.calculate
        └── publish _INBOX.*
```

Request span охватывает ожидание reply. Отдельный receive span не создаётся.
`Calculate(ctx)` и reply publish используют context, полученный обработчиком.

JetStream handler в этом примере использует `observability.WithNATSNewTrace()`.
Чтобы продолжать trace отправителя, уберите эту опцию из `startJetStreamWorker`:

```text
Trace A:
observability-example-server /demo/jetstream
└── publish demo.calculate.js

Trace B:
observability-example-worker process demo.calculate.js [link → publish из Trace A]
```

Baggage передаёт `job.id`, идентификатор задания вызывающей стороны.
Это позволяет найти обработки одного задания в общем worker, даже когда
JetStream начинает новый trace. Передайте один ID в двух запросах:

```sh
curl -H 'baggage: job.id=calculation-42' http://localhost:8080/demo/nats/publish
curl -H 'baggage: job.id=calculation-42' http://localhost:8080/demo/jetstream
```

HTTP middleware извлекает baggage в context, NATS-клиент передаёт его в headers,
а worker получает его через `baggage.FromContext(ctx)`. Логи `NATS message received`
и `JetStream message received` записывают значение в отдельное поле `job_id`.
По `job_id=calculation-42` найдутся три Core обработки и одна JetStream обработка
с разными trace ID. ID используется только для поиска логов, расчёт от него
не зависит; в метрики он не добавляется.

При запуске через `MODE=client` trace server также включает HTTP client spans.
Deadline и cancellation HTTP-запроса через NATS не передаются; consumer использует
свой локальный context, который остаётся активным до завершения drain.

После запросов ищите в OpenObserve сервисы `observability-example-server` и
`observability-example-worker`, spans `request`, `publish`, `process` с subject
в имени. Успешный HTTP-ответ проверяет транспортный сценарий;
доставку traces, metrics и logs в OpenObserve нужно проверить отдельно через
Collector и backend.

## Metrics

Базовый набор server metrics, который должен появиться после запросов:

| Metric                           | Описание                    |
|----------------------------------|-----------------------------|
| `http.server.request.duration`   | Длительность HTTP-запросов. |
| `http.server.request.body.size`  | Размер тела HTTP-запроса.   |
| `http.server.response.body.size` | Размер тела HTTP-ответа.    |

Custom metric создается один раз в конструкторе handler-а:

```go
requests, err := meter.Int64Counter(
    "demo.custom.requests",
    metric.WithDescription("Number of custom demo handler calls."),
    metric.WithUnit("{request}"),
)
```

Custom metric записывается внутри handler-а:

```go
h.requests.Add(ctx, 1, metric.WithAttributes(
    attribute.String("demo.operation", "request"),
))
```

Метрики Go runtime включаются отдельным флагом:

```
--otel.enable_go_runtime_metrics
```

| Metric                  | Описание                                                |
|-------------------------|---------------------------------------------------------|
| `go.config.gogc`        | Текущее значение `GOGC`, которое управляет частотой GC. |
| `go.memory.used`        | Память, используемая Go runtime.                        |
| `go.memory.allocated`   | Объем памяти, выделенный Go runtime.                    |
| `go.memory.allocations` | Количество аллокаций памяти.                            |
| `go.memory.gc.goal`     | Целевой объем heap перед следующей сборкой мусора.      |
| `go.goroutine.count`    | Количество активных goroutine.                          |
| `go.processor.limit`    | Лимит процессоров, доступных Go runtime.                |
| `go.schedule.duration`  | Время задержки планирования goroutine.                  |

В `otel-collector.yaml` добавлен receiver `hostmetrics`, который собирает системные метрики хоста:

| Metric                        | Описание                                                      |
|-------------------------------|---------------------------------------------------------------|
| `system.cpu.time`             | Время CPU по ядрам и состояниям.                              |
| `system.cpu.load_average.1m`  | Средняя нагрузка CPU за 1 минуту.                             |
| `system.cpu.load_average.5m`  | Средняя нагрузка CPU за 5 минут.                              |
| `system.cpu.load_average.15m` | Средняя нагрузка CPU за 15 минут.                             |
| `system.memory.usage`         | Использование RAM по состояниям: used, free, cached и другим. |
| `system.network.io`           | Объем сетевого трафика по интерфейсам и направлениям.         |
| `system.network.errors`       | Количество сетевых ошибок по интерфейсам.                     |
| `system.network.dropped`      | Количество отброшенных сетевых пакетов.                       |
| `system.network.packets`      | Количество сетевых пакетов по интерфейсам и направлениям.     |
| `system.network.connections`  | Количество TCP-соединений по состояниям.                      |
| `system.processes.count`      | Количество процессов по состояниям.                           |
| `system.processes.created`    | Количество созданных процессов.                               |

## Logs

Пример показывает доставку структурных JSON-логов приложения в OpenObserve через Collector:

```text
application -> error.log -> Collector filelog receiver -> OpenObserve
```

## Главное в коде

`observability.New` создает providers, а middleware подключает HTTP instrumentation:

```go
obs, err := observability.New(ctx, cfg.Observability, application, version)
srv.Use(obs.HTTPMiddleware())
```

## Остановка

Остановите приложения через `Ctrl+C`. Worker сначала завершает JetStream callbacks
через consumer drain, затем отправку Core replies и ACK через connection drain
в пределах grace period. Затем остановите Docker stack:

```sh
make down
```

Удалить локальные данные OpenObserve и JetStream:

```sh
make full-down
```
