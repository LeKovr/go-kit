package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/observability/example/api"
	"github.com/LeKovr/go-kit/server"
	"github.com/LeKovr/go-kit/slogger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func runServer(ctx context.Context, cfg Config, obs *observability.Service) error {
	logger := slogger.WithScope(slog.Default(), application+"/http-gateway")
	grpcConn, err := grpc.NewClient(cfg.GRPC.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(obs.GRPCClientHandler()),
		grpc.WithDisableServiceConfig(), // This example does not use DNS service configs.
	)
	if err != nil {
		return err
	}
	defer grpcConn.Close()

	srv := server.New(cfg.Server)
	srv.Use(obs.HTTPMiddleware())
	srv.ServeMux().HandleFunc("/demo", calculatorHTTPHandler(api.NewCalculatorClient(grpcConn), cfg.GRPC.Timeout, logger))
	return srv.Run(ctx)
}

func calculatorHTTPHandler(client api.CalculatorClient, timeout time.Duration, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		logger.InfoContext(ctx, "HTTP calculation requested")
		_, err := client.Calculate(ctx, &api.CalculateRequest{})
		if err != nil {
			logger.ErrorContext(ctx, "gRPC calculation failed", "err", err)
			code := http.StatusBadGateway
			switch status.Code(err) {
			case codes.Unavailable:
				code = http.StatusServiceUnavailable
			case codes.DeadlineExceeded:
				code = http.StatusGatewayTimeout
			}
			http.Error(w, http.StatusText(code), code)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}
}
