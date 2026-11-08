package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/slogger"
)

// Config holds all config vars.
type Config struct {
	Mode string `long:"mode" default:"http-gateway" choice:"http-gateway" choice:"http-client" choice:"grpc-server" choice:"nats-worker" description:"Example mode: http-gateway, http-client, grpc-server or nats-worker" env:"MODE"`

	Server        server.Config        `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	GRPC          GRPCConfig           `group:"gRPC Options" namespace:"grpc" env-namespace:"GRPC"`
	NATS          NATSConfig           `group:"NATS Options" namespace:"nats" env-namespace:"NATS"`
	DB            DBConfig             `group:"Database Options" namespace:"db" env-namespace:"DB"`
	Client        ClientConfig         `group:"Client Options" namespace:"client" env-namespace:"CLIENT"`
	Logger        slogger.Config       `group:"Logging Options" namespace:"log" env-namespace:"LOG"`
	Observability observability.Config `group:"OpenTelemetry Options" namespace:"otel" env-namespace:"OTEL"`

	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump
}

const application = "observability-example"

var version = "0.0-dev"

func serviceName(mode string) string {
	return application + "-" + mode
}

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

	if err = slogger.Setup(cfg.Logger, nil); err != nil {
		return
	}

	ctx := context.Background()
	name := serviceName(cfg.Mode)

	var obs *observability.Service
	if obs, err = observability.New(ctx, cfg.Observability, name, version); err != nil {
		return
	}

	defer func() {
		if er := obs.Shutdown(ctx); er != nil {
			slog.Error("observability shutdown", "err", er)
		}
	}()

	switch cfg.Mode {
	case "http-gateway":
		err = runServer(ctx, cfg, obs)
	case "grpc-server":
		err = runGRPCServer(ctx, cfg, obs)
	case "nats-worker":
		err = runNATSWorker(ctx, cfg, obs)
	case "http-client":
		err = runClient(ctx, cfg.Client, obs)
	default:
		err = fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}
