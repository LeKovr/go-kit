# go-kit

Set of usefull golang packages

* [![GitHub Release][gra1]][gra2]
* [![GitHub Release][grb1]][grb2]
* [![GitHub Release][grc1]][grc2]
* [![GitHub Release][grd1]][grd2]
* [![GitHub Release][gre1]][gre2]


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

## Примеры

* [config](config/example)
* [server](server/example)

## Использование

[embedmd]:# (example/main.go)
```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/slogger"
	"github.com/LeKovr/go-kit/ver"
)

// Config holds all config vars.
type Config struct {
	Root string `default:""      description:"Static files root directory"                                 env:"ROOT"        long:"root"`

	Logger slogger.Config `env-namespace:"LOG" group:"Logging Options"      namespace:"log"`
	Server server.Config  `env-namespace:"SRV" group:"Server Options"       namespace:"srv"`

	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump
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

func main() { Run(context.Background(), os.Exit) }

// Run does whole work with ready for testing signature.
func Run(ctx context.Context, exitFunc func(code int)) {

	/* go-kit/config  */

	config.SetApplicationVersion(application, version)
	var cfg Config
	err := config.Open(&cfg)

	defer func() {
		config.Close(err, exitFunc)
	}()

	if err != nil {
		return
	}

	/* go-kit/slogger  */

	err = slogger.Setup(cfg.Logger, nil)
	if err != nil {
		return
	}

	slog.Info(application, "version", version)

	/* go-kit/ver  */

	go ver.Check(repo, version)

	/* go-kit/server  */

	// static pages server
	hfs := os.DirFS(cfg.Root)
	srv := server.New(cfg.Server).WithStatic(hfs).WithVersion(version)

	err = srv.Run(ctx)
}
```

## Настройки

Использование пакетов добавляет в сервис следующие опции:

```
$ ./server/example/example -h
Usage:
  example [OPTIONS]

Application Options:
      --root=                    Static files root directory [$ROOT]
      --version                  Show version and exit
      --config_gen=[|json|md|mk] Generate and print config definition in given format and exit (default: '', means skip) [$CONFIG_GEN]
      --config_dump=             Dump config dest filename [$CONFIG_DUMP]

Logging Options:
      --log.debug                Show debug info [$LOG_DEBUG]
      --log.format=[|text|json]  Output format (default: '', means use text if DEBUG) [$LOG_FORMAT]
      --log.time_format=         Time format for text output (default: 2006-01-02 15:04:05.000) [$LOG_TIME_FORMAT]
      --log.dest=                Log destination (default: '', means STDERR) [$LOG_DEST]

Server Options:
      --srv.listen=              Addr and port which server listens at (default: :8080) [$SRV_LISTEN]
      --srv.maxheader=           MaxHeaderBytes
      --srv.rto=                 HTTP read timeout (default: 10s)
      --srv.wto=                 HTTP write timeout (default: 60s)
      --srv.grace=               Stop grace period (default: 10s)
      --srv.ip_header=           HTTP Request Header for remote IP (default: X-Real-IP) [$SRV_IP_HEADER]
      --srv.user_header=         HTTP Request Header for username (default: X-Username) [$SRV_USER_HEADER]

HTTPS Options:
      --srv.tls.cert=            CertFile for serving HTTPS instead HTTP [$SRV_TLS_CERT]
      --srv.tls.key=             KeyFile for serving HTTPS instead HTTP [$SRV_TLS_KEY]
      --srv.tls.no-check         disable tls certificate validation

Version response Options:
      --srv.vr.prefix=           URL for version response (default: /js/version.js)
      --srv.vr.format=           Format string for version response (default: "document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n")
      --srv.vr.ctype=            js code Content-Type header (default: text/javascript)

Help Options:
  -h, --help                     Show this help message

```

## Лицензия

Copyright (c) 2022 Aleksei Kovrizhkin <lekovr+gokit@gmail.com>

Исходный код проекта лицензирован под Apache License, Version 2.0 (the "[License](LICENSE)");
