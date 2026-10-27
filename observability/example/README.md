# observability example

Один пример связывает HTTP, gRPC, Core NATS и PostgreSQL. Приложение запускается
четырьмя процессами с режимами `http-client`, `http-gateway`, `grpc-server` и `nats-worker`.

## Запуск

```sh
make up
make requests
```

`make up` собирает приложения, запускает NATS, PostgreSQL, Collector, OpenObserve
и NATS exporter. Команда ждёт готовности OpenObserve. `make requests` запускает HTTP client,
который отправляет один запрос `/demo` через всю цепочку и получает
`200 OK` с телом `ok` после того, как worker выполнит SQL-запрос к PostgreSQL
и отправит reply.

## Что проверить

Откройте [OpenObserve](http://localhost:5080). Логин и пароль находятся в `.env`
и выводятся при `make up`. В одном trace видна вся цепочка:

```text
HTTP client → HTTP gateway → gRPC server → NATS worker → PostgreSQL
```

Проверить baggage отдельным запросом:

```sh
curl -H 'baggage: job.id=calculation-42' http://localhost:8080/demo
```

Worker запишет `job_id=calculation-42` в лог.

Collector отправляет в OpenObserve traces, JSON-логи, HTTP/gRPC и Go runtime
метрики, счётчик `demo.custom.requests`, метрики запросов `db.client.operation.*`
и пула `pgxpool.*`, системные метрики и метрики NATS exporter.
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

- [http_client.go](http_client.go) подключает `HTTPClientTrace` для детализации HTTP-запроса.
- [grpc_server.go](grpc_server.go) отправляет NATS request.
- [nats_worker.go](nats_worker.go) обрабатывает сообщение, выполняет SQL-запрос и отправляет reply.
- [flow_test.go](flow_test.go) проверяет связи spans, baggage и `503` без worker.
- [Makefile.env](Makefile.env) содержит настройки; [docker-compose.yml](docker-compose.yml) задаёт адреса сервисов стенда.
- [Makefile](Makefile) содержит команды сборки и генерации. Go-код из protobuf уже в репозитории; для запуска `protoc` не нужен.
