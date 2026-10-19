# Архитектура observability

Схема показывает путь observability-сигналов от Go-сервиса до backend-ов хранения и анализа.

Пакет `observability` отвечает за Go-side часть: настройку OpenTelemetry SDK, HTTP мониторинг через `otelhttp`, сбор метрик `Go runtime`, `custom Tracer/Meter` и экспорт `traces/metrics` по OTLP.

`slogger` пишет JSON-логи с `trace_id`/`span_id` в `stderr` или файл, а Collector читает их через `filelog` или `journald`

Сбор `host metrics` и логов, обработка сигналов и маршрутизация в один или несколько backend-ов относятся к инфраструктурному слою и настраиваются в конфигурации Collector.

```mermaid
flowchart TB
    subgraph app_layer["Go service process"]
        app("Application code")
        runtime("Go runtime")
        logger("go-kit/slogger")

        subgraph observability_layer["go-kit/<br/>observability"]
            otel("OpenTelemetry SDK")
        end

        app -->|"logs + context"| logger
        app -->|"spans + metrics"| otel
        runtime -->|"runtime metrics"| otel
    end

    host("Host resources<br/>CPU · RAM · disk · network")

    subgraph collector_layer["OpenTelemetry<br/>Collector"]
        log_receiver("Log receivers<br/>filelog / journald")
        otlp_receiver("OTLP receiver<br/>gRPC / HTTP")
        hostmetrics_receiver("Host metrics receiver")
        processors("Processors<br/>resource enrichment<br/>filtering · batching<br/>sampling")
        exporters("Exporters")

        log_receiver --> processors
        otlp_receiver --> processors
        hostmetrics_receiver --> processors
        processors --> exporters
    end

    subgraph backend_layer["Backends"]
        openobserve("OpenObserve")
        logs_backend("Loki / Elasticsearch")
        traces_backend("Tempo / Jaeger")
        metrics_backend("VictoriaMetrics / Prometheus")
    end

    logger -->|"JSON logs"| log_receiver
    otel -->|"traces + metrics"| otlp_receiver
    host -->|"host metrics"| hostmetrics_receiver
    exporters -->|"logs · metrics · traces"| openobserve
    exporters -.->|"logs"| logs_backend
    exporters -.->|"traces"| traces_backend
    exporters -.->|"metrics"| metrics_backend

    classDef app fill:#eaf4ff,stroke:#3b82f6,color:#0f172a,stroke-width:1px
    classDef collector fill:#fff7ed,stroke:#ea580c,color:#0f172a,stroke-width:1px
    classDef backend fill:#ecfdf5,stroke:#059669,color:#0f172a,stroke-width:1px
    classDef host fill:#f1f5f9,stroke:#64748b,color:#0f172a,stroke-width:1px

    class app,runtime,logger,otel app
    class host host
    class otlp_receiver,log_receiver,hostmetrics_receiver,processors,exporters collector
    class openobserve,logs_backend,traces_backend,metrics_backend backend

    style app_layer fill:none,stroke:#64748b,stroke-width:1px,rx:10,ry:10
    style observability_layer fill:none,stroke:#3b82f6,stroke-width:1.5px,rx:8,ry:8
    style collector_layer fill:none,stroke:#ea580c,stroke-width:1px,rx:12,ry:12
    style backend_layer fill:none,stroke:#059669,stroke-width:1px,rx:12,ry:12
```
