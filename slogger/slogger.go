package slogger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path"
	"testing"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"

	slogotel "github.com/remychantenay/slog-otel"
)

// Config holds package configuration.
type Config struct {
	Debug       bool   `long:"debug" description:"Show debug info"  env:"DEBUG"`
	Format      string `description:"Output format (default: '', means use text if DEBUG)" long:"format" env:"FORMAT" choice:"" choice:"text" choice:"json"` //lint:ignore SA5008 accepted as correct
	TimeFormat  string `description:"Time format for text output" long:"time_format" env:"TIME_FORMAT" default:"2006-01-02 15:04:05.000"`
	Destination string `description:"Log destination (default: '', means STDERR)" long:"dest" env:"DEST"`
}

// TimeDisableKey removes time from the output.
const TimeDisableKey = " "

// LogLevel holds
var LogLevel = new(slog.LevelVar) // The zero LevelVar corresponds to [LevelInfo].

// Setup creates slog default logger.
func Setup(cfg Config, out io.Writer) error {
	if out == nil {
		if cfg.Destination == "" {
			out = os.Stderr
		} else {
			var err error
			out, err = os.OpenFile(cfg.Destination, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				return err
			}
		}
	}
	if cfg.Debug {
		LogLevel.Set(slog.LevelDebug)
	} else {
		LogLevel.Set(slog.LevelInfo)
	}
	useText := cfg.Format == "text" ||
		cfg.Format != "json" && cfg.Debug

	var handler slog.Handler
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey && cfg.TimeFormat == TimeDisableKey {
			// Remove time from the output for predictable test output.
			return slog.Attr{}
		}
		if a.Key == slog.SourceKey {
			// Shorten source file path.
			s := a.Value.Any().(*slog.Source)
			s.File = path.Base(s.File)
		}
		return a
	}
	if useText {
		if f, ok := out.(*os.File); ok && (isatty.IsTerminal(f.Fd()) || testing.Testing()) {
			handler = tint.NewHandler(out, &tint.Options{
				AddSource:  true,
				Level:      LogLevel,
				TimeFormat: cfg.TimeFormat,
			})
		} else {
			// out Text if not inTerminal
			handler = slog.NewTextHandler(out, &slog.HandlerOptions{
				AddSource:   true,
				Level:       LogLevel,
				ReplaceAttr: replaceAttr,
			})
		}
	} else {
		handler = slog.NewJSONHandler(out, &slog.HandlerOptions{
			Level:       LogLevel,
			ReplaceAttr: replaceAttr,
		})
	}
	// Works when OTEL_EXPORTER_OTLP_ENDPOINT is set
	handler = slogotel.OtelHandler{Next: handler}
	slog.SetDefault(slog.New(handler))
	return nil
}

// ErrAttr returns slog.Attr for err value.
func ErrAttr(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}

// code from https://evilmartians.com/chronicles/realtime-diagnostic-logging-or-how-to-really-spy-on-your-go-web-apps

// LogLevelSet updates the application log level.
func LogLevelSet(lvl slog.Level) {
	LogLevel.Set(lvl)
}

// LogLevelSwitch switches the application log level between Debug and Info.
func LogLevelSwitch() {
	LogLevel.Set(slog.LevelDebug - LogLevel.Level())
}

type contextKey string

const loggerKey contextKey = "logger"

// NewContext returns a new context with the provided logger.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext returns the logger from the provided context. If no logger it returns the default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}

	return slog.Default()
}
