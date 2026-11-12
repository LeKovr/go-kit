package main

import (
	"context"
	"errors"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestNATSDrain(t *testing.T) {
	for _, scenario := range []string{"graceful", "timeout", "zero"} {
		t.Run(scenario, func(t *testing.T) {
			broker, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, NoSystemAccount: true})
			if err != nil {
				t.Fatal(err)
			}
			broker.Start()
			t.Cleanup(broker.Shutdown)
			if !broker.ReadyForConnections(time.Second) {
				t.Fatal("broker not ready")
			}
			client, err := nats.Connect(broker.ClientURL())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			grace := time.Second
			if scenario == "timeout" {
				grace = 30 * time.Millisecond
			}
			if scenario == "zero" {
				grace = 0
			}
			type key struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "value"))
			defer cancel()
			started, released, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
			workerConn := make(chan *nats.Conn, 1)
			handlerError := make(chan error, 1)
			done := make(chan error, 1)
			go func() {
				done <- runNATSSubscription(ctx, NATSConfig{URL: broker.ClientURL(), Timeout: time.Second, GracePeriod: grace}, func(callbackCtx context.Context, conn *nats.Conn) nats.MsgHandler {
					workerConn <- conn
					return func(msg *nats.Msg) {
						close(started)
						if callbackCtx.Value(key{}) != "value" {
							handlerError <- errors.New("context value lost")
						}
						if scenario == "graceful" {
							<-released
							if callbackCtx.Err() != nil {
								handlerError <- errors.New("callback canceled during drain")
							}
							if err := conn.Publish("completed", []byte("ok")); err != nil {
								handlerError <- err
							}
						} else {
							<-callbackCtx.Done()
							close(canceled)
							<-released // Prove the worker waits for callback cleanup before returning.
						}
					}
				})
			}()
			conn := <-workerConn
			waitCondition(t, func() bool { return broker.NumSubscriptions() == 1 })
			if err := client.Publish(natsSubject, []byte("calculate")); err != nil {
				t.Fatal(err)
			}
			if err := client.FlushTimeout(time.Second); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("callback did not start")
			}
			cancel()
			if scenario == "graceful" {
				waitCondition(t, conn.IsDraining)
			} else {
				select {
				case <-canceled:
				case <-time.After(time.Second):
					t.Fatal("callback not canceled")
				}
			}
			select {
			case err := <-done:
				t.Fatalf("returned before callback finished: %v", err)
			default:
			}
			close(released)
			select {
			case err := <-done:
				if scenario == "timeout" {
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("timeout error = %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("worker did not finish")
			}
			select {
			case err := <-handlerError:
				t.Fatal(err)
			default:
			}
			if !conn.IsClosed() {
				t.Fatal("connection left open")
			}
		})
	}
}

func TestNATSRejectedStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	never := func(context.Context, *nats.Conn) nats.MsgHandler { t.Fatal("handler initialized"); return nil }
	if err := runNATSSubscription(ctx, NATSConfig{URL: "bad://address"}, never); err != nil {
		t.Fatal(err)
	}
	if err := runNATSSubscription(context.Background(), NATSConfig{URL: "bad://address", GracePeriod: -1}, never); err == nil {
		t.Fatal("negative grace accepted")
	}
}

func waitCondition(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("condition not reached")
		}
	}
}
