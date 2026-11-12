# Пример использования пакетов github.com/LeKovr/go-kit

* main.go - пример сервиса
* Makefile - пример Makefile с командами для сборки
* Makefile.env - описание настроек для включения в Makefile деплоя
* config.md - документация по настройкам

## Использование

Использование пакетов добавляет в сервис следующие опции:

```
$ ./example -h
Usage:
  example [OPTIONS]

Application Options:
      --root=                    Static files root directory [$ROOT]
      --version                  Show version and exit
      --config_gen=[|json|md|mk] Generate and print config definition in given format and exit (default: '', means skip) [$CONFIG_GEN]
      --config_dump=             Dump config dest filename [$CONFIG_DUMP]

Logging Options:
      --log.debug                Show debug info [$LOG_DEBUG]
      --log.format=[|text|json]  Output format (default: '', means use text if DEBUG) [$LOG_FORMAT]
      --log.time_format=         Time format for text output (default: 2006-01-02 15:04:05.000) [$LOG_TIME_FORMAT]
      --log.dest=                Log destination (default: '', means STDERR) [$LOG_DEST]

HTTP Options:
      --http.listen=              Addr and port which server listens at (default: :8080) [$HTTP_LISTEN]
      --http.maxheader=           MaxHeaderBytes
      --http.rto=                 HTTP read timeout (default: 10s)
      --http.wto=                 HTTP write timeout, '0' means disable (default: 60s)
      --http.rhto=                HTTP read header timeout (default: 10s)
      --http.ito=                 HTTP idle timeout (default: 10s)
      --http.grace=               Stop grace period (default: 10s)
      --http.ip_header=           HTTP Request Header for remote IP (default: X-Real-IP) [$HTTP_IP_HEADER]
      --http.user_header=         HTTP Request Header for username (default: X-Username) [$HTTP_USER_HEADER]
      --http.access_log=          HTTP access log filename (default: STDOUT, '-' means disable) [$HTTP_ACCESS_LOG]
      --http.etag                 Add ETAG in HTTP response [$HTTP_ETAG]
      --http.etag_max_body_bytes= Maximum response body size buffered for ETag; non-positive means 1 MiB (default: 1048576) [$HTTP_ETAG_MAX_BODY_BYTES]

HTTPS Options:
      --http.tls.cert=            CertFile for serving HTTPS instead HTTP [$HTTP_TLS_CERT]
      --http.tls.key=             KeyFile for serving HTTPS instead HTTP [$HTTP_TLS_KEY]
      --http.tls.no-check         disable tls certificate validation

Version response Options:
      --http.vr.prefix=           URL for version response (default: /js/version.js)
      --http.vr.format=           Format string for version response (default: "document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n")
      --http.vr.ctype=            js code Content-Type header (default: text/javascript)

Help Options:
  -h, --help                     Show this help message
```
