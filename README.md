# go-kit

Set of usefull golang packages

[gra1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=logger/*
[gra2]: https://pkg.go.dev/github.com/LeKovr/go-kit/logger
[grb1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/*
[grb2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server
[grc1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=config/*
[grc2]: https://pkg.go.dev/github.com/LeKovr/go-kit/config
[grd1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=slogger/*
[grd2]: https://pkg.go.dev/github.com/LeKovr/go-kit/slogger
[gre1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=ver/*
[gre2]: https://pkg.go.dev/github.com/LeKovr/go-kit/ver
[grf1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=observability/*
[grf2]: https://pkg.go.dev/github.com/LeKovr/go-kit/observability
[grh1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/http/*
[grh2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server/http
[grr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/grpc/*
[grr2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server/grpc

## Пакеты

| Модуль | Назначение | Релиз |
|---|---|---|
| [config](config/) | конфиг из env/flags/файла, `config.Open`, генератор дефиниций | [![config][grc1]][grc2] |
| [server](server/) | координация воркеров, единый `Run`, graceful shutdown | [![server][grb1]][grb2] |
| [server/http](server/http/) | HTTP-сервер: TLS, access log, ETag, version response, drain | [![server/http][grh1]][grh2] |
| [server/grpc](server/grpc/) | gRPC-сервер: Serve/Shutdown, drain RPC | [![server/grpc][grr1]][grr2] |
| [slogger](slogger/) | slog-совместимый логгер: файл/stdout, формат, `NewContext`/`FromContext`, scope | [![slogger][grd1]][grd2] |
| [observability](observability/) | OTel-телеметрия: middleware, HTTP/gRPC/NATS/pgx-хендлеры | [![observability][grf1]][grf2] |
| [ver](ver/) | проверка версии приложения | [![ver][gre1]][gre2] |
| [logger](logger/) | **deprecated** — legacy-обёртка логгинга (zerolog); используйте `slogger` | [![logger][gra1]][gra2] |

## Примеры

| Пример | Что это | Как запустить |
|---|---|---|
| [config/example](config/example) | минимальный вызов `config.Open`, генерация дефиниций | `go run .` из каталога |
| [example](example) | HTTP+gRPC сервис, health, Makefile-сборка | `go run . --root=.`; см. [example/README.md](example/README.md) |
| [observability/example](observability/example) | связка HTTP→gRPC→NATS→pgx, OTel Collector → OpenObserve, 5 режимов | `make up` / `make requests`; см. [observability/example/README.md](observability/example/README.md) |

Запуск и проверка (общие для всех трёх): `go run`, `curl /health`, `grpc_health_probe`, OTel-флаги — описаны в [example/README.md](example/README.md).

## Лицензия

Copyright (c) 2022 Aleksei Kovrizhkin <lekovr+gokit@gmail.com>

Исходный код проекта лицензирован под Apache License, Version 2.0 (the "[License](LICENSE)");
