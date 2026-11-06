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

Пакет для сборки сервиса, запускающего воркеры.

## Использование

```go
err := server.Run(ctx, httpService.Run, grpcService.Run, backgroundWorker)
```

Воркеры имеют тип `func(context.Context) error` и запускаются параллельно.
SIGINT/SIGTERM, отмена контекста или первая ошибка отменяют общий контекст.
`Run` ждёт завершения всех воркеров и возвращает первую ошибку.
Возврат `nil` завершает только текущий воркер. При штатной отмене воркер
должен завершить работу и вернуть `nil`.

HTTP API и настройки перенесены в [server/http](http), gRPC доступен в
[server/grpc](grpc). Пример совместного запуска: [server/example](example).
