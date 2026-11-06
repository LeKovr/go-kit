//go:build unix

package server

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestRunSignals(t *testing.T) {
	if name := os.Getenv("GO_KIT_SERVER_TEST_SIGNAL"); name != "" {
		var sig os.Signal = os.Interrupt
		if name == "SIGTERM" {
			sig = syscall.SIGTERM
		}
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		defer process.Release()
		ready := make(chan struct{})
		var stopped atomic.Int32
		err = Run(context.Background(),
			func(ctx context.Context) error {
				<-ready
				if err := process.Signal(sig); err != nil {
					return err
				}
				<-ctx.Done()
				stopped.Add(1)
				return nil
			},
			func(ctx context.Context) error {
				close(ready)
				<-ctx.Done()
				stopped.Add(1)
				return nil
			},
		)
		if err != nil || stopped.Load() != 2 {
			t.Fatalf("Run error = %v, stopped workers = %d", err, stopped.Load())
		}
		return
	}

	for _, name := range []string{"SIGINT", "SIGTERM"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunSignals$")
			cmd.Env = append(os.Environ(), "GO_KIT_SERVER_TEST_SIGNAL="+name)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("signal helper: %v\n%s", err, output)
			}
		})
	}
}
