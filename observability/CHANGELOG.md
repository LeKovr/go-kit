# Changelog

## [0.4.1] - 2026-11-27

- upd: goreportcard.com retired (5694f0d)

## [0.4.0] - 2026-11-26

- fix: security fixes (f505dc7)
- upd: transport/* -> server/* (303e167)
- observability/example: исправить остановку NATS-воркера (#18) (7945aea)
- server: единый запуск и остановка воркеров (#17) (5fb7f42)
- observability: передавать опции otelpgx в PGXTracer (#19) (ac89245)
- server: разделить запуск воркеров и HTTP/gRPC-транспорты (#16) (0d9837b)
- upd: google.golang.org/grpc v1.83.1 (7e57084)

## [0.3.0] - 2026-11-02

- refs #14, observability: поддержка pgx и детализация HTTP-клиента (#14) (a51e018)
- i15-добавить sampling, публичный HTTP-вход и параллельный shutdown (#15) (5d78b3d)
- observability: поддержка Core NATS и JetStream (#13) (3d40c69)
- observability: поддержка gRPC и HTTP-клиента (#12) (b80aad3)
- observability: уточнить архитектуру и актуализировать semconv (#11) (0eba901)
- upd: grpc v1.82.1 (8689205)
- upd: rm extra deps (673208f)
- slogger: add OpenTelemetry scope to logs (#10) (05aac4a)
- upd architecture doc (239416c)
- observability: make example Makefile cross-platform (766a0bb)

## [0.2.0] - 2026-06-26

- observability: add filelog collector for example logs (12ff792)
- observability: added example logs (812ceab)
- observability: update README (062d45c)
- observability: use service propagator in HTTP middleware (1fce9b2)
- observability: upd example docs (e72477b)
- observability: add example server and client modes (9ca64f6)
- observability: use local noop providers (536214e)
- observability: upd go.mod (c90b6a4)
- observability: upd example docs (d1f23d9)
- observability: added castom traces and metrics (4cb0b32)

## [0.1.0] - 2026-06-19

- upd: go mod tidy (98faf00)
- added Makefile.env (c0550fe)
- docs: add telemetry architecture diagram (2443de0)
- observability: add OpenTelemetry package (0aef4c7)
