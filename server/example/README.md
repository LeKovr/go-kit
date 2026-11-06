# Один сервис, два транспорта

HTTP и gRPC вызывают одну `service.Health(ctx)`, возвращающую `ok`.
Адаптеры находятся в [handlers.go](handlers.go), запуск через `server.Run` в [main.go](main.go).
Protobuf-код уже сгенерирован в `api/`.

Из каталога `server/example`:

```sh
go run .
```

В другом терминале, из того же каталога:

```sh
curl --fail http://127.0.0.1:8080/health
grpcurl -plaintext -proto api/example.proto -d '{}' 127.0.0.1:9090 example.v1.ExampleService/Health
# Оба вызова возвращают {"status":"ok"}.
```

Настройки доступны через `--help`. SIGINT/SIGTERM запускает остановку обоих транспортов.
Пример запускается без TLS.

Перегенерация, если установлены `protoc`, `protoc-gen-go` и `protoc-gen-go-grpc`:

```sh
protoc --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. api/example.proto
```
