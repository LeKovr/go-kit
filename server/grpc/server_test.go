package grpcserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	pb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestRegistrationAndNativeOptions(t *testing.T) {
	ln := testListener(t)
	var calls atomic.Int32
	srv := New(Config{GracePeriod: time.Second}, grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		return handler(ctx, req)
	})).WithListener(ln)
	pb.RegisterHealthServer(srv.Server(), health.NewServer())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	client := healthClient(t, ln)
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), time.Second)
	defer rpcCancel()
	response, err := client.Check(rpcCtx, &pb.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != pb.HealthCheckResponse_SERVING || calls.Load() != 1 {
		t.Fatalf("response = %v, calls = %d", response, calls.Load())
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := srv.Run(ctx); err == nil {
		t.Fatal("second Run succeeded")
	}
}

func TestShutdown(t *testing.T) {
	for _, scenario := range []string{"graceful", "timeout", "zero"} {
		t.Run(scenario, func(t *testing.T) {
			ln := testListener(t)
			entered, closing := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			grace := time.Second
			if scenario == "timeout" {
				grace = 20 * time.Millisecond
			}
			if scenario == "zero" {
				grace = 0
			}
			srv := New(Config{GracePeriod: grace}).WithListener(&signalListener{Listener: ln, closing: closing, entered: entered})
			started, canceled := make(chan struct{}), make(chan struct{})
			pb.RegisterHealthServer(srv.Server(), &checkServer{check: func(ctx context.Context) (*pb.HealthCheckResponse, error) {
				close(started)
				select {
				case <-release:
					return &pb.HealthCheckResponse{Status: pb.HealthCheckResponse_SERVING}, nil
				case <-ctx.Done():
					close(canceled)
					return nil, ctx.Err()
				}
			}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- srv.Run(ctx) }()
			<-entered
			if err := srv.Run(ctx); err == nil {
				t.Fatal("concurrent Run succeeded")
			}
			rpcDone := make(chan error, 1)
			client := healthClient(t, ln)
			rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer rpcCancel()
			go func() {
				_, err := client.Check(rpcCtx, &pb.HealthCheckRequest{}, grpc.WaitForReady(true))
				rpcDone <- err
			}()
			select {
			case <-started:
			case <-rpcCtx.Done():
				t.Fatal("RPC did not start")
			}
			cancel()
			<-closing
			if scenario == "graceful" {
				select {
				case <-canceled:
					t.Fatal("RPC canceled during drain")
				default:
				}
				select {
				case err := <-done:
					t.Fatalf("returned before active RPC: %v", err)
				default:
				}
				close(release)
			}
			select {
			case err := <-done:
				if scenario == "timeout" {
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("error = %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-rpcCtx.Done():
				t.Fatal("Run did not stop")
			}
			err := <-rpcDone
			if (err == nil) != (scenario == "graceful") {
				t.Fatalf("RPC error = %v", err)
			}
			if scenario != "graceful" {
				select {
				case <-canceled:
				case <-rpcCtx.Done():
					t.Fatal("handler not canceled")
				}
			}
		})
	}
}

func TestStartupAndCleanup(t *testing.T) {
	for _, scenario := range []string{"canceled", "negative", "bind"} {
		t.Run(scenario, func(t *testing.T) {
			ln := testListener(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := Config{Listen: ln.Addr().String()}
			if scenario == "canceled" {
				cancel()
			}
			if scenario == "negative" {
				cfg.GracePeriod = -1
			}
			srv := New(cfg)
			if scenario != "bind" {
				srv.WithListener(ln)
			}
			err := srv.Run(ctx)
			if (err == nil) != (scenario == "canceled") {
				t.Fatalf("error = %v", err)
			}
			if scenario != "bind" {
				if _, err := ln.Accept(); !errors.Is(err, net.ErrClosed) {
					t.Fatalf("listener left open: %v", err)
				}
			}
			if err := srv.Run(ctx); err == nil {
				t.Fatal("second Run succeeded")
			}
		})
	}
}

func TestServeAndCloseErrors(t *testing.T) {
	acceptErr, closeErr := errors.New("accept failed"), errors.New("close failed")
	ln := &errorListener{Listener: testListener(t), acceptErr: acceptErr, closeErr: closeErr}
	err := New(Config{}).WithListener(ln).Run(context.Background())
	if !errors.Is(err, acceptErr) || !errors.Is(err, closeErr) {
		t.Fatalf("lost errors: %v", err)
	}
	if ln.closes.Load() != 1 {
		t.Fatalf("Close called %d times", ln.closes.Load())
	}
}

func TestExternalCloseIsError(t *testing.T) {
	ln := testListener(t)
	_ = ln.Close()
	err := New(Config{}).WithListener(ln).Run(context.Background())
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("unexpected closure ignored: %v", err)
	}
}

type checkServer struct {
	pb.UnimplementedHealthServer
	check func(context.Context) (*pb.HealthCheckResponse, error)
}

func (s *checkServer) Check(ctx context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return s.check(ctx)
}
func testListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}
func healthClient(t *testing.T, ln net.Listener) pb.HealthClient {
	t.Helper()
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewHealthClient(conn)
}

type signalListener struct {
	net.Listener
	entered, closing chan struct{}
	once             sync.Once
}

func (ln *signalListener) Accept() (net.Conn, error) {
	ln.once.Do(func() { close(ln.entered) })
	return ln.Listener.Accept()
}
func (ln *signalListener) Close() error { close(ln.closing); return ln.Listener.Close() }

type errorListener struct {
	net.Listener
	acceptErr, closeErr error
	closes              atomic.Int32
}

func (ln *errorListener) Accept() (net.Conn, error) { return nil, ln.acceptErr }
func (ln *errorListener) Close() error {
	ln.closes.Add(1)
	return errors.Join(ln.Listener.Close(), ln.closeErr)
}
