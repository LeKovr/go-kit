package slogger_test

import (
	"log/slog"
	"strings"
	"testing"

	ass "github.com/alecthomas/assert/v2"

	"github.com/LeKovr/go-kit/slogger"
)

func TestSetup(t *testing.T) {
	want := `level=WARN source=slogger_test.go:42 msg=sample format=auto debug=true
level=INFO source=slogger_test.go:43 msg="info string"
level=DEBUG source=slogger_test.go:44 msg="debug string"
level=WARN source=slogger_test.go:42 msg=sample format=text debug=true
level=INFO source=slogger_test.go:43 msg="info string"
level=DEBUG source=slogger_test.go:44 msg="debug string"
{"level":"WARN","msg":"sample","format":"json","debug":true}
{"level":"INFO","msg":"info string"}
{"level":"DEBUG","msg":"debug string"}
{"level":"WARN","msg":"sample","format":"auto","debug":false}
{"level":"INFO","msg":"info string"}
level=WARN source=slogger_test.go:42 msg=sample format=text debug=false
level=INFO source=slogger_test.go:43 msg="info string"
{"level":"WARN","msg":"sample","format":"json","debug":false}
{"level":"INFO","msg":"info string"}
`
	formats := []string{"auto", "text", "json"}
	debugLevels := []bool{true, false}
	var b strings.Builder
	cfg := slogger.Config{TimeFormat: slogger.TimeDisableKey}
	//	out := os.Stdout
	out := &b
	for _, level := range debugLevels {
		cfg.Debug = level
		for _, format := range formats {
			cfg.Format = format
			err := slogger.Setup(cfg, out)
			ass.NoError(t, err)
			slog.Warn("sample", "format", format, "debug", level)
			slog.Info("info string")
			slog.Debug("debug string")
		}
	}
	t.Log(b.String())
	ass.Equal(t, want, b.String())
}

func TestSwitch(t *testing.T) {
	want := `{"level":"DEBUG","msg":"debug","id":2}
`
	var b strings.Builder
	cfg := slogger.Config{TimeFormat: slogger.TimeDisableKey}
	//	out := os.Stdout
	out := &b
	err := slogger.Setup(cfg, out)
	ass.NoError(t, err)
	for i := 0; i < 3; i++ {
		slog.Debug("debug", "id", i+1)
		slogger.LogLevelSwitch()
	}
	ass.Equal(t, want, b.String())

}

func TestWithScope(t *testing.T) {
	out := setupScope(t)
	base := slog.Default()
	module := slogger.WithScope(base, "module")

	base.Info("without scope")
	module.Info("module scope")
	slogger.WithScope(base, "").Info("empty name")
	module.With("worker", "sync").Info("with attributes")
	module.WithGroup("request").Info("with group", "id", 42)
	slogger.WithScope(nil, "default").Info("nil base")

	want := `{"level":"INFO","msg":"without scope"}
{"level":"INFO","msg":"module scope","otel.scope.name":"module"}
{"level":"INFO","msg":"empty name"}
{"level":"INFO","msg":"with attributes","otel.scope.name":"module","worker":"sync"}
{"level":"INFO","msg":"with group","otel.scope.name":"module","request":{"id":42}}
{"level":"INFO","msg":"nil base","otel.scope.name":"default"}
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
