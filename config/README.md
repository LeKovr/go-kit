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

Пакет для работы с конфигурацией приложения, основанный на [go-flags](https://github.com/jessevdk/go-flags).

## Использование

```golang
package main

import (
	"os"

	"github.com/LeKovr/go-kit/config"
)

// Config holds all config vars.
type Config struct {
	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump

	// ... other config options ...
}

const (
	// Application name
	application = "myapp"
)

var (
	// App version, actual value will be set at build time.
	version = "0.0-dev"

	// Repository address, actual value will be set at build time.
	repo = "repo.git"
)

func main() {

	config.SetApplicationVersion(application, version)
	var cfg Config
	err := config.Open(&cfg)

	defer func() {
		config.Close(err, os.Exit)
	}()

	if err != nil {
		return
	}

	// Do other application work ..
}

```
См. Также: [example/main.go](example/main.go)

## Функционал

В дополнение к возможностям [go-flags](https://github.com/jessevdk/go-flags), добавлен функционал, который активируется при добавлении (embedding) соответствующей структуры в структуру Config, а выполняется в составе `config.Open`.

Доступный в конкретном приложении функционал можно увидеть, вызвав его с ключом `-h`, пример:

```sh
$ ./example -h
Usage:
  example [OPTIONS]

Application Options:
      --version                  Show version and exit
      --config_gen=[|json|md|mk] Generate and print config definition in given format and exit (default: '', means skip) [$CONFIG_GEN]
      --config_dump=             Dump config dest filename [$CONFIG_DUMP]

Help Options:
  -h, --help                     Show this help message

```

### EnableShowVersion

При вызове с ключом `--version`, происходит печать версии приложения и завершение работы

### EnableConfigDump

При вызове с ключом `--config_dump=config.json`, в файл `config.json` сохраняются все настройки приложения и работа продолжается

### EnableConfigDefGen

При вызове с ключом `--config_gen=X`, происходит печать описания параметров конфигурации в формате X и завершение работы.

Поддерживаемые форматы описания

* md - Markdown, для документирования
* mk - Makefile, для генерации первичных значений `.env`
* json - JSON, для внешних систем поддержки конфигураций


## Почему github.com/jessevdk/go-flags ?

Как преимущества, были оценены возможности
1. инкапсулировать все настройки пакета в некую структуру, у которой есть значения по умолчанию и их изменения
пользователь может передать пакету минуя основное приложение (в т.ч. при изменении их списка код приложения менять не надо, достаточно перекомпилировать)
2. иметь удобный способ любую константу, которая в процессе эксплуатации может измениться, вынести в конфиг

### Минусы

Задача выбора из заданного списка вариантов решена Jesse van den Kieboom как перечисление нескольких `choice` в теге поля. Это расходится с `reflect.StructTag.Get`, который не может вернуть массив, и порождает оценки вроде [bad library design](https://github.com/dominikh/go-tools/issues/540).

Замечание линтера `duplicate struct tag "choice" (SA5008)` отключается директивой ` //lint:ignore SA5008 accepted as correct`.

## См также

* https://github.com/ilyakaznacheev/cleanenv
