# Архитектура observability

Схема показывает путь observability-сигналов от Go-сервиса до backend-ов хранения и анализа.

Пакет `observability` отвечает за Go-side часть: настройку OpenTelemetry SDK, `otelhttp` instrumentation, Go runtime metrics, custom `Tracer`/`Meter` и OTLP export traces/metrics.

Пайплайны Collector, host metrics, сбор логов, хранение данных и маршрутизация в observability backend-ы относятся к инфраструктурному слою и настраиваются вне Go-пакета.

Go-сервис не зависит от observability backend-ов: выбор одного или нескольких назначений определяется конфигурацией Collector.

```mermaid
flowchart TB
    subgraph app_layer["Go service process"]
        app["Application code"]
        runtime["Go runtime"]
        otel["OpenTelemetry SDK<br/>traces and metrics"]
        logger["slogger<br/>slog + slog-otel"]

        app --> |"otelhttp traces and metrics<br/>+ custom business metrics"| otel
        runtime -->|"runtime metrics:<br/>memory / GC / goroutines / scheduler"| otel
        app --> logger
    end

    host["Host / container runtime"]

    subgraph collector_layer["OpenTelemetry Collector"]
        otlp_receiver["OTLP receiver<br/>gRPC / HTTP<br/>traces and metrics"]
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

    otel -->|"OTLP/gRPC traces and metrics"| otlp_receiver
    logger -->|"structured JSON logs with<br/>trace_id / span_id"| log_receiver
    host -->|"host metrics<br/>CPU / RAM / disk / network"| hostmetrics_receiver
    exporters -->|"logs + metrics + traces"| openobserve
    exporters -.->|"logs"| logs_backend
    exporters -.->|"traces"| traces_backend
    exporters -.->|"metrics"| metrics_backend

    classDef app fill:#eaf4ff,stroke:#3b82f6,color:#0f172a
    classDef boundary fill:#f8fafc,stroke:#64748b,color:#0f172a
    classDef collector fill:#fff7ed,stroke:#f97316,color:#0f172a
    classDef backend fill:#ecfdf5,stroke:#10b981,color:#0f172a

    class app,otel,runtime,logger app
    class host boundary
    class otlp_receiver,log_receiver,hostmetrics_receiver,processors,exporters collector
    class openobserve,logs_backend,traces_backend,metrics_backend backend
```
