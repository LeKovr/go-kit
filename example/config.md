
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

### Server Options {#srv}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| srv.listen           | SRV_LISTEN           | string | `:8080` | Addr and port which server listens at |
| srv.maxheader        | -                    | int |  | MaxHeaderBytes |
| srv.rto              | -                    | time.Duration | `10s` | HTTP read timeout |
| srv.wto              | -                    | time.Duration | `60s` | HTTP write timeout |
| srv.grace            | -                    | time.Duration | `10s` | Stop grace period |
| srv.ip_header        | SRV_IP_HEADER        | string | `X-Real-IP` | HTTP Request Header for remote IP |
| srv.user_header      | SRV_USER_HEADER      | string | `X-Username` | HTTP Request Header for username |

### HTTPS Options {#srv.tls}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| srv.tls.cert         | SRV_TLS_CERT         | string |  | CertFile for serving HTTPS instead HTTP |
| srv.tls.key          | SRV_TLS_KEY          | string |  | KeyFile for serving HTTPS instead HTTP |
| srv.tls.no-check     | -                    | bool | `false` | disable tls certificate validation |

### Version response Options {#srv.vr}

| Name | ENV | Type | Default | Description |
|------|-----|------|---------|-------------|
| srv.vr.prefix        | -                    | string | `/js/version.js` | URL for version response |
| srv.vr.format        | -                    | string | `document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n` | Format string for version response |
| srv.vr.ctype         | -                    | string | `text/javascript` | js code Content-Type header |
