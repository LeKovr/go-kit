package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/LeKovr/go-kit/server"
	grpcserver "github.com/LeKovr/go-kit/transport/grpc"
	httpserver "github.com/LeKovr/go-kit/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestApplicationDrainsBothTransports(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	appCtx, stop := context.WithCancel(ctx)
	defer stop()

	httpListener, grpcListener := lifecycleListener(t), lifecycleListener(t)
	started := make(chan context.Context, 2)
	release := make(chan struct{})

	releaseRequests := sync.OnceFunc(func() { close(release) })
	defer releaseRequests()

	httpSrv := httpserver.New(httpserver.Config{AccessLog: httpserver.AccessLogDisabled}).WithListener(httpListener)
	httpSrv.ServeMux().HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		started <- r.Context()

		select {
		case <-release:
			_, _ = w.Write([]byte("ok\n"))
		case <-r.Context().Done():
		}
	})

	grpcSrv := grpcserver.New(grpcserver.Config{}, grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		started <- ctx

		select {
		case <-release:
			return handler(ctx, req)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})).WithListener(grpcListener)
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcSrv.Server(), healthSrv)
	srv := server.New(server.Config{GracePeriod: 2 * time.Second}).WithShutdown(httpSrv.Shutdown, grpcSrv.Shutdown)

	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(appCtx, httpSrv.Run, grpcSrv.Run) }()

	conn, err := grpc.NewClient(grpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	requests := make(chan error, 2)

	go func() {
		resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true))
		if err == nil && resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			err = fmt.Errorf("gRPC health status = %v, want SERVING", resp.GetStatus())
		}

		requests <- err
	}()
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+httpListener.Addr().String()+"/health", nil)
		if err == nil {
			var resp *http.Response

			resp, err = http.DefaultClient.Do(req)
			if err == nil {
				var body []byte

				body, err = io.ReadAll(resp.Body)

				_ = resp.Body.Close()
				if err == nil && (resp.StatusCode != http.StatusOK || string(body) != "ok\n") {
					err = fmt.Errorf("HTTP health = %d %q, want 200 ok", resp.StatusCode, body)
				}
			}
		}

		requests <- err
	}()

	var requestContexts []context.Context

	for range 2 {
		select {
		case c := <-started:
			requestContexts = append(requestContexts, c)
		case <-ctx.Done():
			t.Fatal("requests did not start")
		}
	}

	stop()

	for _, ln := range []*closingListener{httpListener, grpcListener} {
		select {
		case <-ln.closed:
		case <-ctx.Done():
			t.Fatal("shutdown did not close both listeners")
		}
	}

	for _, c := range requestContexts {
		if c.Err() != nil {
			t.Fatal("active request canceled during graceful shutdown")
		}
	}

	select {
	case err := <-runDone:
		t.Fatalf("Run returned before requests completed: %v", err)
	default:
	}

	releaseRequests()

	for range 2 {
		select {
		case err := <-requests:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("request did not finish")
		}
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("application did not stop")
	}
}

type closingListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (ln *closingListener) Close() error {
	err := ln.Listener.Close()
	ln.once.Do(func() { close(ln.closed) })

	return err
}

func lifecycleListener(t *testing.T) *closingListener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	return &closingListener{Listener: ln, closed: make(chan struct{})}
}
