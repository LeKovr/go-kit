package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
)

// Config holds all config vars.
type Config struct {
	Server        server.Config        `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	Observability observability.Config `group:"OpenTelemetry Options" namespace:"otel" env-namespace:"OTEL"`

	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump
}

const application = "observability-example"

var version = "0.0-dev"

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

	ctx := context.Background()

	var obs *observability.Service
	if obs, err = observability.New(ctx, cfg.Observability, application, version); err != nil {
		return
	}

	defer func() {
		if er := obs.Shutdown(ctx); er != nil {
			slog.Error("observability shutdown", "err", er)
		}
	}()

	srv := server.New(cfg.Server)
	srv.Use(obs.HTTPMiddleware())
	srv.ServeMux().HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "hello request")
		_, _ = w.Write([]byte("hello\n"))
	})

	err = srv.Run(ctx)
	if err != nil {
		log.Print(err)
	}
}
