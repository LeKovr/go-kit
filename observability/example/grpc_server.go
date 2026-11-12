package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/LeKovr/go-kit/observability"
	"github.com/LeKovr/go-kit/observability/example/api"
	grpcserver "github.com/LeKovr/go-kit/server/grpc"
	"github.com/LeKovr/go-kit/slogger"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCConfig holds gRPC connection and request settings for the demo.
type GRPCConfig struct {
	grpcserver.Config
	Address string        `default:"127.0.0.1:9090" description:"gRPC destination address" env:"ADDRESS" long:"address"`
	Timeout time.Duration `default:"3s" description:"gRPC calculation timeout" env:"TIMEOUT" long:"timeout"`
}

type calculatorServer struct {
	api.UnimplementedCalculatorServer
	client  *observability.NATSClient
	timeout time.Duration
	logger  *slog.Logger
}

func (s *calculatorServer) Calculate(ctx context.Context, _ *api.CalculateRequest) (*api.CalculateResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	s.logger.InfoContext(ctx, "gRPC calculation requested")
	msg, err := s.client.RequestMsg(ctx, &nats.Msg{Subject: natsSubject, Data: []byte("calculate")})
	if err != nil {
		s.logger.ErrorContext(ctx, "NATS request failed", "err", err)
		code := codes.Internal
		switch {
		case errors.Is(err, nats.ErrNoResponders), errors.Is(err, nats.ErrConnectionClosed):
			code = codes.Unavailable
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
			code = codes.DeadlineExceeded
		case errors.Is(err, context.Canceled):
			code = codes.Canceled
		}
		return nil, status.Error(code, "calculation failed")
	}

	return &api.CalculateResponse{Result: string(msg.Data)}, nil
}

func runGRPCServer(ctx context.Context, cfg Config, obs *observability.Service) error {
	logger := slogger.WithScope(slog.Default(), application+"/grpc-server")
	conn, err := nats.Connect(cfg.NATS.URL,
		nats.Name(application+"-grpc-server"), nats.Timeout(cfg.NATS.Timeout),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	srv := grpcserver.New(cfg.GRPC.Config, grpc.StatsHandler(obs.GRPCServerHandler()))
	api.RegisterCalculatorServer(srv.Server(), &calculatorServer{
		client:  obs.NATSClient(conn),
		timeout: cfg.NATS.Timeout,
		logger:  logger,
	})

	return srv.Run(ctx)
}
