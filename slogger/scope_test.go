package slogger_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	ass "github.com/alecthomas/assert/v2"

	"github.com/LeKovr/go-kit/slogger"
)

func TestContextWithScope(t *testing.T) {
	out := setupScope(t)
	ctx := context.Background()
	moduleCtx := slogger.ContextWithScope(ctx, "module")
	workerCtx := slogger.ContextWithScope(moduleCtx, "module.worker")

	slog.InfoContext(ctx, "without scope")
	slog.InfoContext(moduleCtx, "module scope")
	slog.InfoContext(slogger.ContextWithScope(moduleCtx, ""), "empty name")
	slog.InfoContext(workerCtx, "nested scope")

	want := `{"level":"INFO","msg":"without scope"}
{"level":"INFO","msg":"module scope","otel.scope.name":"module"}
{"level":"INFO","msg":"empty name","otel.scope.name":"module"}
{"level":"INFO","msg":"nested scope","otel.scope.name":"module.worker"}
`
	ass.Equal(t, want, out.String())
}

func setupScope(t *testing.T) *strings.Builder {
	t.Helper()
	previousLogger := slog.Default()
	previousLevel := slogger.LogLevel.Level()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		slogger.LogLevelSet(previousLevel)
	})

	out := new(strings.Builder)
	err := slogger.Setup(slogger.Config{
		Format:     "json",
		TimeFormat: slogger.TimeDisableKey,
	}, out)
	ass.NoError(t, err)

	return out
}
