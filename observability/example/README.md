# observability example

Минимальный пример HTTP server и HTTP client с observability:

* `server` mode показывает HTTP middleware для server spans и HTTP metrics;
* `handler` добавляет custom span и custom metric;
* `client` передает W3C trace context через `otelhttp.NewTransport`, а server продолжает trace через HTTP middleware.

## Запуск

```sh
make up
go run . --mode server --otel.enable_traces --otel.enable_metrics
```

Обычный запрос:

```sh
curl -v http://localhost:8080/demo
```

Запрос с trace context:

```sh
go run . --mode client --otel.enable_traces
```

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

Обычный `curl` создает trace на server:

```text
curl -> server:
observability-example-server /demo
└── observability-example-server demo.calculate
```

`client` mode создает trace на client и продолжает его на server:

```text
demo client -> server:
observability-example-client demo.client.request
└── localhost HTTP GET
    └── observability-example-server /demo
        └── observability-example-server demo.calculate
```

`localhost HTTP GET` добавляет `traceparent`, а server middleware читает его и связывает `/demo` с client trace.

В `client` mode `obs.InstallGlobal()` делает providers доступными для `otelhttp.NewTransport`.

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

```sh
go run . --otel.enable_metrics --otel.enable_go_runtime_metrics
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

## Главное в коде

`observability.New` создает providers, а middleware подключает HTTP instrumentation:

```go
obs, err := observability.New(ctx, cfg.Observability, application, version)
srv.Use(obs.HTTPMiddleware())
```

## Остановка

```sh
make down
```

Удалить локальные данные OpenObserve:

```sh
make full-down
```
