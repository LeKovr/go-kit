# Архитектура observability

Схема показывает путь observability-сигналов от Go-сервиса до backend-ов хранения и анализа.

Пакет `observability` отвечает за Go-side часть: настройку OpenTelemetry SDK, HTTP мониторинг через `otelhttp`, сбор метрик `Go runtime`, `custom Tracer/Meter` и экспорт `traces/metrics` по OTLP.

`slogger` пишет JSON-логи с `trace_id`/`span_id` в `stderr` или файл, а Collector читает их через `filelog` или `journald`

Сбор `host metrics` и логов, обработка сигналов и маршрутизация в один или несколько backend-ов относятся к инфраструктурному слою и настраиваются в конфигурации Collector

```mermaid
flowchart TB
    subgraph host_layer["Host / container"]
        subgraph app_layer["Go service process"]
            app["Application code"]
            runtime["Go runtime"]

            subgraph observability_layer["go-kit/observability"]
                otel["OpenTelemetry SDK setup"]
            end

            logger["go-kit/slogger"]

            app -->|"HTTPMiddleware · Tracer / Meter"| otel
            runtime -->|"Go runtime metrics"| otel
            app -->|"trace_id / span_id"| logger
        end
    end

    subgraph collector_layer["OpenTelemetry Collector"]
        otlp_receiver["OTLP receiver<br/>gRPC / HTTP"]
        log_receiver["Log receivers<br/>filelog / journald"]
        hostmetrics_receiver["hostmetrics receiver"]
        processors["Processors<br/>resource enrichment / <br/> filtering / batching / sampling"]
        exporters["Exporters"]

        otlp_receiver --> processors
        log_receiver --> processors
        hostmetrics_receiver --> processors
        processors --> exporters
    end

    subgraph backend_layer["Observability backends"]
        openobserve["OpenObserve"]
        logs_backend["Loki / Elasticsearch"]
        traces_backend["Tempo / Jaeger"]
        metrics_backend["VictoriaMetrics / Prometheus"]
    end

    otel -->|"OTLP traces + metrics"| otlp_receiver
    logger -->|"JSON logs"| log_receiver
    host_layer -->|"host metrics<br/>CPU / RAM / disk / network"| hostmetrics_receiver
    exporters -->|"logs + metrics + traces"| openobserve
    exporters -.->|"logs"| logs_backend
    exporters -.->|"traces"| traces_backend
    exporters -.->|"metrics"| metrics_backend

    classDef app fill:#eaf4ff,stroke:#3b82f6,color:#0f172a
    classDef collector fill:#fff7ed,stroke:#f97316,color:#0f172a
    classDef backend fill:#ecfdf5,stroke:#10b981,color:#0f172a

    class app,runtime,logger,otel app
    class otlp_receiver,log_receiver,hostmetrics_receiver,processors,exporters collector
    class openobserve,logs_backend,traces_backend,metrics_backend backend

    style host_layer fill:none,stroke:#64748b,stroke-width:2px
    style observability_layer fill:none,stroke:#3b82f6,stroke-width:1.5px
```
