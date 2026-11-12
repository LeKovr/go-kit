package main

import (
	"context"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/server/example/api"
	grpcserver "github.com/LeKovr/go-kit/server/grpc"
	httpserver "github.com/LeKovr/go-kit/server/http"
)

type Config struct {
	HTTP httpserver.Config `group:"HTTP Options" namespace:"http" env-namespace:"HTTP"`
	GRPC grpcserver.Config `group:"gRPC Options" namespace:"grpc" env-namespace:"GRPC"`

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

	err = server.Run(context.Background(), httpServer.Run, grpcServer.Run)
}
