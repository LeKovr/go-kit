# go-kit/logger

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/logger.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/logger
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/logger
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/logger
[gr1]: https://img.shields.io/github/release/LeKovr/go-kit/logger.svg
[gr2]: https://github.com/LeKovr/go-kit/logger/releases
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

## Требования

* поля со значениями, а не строки
* в числе полей есть имя файла и номер строки
* в проде поля пишутся в json
* при отладке в консоли - читаемый вывод
* для тестов вывод пишется в буфер и его можно анализировать
* ?? возможность влючить отладку заданного пакета


## Почему github.com/go-logr/logr ?

Автор приложения, использующего ваш пакет, по разным причинам может выбрать одну из многих систем журналирования.
Я предпочитаю вариант, при котором этот выбор не ограничивается моим пакетом.
Т.е. мои пакеты для журналирования используют внешний интерфейс, а выбор пакета журналирования я оставляю за автором приложения.

## Зачем log.V(X).Info?

По сравнению с вариантом `log.Debug()` и `log.Warn()`. использование переменной позволяет изменять уровень журналирования пакета
при старте программы или в процессе ее работы.

В частности, если число для отладки (1) положить в переменную `DL` и для журналирования использовать `log.V(DL).Info`,
то при отладке всего приложения можно выключить журналирование неактуального пакета инструкцией вида `pkg.DL = 9`.

## TODO

примеры (narra? webtail?)
