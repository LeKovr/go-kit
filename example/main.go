package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/LeKovr/go-kit/config"
	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/server"
	grpcserver "github.com/LeKovr/go-kit/server/grpc"
	httpserver "github.com/LeKovr/go-kit/server/http"
	"github.com/LeKovr/go-kit/slogger"
	"github.com/LeKovr/go-kit/ver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Config holds all config vars.
type Config struct {
	Root string `default:""      description:"Static files root directory"                                 env:"ROOT"        long:"root"`

	Logger slogger.Config    `env-namespace:"LOG" group:"Logging Options"      namespace:"log"`
	HTTP   httpserver.Config `env-namespace:"HTTP" group:"HTTP Options"       namespace:"http"`
	GRPC   grpcserver.Config `env-namespace:"GRPC" group:"gRPC Options" namespace:"grpc"`
	Server server.Config     `env-namespace:"SERVER" group:"Server Options" namespace:"server"`

	Observability observability.Config `env-namespace:"OTEL" group:"OpenTelemetry Options" namespace:"otel"`

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

	/* go-kit/observability  */

	obs, err := observability.New(ctx, cfg.Observability, application, version)
	if err != nil {
		return
	}

	defer func() {
		err = errors.Join(err, obs.Shutdown(context.WithoutCancel(ctx)))
	}()

	/* go-kit/server/http  */

	// static pages server
	hfs := os.DirFS(cfg.Root)
	httpSrv := httpserver.New(cfg.HTTP).WithStatic(hfs).WithVersion(version)
	httpSrv.Use(obs.HTTPMiddleware())
	httpSrv.ServeMux().HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("ok\n")); err != nil {
			slog.Error("Write health response", "err", err)
		}
	})

	/* go-kit/server/grpc  */

	grpcSrv := grpcserver.New(cfg.GRPC, grpc.StatsHandler(obs.GRPCServerHandler()))
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcSrv.Server(), healthSrv)

	/* go-kit/server  */

	srv := server.New(cfg.Server).WithShutdown(httpSrv.Shutdown, grpcSrv.Shutdown)
	err = srv.Run(ctx, httpSrv.Run, grpcSrv.Run)
}
