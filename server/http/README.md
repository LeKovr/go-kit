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

При отмене контекста или ошибке обслуживания HTTP-сервер ждёт завершения
активных запросов в пределах `GracePeriod`; по истечении срока принудительно закрывает соединения и возвращает ошибку.
Ноль означает немедленную остановку, отрицательное значение запрещено.
Обработчик, игнорирующий отмену контекста, может продолжить работу после возврата `Run`.

Использование пакета добавляет в сервис следующие опции:

```
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
