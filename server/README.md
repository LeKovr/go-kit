# go-kit/server

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/server.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/server
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/server
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=server&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет для сборки сервиса, запускающего воркеры и, если нужен, HTTP-сервер.

## API

HTTP и gRPC находятся в отдельных модулях [server/http](http) и
[server/grpc](grpc). Воркеры запускаются через `server.Run`:

```go
httpSrv := httpserver.New(cfg.HTTP)
grpcSrv := grpcserver.New(cfg.GRPC)
err := server.Run(ctx, httpSrv.Run, grpcSrv.Run, backgroundWorker)
```

`Run` запускает воркеры параллельно, отменяет общий контекст при сигнале,
отмене родителя или первой ошибке и ждёт всех воркеров. Он возвращает первую
ошибку без изменений; при штатной отмене воркер возвращает `nil`.
Если все воркеры завершились успешно, `Run` возвращается.
Пример: [server/example](example).

Прежний объектный API сохранён в `legacy.go` и помечен `Deprecated:`:

```go
err := server.New(cfg.Server).WithWorkers(backgroundWorker).WithShutdown(cleanup).Run(ctx)
```

Его сигнатуры, поля конфигурации, CLI/ENV-теги и смысл нулевых значений сохранены.
HTTP-типы являются aliases типов `server/http`. Старый `Run` включает HTTP,
а `RunWorkers` сам HTTP не добавляет и ждёт отмену даже после конечных воркеров.
`WithHTTPWorkers` регистрирует HTTP один раз. Callback вызывается параллельно
с остановкой воркеров, с сохранёнными значениями контекста и новым таймаутом
`GracePeriod`; возвращается первая ошибка. Только legacy подавляет
`context.Canceled` и его обычные обёртки при отмене; составные ошибки и `context.DeadlineExceeded`
сохраняются.

При миграции cleanup можно вызвать после `Run`, если воркеры его не ждут.
Иначе нужен отдельный воркер, ожидающий `ctx.Done()`, со своим grace-контекстом.
HTTP использует [один запуск и graceful drain](http#использование).
Старый `Run` открывает порт до запуска воркеров. Подготовка access log и TLS
выполняется внутри HTTP-воркера, поэтому её ошибки могут возникнуть позже.

Локальная зависимость `server/http v0.0.0` разрешается через `go.work`.
Для `GOWORK=off` используйте временный modfile или consumer с локальными
`replace` обоих модулей. До upstream-релиза нужно опубликовать HTTP-модуль,
указать его реальную версию в зависимости и проверить потребителя без HTTP `replace`.

## Использование

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
