# observability example

Один пример связывает HTTP, gRPC и Core NATS. Приложение запускается четырьмя
процессами с режимами `http-client`, `http-gateway`, `grpc-server` и `nats-worker`:

```text
HTTP client → HTTP gateway → gRPC server → NATS worker
HTTP client ← HTTP gateway ← gRPC server ← NATS reply
```

## Запуск

Из каталога `observability/example` запустите весь стенд:

```sh
make up
```

Compose собирает example и запускает HTTP gateway, gRPC server, NATS worker,
NATS, exporter, Collector и OpenObserve. Команда ждёт готовности OpenObserve. Нужны Docker и Docker Compose. Go на хосте не требуется.

Выполните демонстрационный запрос:

```sh
make requests
```

Команда запускает HTTP client для `/demo`. Запрос проходит всю цепочку
HTTP → gRPC → NATS → worker и возвращает `200 ok`. Все приложения экспортируют traces, metrics и JSON-логи.

Отдельный запрос без HTTP client span:

```sh
curl -v http://localhost:8080/demo
```

## Traces

Для успешного запроса `MODE=http-client` ожидается один trace из семи spans в четырёх
сервисах. Ниже указаны роли; имена сервисов начинаются с `observability-example-`:

```text
http-client: HTTP GET [CLIENT]
└── http-gateway: GET /demo [SERVER]
    └── http-gateway: demo.Calculator/Calculate [client]
        └── grpc-server: demo.Calculator/Calculate [server]
            └── grpc-server: request demo.calculate
                └── nats-worker: process demo.calculate
                    └── nats-worker: publish _INBOX.*
```

OpenObserve может показывать у исходящих spans имя назначения из `infer_service_name`.
Например, у `HTTP GET` это `http-gateway`; сервис-источник указан в `service_name`:
`observability-example-http-client`.

Request span охватывает отправку и ожидание reply. HTTP, gRPC и NATS используют
providers своего `obs`, без `InstallGlobal()`. Ручных spans в примере нет.
Deadline передаётся через gRPC, но через NATS не передаётся. Отмена HTTP-запроса
не отменяет callback worker.

Baggage проходит всю цепочку; worker записывает `job.id` в поле лога `job_id`:

```sh
curl -v -H 'baggage: job.id=calculation-42' http://localhost:8080/demo
```

## Telemetry

Откройте [OpenObserve](http://localhost:5080). Логин и пароль находятся в `.env`
и выводятся при `make up`. Логи Collector:

```sh
make logs
```

Пример собирает HTTP и gRPC metrics, custom counter `demo.custom.requests`,
Go runtime metrics и системные метрики из Collector `hostmetrics`.
JSON-логи проходят через `error.log` и Collector `filelog` в OpenObserve.

Метрики брокера собирает `prometheus-nats-exporter`:

```text
NATS monitoring :8222 → exporter :7777/metrics → Collector → OpenObserve
```

Collector получает их через Prometheus receiver `prometheus/nats` каждые 10 секунд.
Для входящих сообщений смотрите `gnatsd_varz_in_msgs`, для активных соединений
`gnatsd_varz_connections`. NATS spans создаёт Go-обвязка.

Проверить endpoint exporter:

```sh
curl -fsS http://localhost:7777/metrics
```

Успешный HTTP-ответ проверяет выполнение запроса; появление telemetry проверяйте
в Collector и OpenObserve.

## Настройки

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `GRPC_ADDRESS` | `127.0.0.1:9090` | Адрес gRPC server и подключения из HTTP gateway. |
| `GRPC_TIMEOUT` | `3s` | Таймаут gRPC-вызова из HTTP handler. |
| `NATS_URL` | `nats://127.0.0.1:4222` | Адрес брокера. |
| `NATS_TIMEOUT` | `3s` | Подключение и ожидание NATS reply в gRPC handler. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4317` | OTLP endpoint Collector. |

В Compose gRPC доступен внутри сети как `grpc:9090`, NATS — как `nats:4222`.
gRPC в этом локальном примере работает без TLS.

## Код

| Файл | Назначение |
| --- | --- |
| `main.go` | Конфигурация, выбор режима и observability. |
| `http_client.go` | HTTP client. |
| `http_server.go` | HTTP gateway и вызов gRPC. |
| `grpc_server.go` | gRPC server и NATS request. |
| `nats_worker.go` | Callback со счётчиком и отправкой reply через `obs.NATSClient`; завершение worker. |
| `api/` | `calculate.proto` и generated Go-код. |
| `flow_test.go` | Проверка цепочки, baggage и ошибки без worker. |

## Остановка

Остановите стенд; gRPC server и worker завершают текущую работу в пределах grace period:

```sh
make down
```

Удалить volume с локальными данными OpenObserve:

```sh
make full-down
```

## Генерация gRPC

Generated-файлы сохранены в репозитории, для запуска `protoc` не нужен.
После изменения `api/calculate.proto` установите `protoc` и Go-плагины:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.4
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
make generate
```

После изменения полей конфигурации обновите `Makefile.env`:

```sh
make -B Makefile.env
```
