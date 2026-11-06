package main

import (
	"os"
	"testing"
	"time"

	"github.com/LeKovr/go-kit/config"
)

func TestTransportConfig(t *testing.T) {
	t.Setenv("GRPC_LISTEN", ":9191")
	t.Setenv("GRPC_ADDRESS", "upstream:9292")
	t.Setenv("GRPC_GRACE", "4s")
	t.Setenv("NATS_GRACE", "5s")
	var cfg Config
	if err := config.Open(&cfg, "--mode=grpc-server"); err != nil {
		t.Fatal(err)
	}
	if cfg.GRPC.Listen != ":9191" || cfg.GRPC.Address != "upstream:9292" || cfg.GRPC.GracePeriod != 4*time.Second || cfg.NATS.GracePeriod != 5*time.Second {
		t.Fatalf("config: GRPC=%+v NATS=%+v", cfg.GRPC, cfg.NATS)
	}
	if err := config.Open(&cfg, "--grpc.listen=:9393", "--grpc.address=other:9494", "--grpc.grace=0", "--nats.grace=0"); err != nil {
		t.Fatal(err)
	}
	if cfg.GRPC.Listen != ":9393" || cfg.GRPC.Address != "other:9494" || cfg.GRPC.GracePeriod != 0 || cfg.NATS.GracePeriod != 0 {
		t.Fatalf("CLI: GRPC=%+v NATS=%+v", cfg.GRPC, cfg.NATS)
	}
}

func TestTransportConfigDefaults(t *testing.T) {
	for _, key := range []string{"GRPC_LISTEN", "GRPC_ADDRESS", "GRPC_GRACE", "GRPC_TIMEOUT", "NATS_GRACE"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	var cfg Config
	if err := config.Open(&cfg, "--mode=http-gateway"); err != nil {
		t.Fatal(err)
	}
	if cfg.GRPC.Listen != ":9090" || cfg.GRPC.Address != "127.0.0.1:9090" || cfg.GRPC.GracePeriod != 10*time.Second || cfg.GRPC.Timeout != 3*time.Second || cfg.NATS.GracePeriod != 10*time.Second {
		t.Fatalf("defaults: GRPC=%+v NATS=%+v", cfg.GRPC, cfg.NATS)
	}
}
