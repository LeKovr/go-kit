package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/slogger"
	httpserver "github.com/LeKovr/go-kit/transport/http"
	"github.com/LeKovr/go-kit/ver"
)

// Config holds all config vars.
type Config struct {
	Root string `default:""      description:"Static files root directory"                                 env:"ROOT"        long:"root"`

	Logger slogger.Config    `env-namespace:"LOG" group:"Logging Options"      namespace:"log"`
	HTTP   httpserver.Config `env-namespace:"HTTP" group:"HTTP Options"       namespace:"http"`

	Server server.Config `env-namespace:"SERVER" group:"Server Options" namespace:"server"`

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
	httpSrv := httpserver.New(cfg.HTTP).WithStatic(hfs).WithVersion(version)

	srv := server.New(cfg.Server).WithShutdown(httpSrv.Shutdown)
	err = srv.Run(ctx, httpSrv.Run)
}
