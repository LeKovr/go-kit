# Архитектура observability

Схема показывает путь observability-сигналов от Go-сервиса до backend-ов хранения и анализа.

Пакет `observability` отвечает за Go-side часть: настройку OpenTelemetry SDK, `otelhttp` instrumentation, Go runtime metrics, custom `Tracer`/`Meter` и OTLP export traces/metrics.

Пайплайны Collector, host metrics, сбор логов, хранение данных и маршрутизация в observability backend-ы относятся к инфраструктурному слою и настраиваются вне Go-пакета.

```mermaid
flowchart TB
    subgraph app_layer["Go service"]
        direction TB
        app["Application code"]
        runtime["Go runtime"]
        otel["OpenTelemetry SDK<br/>traces and metrics"]
        logger["Logger slog"]

        app --> |"otelhttp traces and metrics<br> + custom business metrics"| otel
        runtime -->|"runtime metrics:<br/>memory / GC / goroutines / scheduler"| otel
        app --> logger
    end

    subgraph runtime_layer["Runtime / Host environment"]
        direction TB
        otlp["OTLP endpoint<br/>gRPC / HTTP"]
        stdout["stdout / stderr"]
        log_source["Log source<br/>container logs / journald / filelog"]
        host["Host / container runtime"]

        stdout --> log_source
    end

    subgraph collector_layer["OpenTelemetry Collector"]
        direction TB
        receivers["Receivers<br/>otlp / filelog / journald / hostmetrics"]
        processors["Processors<br/>resource detection / attributes / memory_limiter / batch / filter / sampling"]
        exporters["Exporters"]

        receivers --> processors
        processors --> exporters
    end

    subgraph backend_layer["Observability backend"]
        direction TB
        openobserve["OpenObserve<br/>logs + metrics + traces"]
        other["Alternative backends<br/>Loki / Tempo / VictoriaMetrics / Prometheus / Datadog / ..."]
    end

    otel -->|"OTLP traces and metrics"| otlp
    logger -->|"structured JSON logs with<br/>trace_id / span_id"| stdout
    otlp --> receivers
    log_source --> receivers
    host -->|"host metrics<br/>CPU / RAM / disk / network"| receivers
    exporters --> openobserve
    exporters -.-> other

    classDef app fill:#eaf4ff,stroke:#3b82f6,color:#0f172a
    classDef boundary fill:#f8fafc,stroke:#64748b,color:#0f172a
    classDef collector fill:#fff7ed,stroke:#f97316,color:#0f172a
    classDef backend fill:#ecfdf5,stroke:#10b981,color:#0f172a

    class app,otel,runtime,logger app
    class otlp,stdout,log_source,host boundary
    class receivers,processors,exporters collector
    class openobserve,other backend
```
