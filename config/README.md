# go-kit/config

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/config.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/config
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/config
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/config
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=config/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=config&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

## Почему github.com/jessevdk/go-flags ?

Мне очень нравится идеи
1. инкапсулировать все настройки пакета в некую структуру, у которой есть значения по умолчанию и их изменения
пользователь может передать пакету минуя основное приложение (в т.ч. при изменении их списка код приложения менять не надо, достаточно перекомпилировать)
2. иметь удобный способ любую константу, которая в процессе эксплуатации может измениться, вынести в конфиг

поэтому каждый раз, встречая новый пакет для работы с конфигурацией, я ищу ответ на вопрос - решены ли эти идеи тут лучше, чем Jesse van den Kieboom. По сейчас я ничего лучше не нашел.

## See also

* https://github.com/ilyakaznacheev/cleanenv

## TODO

примеры (narra? webtail?)
