package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunForcesCloseAfterGracePeriod(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	srv := New(Config{
		AccessLog:   AccessLogDisabled,
		GracePeriod: 20 * time.Millisecond,
	}).WithListener(ln)
	srv.ServeMux().HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(ctx) }()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, requestErr := http.Get("http://" + ln.Addr().String() + "/")
		if requestErr == nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()

	select {
	case runErr := <-runDone:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("Run error: got %v, want context deadline exceeded", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after grace period")
	}

	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request did not finish after forced close")
	}
}

func TestRunWorkersReturnsShutdownError(t *testing.T) {
	want := errors.New("shutdown failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := New(Config{GracePeriod: time.Second}).
		WithShutdown(func(context.Context) error { return want }).
		RunWorkers(ctx)
	if !errors.Is(err, want) {
		t.Fatalf("RunWorkers error: got %v, want %v", err, want)
	}
}
