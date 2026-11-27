# go-kit/server

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/server.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=server&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет для сборки сервиса, запускающего воркеры.

## Использование

```go
srv := server.New(cfg.Server).
    WithShutdown(httpSrv.Shutdown, grpcSrv.Shutdown)

err := srv.Run(ctx, httpSrv.Run, grpcSrv.Run, backgroundWorker)
```

`cfg.Server` имеет тип `server.Config`. Воркеры имеют тип `func(context.Context) error`
и запускаются параллельно. Для воркеров, которым достаточно отмены контекста,
`WithShutdown` не нужен.

SIGINT/SIGTERM, отмена внешнего контекста, ошибка воркера или завершение всех воркеров
запускают остановку. Функции из `WithShutdown` вызываются параллельно; после их
завершения или истечения `GracePeriod` отменяется контекст воркеров.
`Run` ждёт завершения всех функций и объединяет ошибки, даже если срок уже истёк.
Возврат `nil` завершает только текущий воркер. При штатной отмене воркер
должен завершить работу и вернуть `nil`.

`config.Open` задаёт `GracePeriod` по умолчанию `10s`; в ручном конфиге задайте его явно.
БД и клиентов закрывайте после возврата `Run`.

HTTP и gRPC реализованы в отдельных пакетах: [server/http](./http) и
[server/grpc](./grpc). Их совместный запуск показан в [примере](../example).
