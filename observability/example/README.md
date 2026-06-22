# observability example

Минимальный пример HTTP-сервиса с observability:

* `observability.Service` настраивает traces и metrics;
* HTTP middleware собирает server spans и HTTP metrics;
* handler добавляет custom span, custom metric и исходящий HTTP-вызов.

## Запуск

```sh
make up
go run . --otel.enable_traces --otel.enable_metrics
```

Сделайте несколько запросов, чтобы появились метрики.

```sh
curl -v http://localhost:8080/demo
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

Базовый набор metrics, который должен появиться после запросов:

| Metric                           | Описание                    |
|----------------------------------|-----------------------------|
| `http.server.request.duration`   | Длительность HTTP-запросов. |
| `http.server.request.body.size`  | Размер тела HTTP-запроса.   |
| `http.server.response.body.size` | Размер тела HTTP-ответа.    |

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

Trace для `GET /demo` содержит:

```text
HTTP server span for /demo
├── demo.calculate
└── HTTP GET
```

## Главное в коде

`observability.New` создает providers, а middleware подключает HTTP instrumentation:

```go
obs, err := observability.New(ctx, cfg.Observability, application, version)
srv.Use(obs.HTTPMiddleware())
```

Custom metric создается один раз в конструкторе handler-а:

```go
requests, err := meter.Int64Counter(
    "demo.custom.requests",
    metric.WithDescription("Number of custom demo handler calls."),
    metric.WithUnit("{request}"),
)
```

Custom span создается вокруг своего участка кода:

```go
_, span := h.tracer.Start(ctx, "demo.calculate")
defer span.End()

span.SetAttributes(attribute.String("demo.step", "calculate"))
```

Custom metric:

```go
h.requests.Add(ctx, 1, metric.WithAttributes(
    attribute.String("demo.operation", "request"),
))
```

Внешний HTTP API в примере имитируется через `httptest.NewServer`. В реальном сервисе здесь будет URL вашей зависимости.

`obs.InstallGlobal()` делает providers доступными для стандартной instrumentation. Поэтому `otelhttp.NewTransport` создает span для исходящего HTTP-запроса, а `r.Context()` связывает его с текущим server span:

```go
restore := obs.InstallGlobal()
defer restore()

client := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport),
    Timeout:   3 * time.Second,
}

req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.externalURL, nil)
resp, err := h.client.Do(req)
```

## Остановка

```sh
make down
```

Удалить локальные данные OpenObserve:

```sh
make full-down
```
