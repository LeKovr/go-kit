package main

import (
	"context"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/server/example/api"
	grpcserver "github.com/LeKovr/go-kit/transport/grpc"
	httpserver "github.com/LeKovr/go-kit/transport/http"
)

type Config struct {
	HTTP httpserver.Config `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	GRPC grpcserver.Config `group:"gRPC Options" namespace:"grpc" env-namespace:"GRPC"`

	Server server.Config `env-namespace:"SERVER" group:"Server Options" namespace:"server"`

	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump
}

func main() {
	var cfg Config
	err := config.Open(&cfg)
	defer func() { config.Close(err, os.Exit) }()
	if err != nil {
		return
	}

	svc := &service{}

	httpServer := httpserver.New(cfg.HTTP)
	httpServer.ServeMux().HandleFunc("GET /health", healthHTTP(svc))

	grpcServer := grpcserver.New(cfg.GRPC)
	api.RegisterExampleServiceServer(grpcServer.Server(), &grpcHandler{service: svc})

	srv := server.New(cfg.Server).WithShutdown(httpServer.Shutdown, grpcServer.Shutdown)
	err = srv.Run(context.Background(), httpServer.Run, grpcServer.Run)
}
