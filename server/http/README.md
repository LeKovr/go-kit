# go-kit/server/http

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/server/http.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server/http
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/server/http
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/server/http
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/http/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=server&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет для сборки HTTP-сервера.

## Использование

```go
import httpserver "github.com/LeKovr/go-kit/server/http"

srv := httpserver.New(cfg.Server).WithStatic(files).WithVersion(version)
srv.Use(middleware)
err := srv.Run(ctx)
```

Defaults применяет `config.Open`. `Run` допускает один запуск и закрывает listener.
Для обработки SIGINT/SIGTERM используйте `server.Run(ctx, srv.Run)`.

При отмене контекста или ошибке обслуживания сервер ждёт запросы в пределах
`GracePeriod`; по истечении срока принудительно закрывает соединения и возвращает ошибку.
Ноль означает немедленную остановку, отрицательное значение запрещено.
Обработчик, игнорирующий отмену контекста, может продолжить работу после возврата `Run`.

Использование пакета добавляет в сервис следующие опции:

```
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
      --srv.etag_max_body_bytes= Maximum response body size buffered for ETag; non-positive means 1 MiB (default: 1048576) [$SRV_ETAG_MAX_BODY_BYTES]

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
