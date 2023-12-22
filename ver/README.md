# go-kit/ver

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/ver.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/ver
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/ver
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/ver
[gr1]: https://img.shields.io/github/release/LeKovr/go-kit/ver.svg
[gr2]: https://github.com/LeKovr/go-kit/ver/releases
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

## Назначение

По метаданным приложения
* адрес git репозитория
* номер версии

запросить git репозиторий на предмет истории релизов и, если последний релиз отличается от номера версии, выдать предупреждение в лог.

## Использование

см [ver_test.go](ver_test.go)