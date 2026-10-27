# observability example

Один пример связывает HTTP, gRPC и Core NATS. Приложение запускается четырьмя
процессами с режимами `http-client`, `http-gateway`, `grpc-server` и `nats-worker`.

## Запуск

```sh
make up
make requests
```

`make up` собирает приложения, запускает NATS, Collector, OpenObserve и NATS exporter.
Команда ждёт готовности OpenObserve. `make requests` запускает HTTP client,
который отправляет один запрос `/demo` через всю цепочку и получает
`200 OK` с телом `ok`.

## Что проверить

Откройте [OpenObserve](http://localhost:5080). Логин и пароль находятся в `.env`
и выводятся при `make up`. Для запроса ожидается один trace из семи spans:

```text
http-client: HTTP GET
└── http-gateway: GET /demo
    └── http-gateway: demo.Calculator/Calculate
        └── grpc-server: demo.Calculator/Calculate
            └── grpc-server: request demo.calculate
                └── nats-worker: process demo.calculate
                    └── nats-worker: publish _INBOX.*
```

Проверить baggage отдельным запросом:

```sh
curl -H 'baggage: job.id=calculation-42' http://localhost:8080/demo
```

Worker запишет `job_id=calculation-42` в лог.

Collector отправляет в OpenObserve traces, JSON-логи, HTTP/gRPC и Go runtime
метрики, счётчик `demo.custom.requests`, системные метрики и метрики NATS exporter.
Успешный HTTP-ответ подтверждает выполнение запроса; доставку telemetry
проверяйте отдельно в OpenObserve. Логи Collector:

```sh
make logs
```

## Остановка

```sh
make down
```

Для остановки с удалением локальных данных OpenObserve:

```sh
make full-down
```

## Код и настройки

- [grpc_server.go](grpc_server.go) отправляет NATS request.
- [nats_worker.go](nats_worker.go) обрабатывает сообщение и отправляет reply.
- [flow_test.go](flow_test.go) проверяет связи spans, baggage и `503` без worker.
- [Makefile.env](Makefile.env) содержит настройки; [docker-compose.yml](docker-compose.yml) задаёт адреса сервисов стенда.
- [Makefile](Makefile) содержит команды сборки и генерации. Go-код из protobuf уже в репозитории; для запуска `protoc` не нужен.
