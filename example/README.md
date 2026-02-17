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

Server Options:
      --srv.listen=              Addr and port which server listens at (default: :8080) [$SRV_LISTEN]
      --srv.maxheader=           MaxHeaderBytes
      --srv.rto=                 HTTP read timeout (default: 10s)
      --srv.wto=                 HTTP write timeout, '0' means disable (default: 60s)
      --srv.rhto=                HTTP read header timeout (default: 10s)
      --srv.ito=                 HTTP idle timeout (default: 10s)
      --srv.grace=               Stop grace period (default: 10s)
      --srv.ip_header=           HTTP Request Header for remote IP (default: X-Real-IP) [$SRV_IP_HEADER]
      --srv.user_header=         HTTP Request Header for username (default: X-Username) [$SRV_USER_HEADER]
      --srv.access_log=          HTTP access log filename (default: STDOUT, '-' means disable) [$SRV_ACCESS_LOG]
      --srv.etag                 Add ETAG in HTTP response [$SRV_ETAG]

HTTPS Options:
      --srv.tls.cert=            CertFile for serving HTTPS instead HTTP [$SRV_TLS_CERT]
      --srv.tls.key=             KeyFile for serving HTTPS instead HTTP [$SRV_TLS_KEY]
      --srv.tls.no-check         disable tls certificate validation

Version response Options:
      --srv.vr.prefix=           URL for version response (default: /js/version.js)
      --srv.vr.format=           Format string for version response (default: "document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n")
      --srv.vr.ctype=            js code Content-Type header (default: text/javascript)

Help Options:
  -h, --help                     Show this help message
```
