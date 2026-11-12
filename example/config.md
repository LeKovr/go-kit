
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
| http.listen           | HTTP_LISTEN           | string | `:8080` | Addr and port which server listens at |
| http.maxheader        | -                    | int |  | MaxHeaderBytes |
| http.rto              | -                    | time.Duration | `10s` | HTTP read timeout |
| http.wto              | -                    | time.Duration | `60s` | HTTP write timeout, '0' means disable |
| http.rhto             | -                    | time.Duration | `10s` | HTTP read header timeout |
| http.ito              | -                    | time.Duration | `10s` | HTTP idle timeout |
| http.grace            | -                    | time.Duration | `10s` | Stop grace period |
| http.ip_header        | HTTP_IP_HEADER        | string | `X-Real-IP` | HTTP Request Header for remote IP |
| http.user_header      | HTTP_USER_HEADER      | string | `X-Username` | HTTP Request Header for username |
| http.access_log       | HTTP_ACCESS_LOG       | string |  | HTTP access log filename (default: STDOUT, '-' means disable) |
| http.etag             | HTTP_ETAG             | bool | `false` | Add ETAG in HTTP response |
| http.etag_max_body_bytes | HTTP_ETAG_MAX_BODY_BYTES | int | `1048576` | Maximum response body size buffered for ETag; non-positive means 1 MiB |

### HTTPS Options {#http.tls}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| http.tls.cert         | HTTP_TLS_CERT         | string |  | CertFile for serving HTTPS instead HTTP |
| http.tls.key          | HTTP_TLS_KEY          | string |  | KeyFile for serving HTTPS instead HTTP |
| http.tls.no-check     | -                    | bool | `false` | disable tls certificate validation |

### Version response Options {#http.vr}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| http.vr.prefix        | -                    | string | `/js/version.js` | URL for version response |
| http.vr.format        | -                    | string | `document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n` | Format string for version response |
| http.vr.ctype         | -                    | string | `text/javascript` | js code Content-Type header |
