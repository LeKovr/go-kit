# Changelog

## [0.17.0] - 2026-11-26

- fix: security (f505dc7)
- upd: transport/* -> server/* (303e167)
- server: единый запуск и остановка воркеров (#17) (5fb7f42)
- server: разделить запуск воркеров и HTTP/gRPC-транспорты (#16) (0d9837b)

## [0.16.3] - 2026-08-28

- fix: etag, grace (e19fc5e)

## [0.16.2] - 2026-06-19

- sync and update module dependencies (af34cc6)
- observability: add OpenTelemetry package (0aef4c7)
- upd: set go ver 1.25 (632d2c1)

## [0.16.1] - 2026-05-13

- fix: process ACCESS_LOG as noted in help (53dbee3)
- upd: set go ver 1.23 (e07ccd0)
- upd: docs (d031121)

## [0.16.0] - 2026-02-17

- add: * config: ReadHeaderTimeout, IdleTimeout, AccessLog, UseETag * service: WithWorkers, WithNWorkers, WithHTTPWorkers (26c81d0)
- fix: test lint (5e6c592)
- add: docs (19a85eb)
- add: server tests & example (8100e89)

## [0.15.2] - 2025-04-24

- upd: clean debug (95fd79f)

## [0.15.1] - 2025-04-24

- fix: RunWorkers (d746fa5)

## [0.15.0] - 2025-04-22

- add: RunWorkers (6598f9f)
- upd: deps versions (f721be3)

## [0.14.0] - 2024-07-04

- add: ENV{LISTEN}, rm syscall.SIGINT, it eq to os.Interrupt (fefd3db)
- upd: rm old doc (c16c5d5)

## [0.13.0] - 2024-04-08

- add: HTTPS; upd: move version resp to vr namespace (17264b0)

## [0.12.24] - 2024-01-02

- upd: linter fixes (893b3a7)
- upd: VersionCType, net.ErrClosed (5ef7e60)

## [0.12.23] - 2023-12-23

- fix: badges (6e654ae)
- upd: badges (c119eb4)
- add MVP (d2ebca0)
