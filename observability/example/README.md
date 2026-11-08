# observability example

Демо связывает HTTP → gRPC → NATS → PostgreSQL и отправляет traces, логи и метрики
через OpenTelemetry Collector в OpenObserve.

## Запуск

```sh
make up
make requests
```

Первый `make up` создаёт `.env` и запускает стенд. `make requests` отправляет
запрос через всю цепочку; ожидаемый ответ — `200 OK`, тело `ok`.

## Проверка и остановка

Откройте [OpenObserve](http://localhost:5080), логин и пароль находятся в `.env`.
Проверьте trace всей цепочки, логи и метрики: HTTP-ответ сам по себе не подтверждает
доставку telemetry.

```sh
make logs       # логи Collector
make down       # остановить стенд
make full-down  # остановить и удалить локальные данные OpenObserve
```
