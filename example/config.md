
### Main Options

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| root                 | ROOT                 | string |  | Static files root directory |
| version              | -                    | bool | `false` | Show version and exit |
| config_gen           | CONFIG_GEN           | ,json,md,mk |  | Generate and print config definition in given format and exit (default: '', means skip) |
| config_dump          | CONFIG_DUMP          | string |  | Dump config dest filename |

### Logging Options {#log}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| log.debug            | LOG_DEBUG            | bool | `false` | Show debug info |
| log.format           | LOG_FORMAT           | ,text,json |  | Output format (default: '', means use text if DEBUG) |
| log.time_format      | LOG_TIME_FORMAT      | string | `2006-01-02 15:04:05.000` | Time format for text output |
| log.dest             | LOG_DEST             | string |  | Log destination (default: '', means STDERR) |

### HTTP Options {#http}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| http.listen          | HTTP_LISTEN          | string | `:8080` | Addr and port which server listens at |
| http.maxheader       | -                    | int |  | MaxHeaderBytes |
| http.rto             | -                    | time.Duration | `10s` | HTTP read timeout |
| http.wto             | -                    | time.Duration | `60s` | HTTP write timeout, '0' means disable |
| http.rhto            | -                    | time.Duration | `10s` | HTTP read header timeout |
| http.ito             | -                    | time.Duration | `10s` | HTTP idle timeout |
| http.ip_header       | HTTP_IP_HEADER       | string | `X-Real-IP` | HTTP Request Header for remote IP |
| http.user_header     | HTTP_USER_HEADER     | string | `X-Username` | HTTP Request Header for username |
| http.access_log      | HTTP_ACCESS_LOG      | string |  | HTTP access log filename (default: STDOUT, '-' means disable) |
| http.etag            | HTTP_ETAG            | bool | `false` | Add ETAG in HTTP response |
| http.etag_max_body_bytes | HTTP_ETAG_MAX_BODY_BYTES | int | `1048576` | Maximum response body size buffered for ETag; non-positive means 1 MiB |

### HTTPS Options {#http.tls}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| http.tls.cert        | HTTP_TLS_CERT        | string |  | CertFile for serving HTTPS instead HTTP |
| http.tls.key         | HTTP_TLS_KEY         | string |  | KeyFile for serving HTTPS instead HTTP |
| http.tls.no-check    | -                    | bool | `false` | disable tls certificate validation |

### Version response Options {#http.vr}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| http.vr.prefix       | -                    | string | `/js/version.js` | URL for version response |
| http.vr.format       | -                    | string | `document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n` | Format string for version response |
| http.vr.ctype        | -                    | string | `text/javascript` | js code Content-Type header |

### gRPC Options {#grpc}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| grpc.listen          | GRPC_LISTEN          | string | `:9090` | Addr and port which gRPC server listens at |

### Server Options {#server}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| server.grace         | SERVER_GRACE         | time.Duration | `10s` | Shutdown timeout |

### OpenTelemetry Options {#otel}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| otel.exporter_otlp_endpoint | OTEL_EXPORTER_OTLP_ENDPOINT | string | `http://localhost:4317` | OTLP gRPC endpoint URL |
| otel.service_instance_id | OTEL_SERVICE_INSTANCE_ID | string |  | Unique service instance id, e.g. pod name, pod UID, hostname, or container id |
| otel.metric_interval | OTEL_METRIC_INTERVAL | time.Duration | `10s` | OTLP metrics export interval |
| otel.shutdown_timeout | OTEL_SHUTDOWN_TIMEOUT | time.Duration | `5s` | OpenTelemetry shutdown timeout |
| otel.trace_sample_percent | OTEL_TRACE_SAMPLE_PERCENT | int | `100` | Percentage of new root traces to sample, from 0% to 100% |
| otel.enable_traces   | OTEL_ENABLE_TRACES   | bool | `false` | Enable traces export |
| otel.enable_metrics  | OTEL_ENABLE_METRICS  | bool | `false` | Enable metrics export |
| otel.enable_go_runtime_metrics | OTEL_ENABLE_GO_RUNTIME_METRICS | bool | `false` | Enable Go runtime metrics collection |
