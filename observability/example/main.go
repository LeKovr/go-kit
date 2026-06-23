package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
)

// Config holds all config vars.
type Config struct {
	Mode string `long:"mode" default:"server" choice:"server" choice:"client" description:"Example mode: server or client" env:"MODE"`

	Server        server.Config        `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	Client        ClientConfig         `group:"Client Options" namespace:"client" env-namespace:"CLIENT"`
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
	serviceName := application + "-" + cfg.Mode

	var obs *observability.Service
	if obs, err = observability.New(ctx, cfg.Observability, serviceName, version); err != nil {
		return
	}

	defer func() {
		if er := obs.Shutdown(ctx); er != nil {
			slog.Error("observability shutdown", "err", er)
		}
	}()

	switch cfg.Mode {
	case "server":
		err = runServer(ctx, cfg, obs)
	case "client":
		err = runClient(ctx, cfg.Client, obs)
	default:
		err = fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}
