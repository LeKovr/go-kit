# Пример использования пакетов github.com/LeKovr/go-kit

* main.go - пример сервиса
* Makefile - пример Makefile с командами для сборки
* Makefile.env - описание настроек для включения в Makefile деплоя
* config.md - документация по настройкам

## Использование

Из каталога `example`:

```sh
go run . --root=.
```

HTTP раздаёт статику и отвечает `ok` на `/health`, gRPC предоставляет стандартный health-сервис.

```sh
curl --fail http://127.0.0.1:8080/health
# Для gRPC нужен установленный grpc_health_probe:
grpc_health_probe -addr=127.0.0.1:9090
```

Экспорт traces и metrics по умолчанию выключен. Для отправки в OTLP Collector:

```sh
go run . --root=. --otel.enable_traces --otel.enable_metrics
```

Настройки описаны в [config.md](config.md), файл обновляется при `make build`.
