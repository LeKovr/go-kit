# observability example

Пример показывает базовый контракт:

* приложение на Go отправляет traces и metrics в OTLP endpoint;
* приложение пишет logs в stdout/stderr;
* `observability.Service` настраивает OpenTelemetry providers и HTTP middleware;
* OpenTelemetry Collector принимает telemetry, добавляет infra attributes, батчит и отправляет данные в выбранное хранилище.
* Collector можно расширить profiling данными, чтобы собирать базовые серверные metrics: CPU, RAM, network, load и process count через `hostmetrics` receiver.

В этом примере backendом выбран OpenObserve потому, что он легковесный и умеет работать сразу и с логами, и с метриками, и с трейсами. Код приложения не зависит от OpenObserve.

## Запуск

Из каталога `observability/example`:

```sh
make up
```

Запустить приложение:

```sh
go run . --otel.enable_traces --otel.enable_metrics
```

`OTEL_EXPORTER_OTLP_ENDPOINT` задается только URL-форматом, например `http://127.0.0.1:4317` для Collector на том же сервере или `https://collector.example.com:4317` для TLS endpoint. Формат `host:port` не поддерживается.

Traces и metrics выключены по умолчанию, чтобы простой локальный запуск не пытался отправлять telemetry в Collector. Включить их можно независимо:

```sh
go run . --otel.enable_traces
go run . --otel.enable_metrics
```

Проверить HTTP handler:

```sh
curl -v http://localhost:8080/hello
```

Сделайте несколько запросов, чтобы появились spans и HTTP metrics.

OpenObserve UI:

```text
http://localhost:5080
```

Логин:

```sh
grep ZO_ROOT_USER_EMAIL .env
```

Пароль:

```sh
grep ZO_ROOT_USER_PASSWORD .env
```

Посмотреть поток telemetry без UI:

```sh
docker compose logs -f otel-collector
```

App metrics по умолчанию отключены вместе с общим metrics-сигналом. Включить их можно флагом:

```sh
go run . --otel.enable_metrics
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

Если добавить `hostmetrics` receiver в `otel-collector.yaml` и подключить его к `metrics` pipeline, Collector начнет собирать системные метрики хоста:

```yaml
receivers:
  hostmetrics:
    collection_interval: 10s
    root_path: /hostfs
    scrapers:
      cpu:
      memory:
      network:
      load:
      processes:

service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
```

В OpenObserve после этого добавятся метрики:

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


## Другие backend-ы

Go-приложение не нужно менять при переходе с OpenObserve на другое хранилище. Меняется только конфиг OpenTelemetry Collector.

Collector уже собирает CPU, RAM, network, load и process count с сервера через `hostmetrics`. Его можно расширить disk/filesystem/paging metrics или profiling pipeline, если backend умеет принимать profiles. Эти сигналы не требуют изменений в коде приложения: сервис по-прежнему отправляет свои traces/metrics в OTLP и пишет logs в stdout/stderr, а серверная telemetry собирается на уровне инфраструктуры.

Примеры направлений:

* Loki: отправлять logs из Collector/Filelog/Journald receiver в `loki` exporter, а traces/metrics оставить в других pipeline.
* Prometheus: включить `prometheus` exporter в metrics pipeline, чтобы Prometheus scrape-ил Collector endpoint.
* VictoriaMetrics: отправлять metrics через `prometheusremotewrite` exporter в VictoriaMetrics.
* Tempo: отправлять traces через `otlp` exporter в Tempo.
* Jaeger: отправлять traces через OTLP или Jaeger exporter, в зависимости от версии стека.
* Datadog/New Relic/Honeycomb: заменить exporter в Collector на vendor-specific exporter или OTLP endpoint вендора.

Граница ответственности остается такой:

```text
Go app
  traces/metrics -> OTLP Collector endpoint
  logs           -> stdout/stderr

OpenTelemetry Collector
  receivers   -> otlp/filelog/journald/...
  processors  -> resource/batch/filter/sampling/...
  exporters   -> OpenObserve/Loki/Prometheus/VictoriaMetrics/Tempo/Datadog/...
  hostmetrics -> CPU/RAM/network/load/process metrics, optional disk/profiles
```

## Остановка

```sh
make down
```

Удалить локальные данные OpenObserve:

```sh
make full-down
```
