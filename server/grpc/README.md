# go-kit/server/grpc

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/server/grpc.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server/grpc
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/server/grpc
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/server/grpc
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/grpc/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=server/grpc&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет для сборки gRPC-сервера.

## Использование

```go
import grpcserver "github.com/LeKovr/go-kit/server/grpc"

srv := grpcserver.New(cfg.GRPC)
api.RegisterMyServiceServer(srv.Server(), implementation)
err := srv.Run(ctx)
```

`New` принимает `grpc.ServerOption` для TLS, interceptors и stats handlers.
Defaults применяет `config.Open`.

Использование этого пакета добавляет в сервис следующие опции:

```
gRPC Options:
      --grpc.listen=  Addr and port which gRPC server listens at (default: :9090) [$GRPC_LISTEN]
```

`Run` допускает один запуск и закрывает listener. При отмене контекста выполняется
принудительный `Stop`. `Shutdown(ctx)` ждёт завершения активных RPC до истечения
переданного контекста, затем вызывает `Stop`.
Обработчик, игнорирующий отмену контекста, может задержать возврат `Run`.

Для обработки SIGINT/SIGTERM используйте [server](../../server)
с `WithShutdown(srv.Shutdown)`. Совместный запуск с HTTP показан в [примере](../../example).
