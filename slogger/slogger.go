package slogger

import (
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"

	slogotel "github.com/remychantenay/slog-otel"
)

// Config holds package configuration.
type Config struct {
	Debug       bool   `long:"debug" description:"Show debug info"`
	Destination string `long:"dest" description:"Log destination (default: STDERR)"`
}

// Setup creates slog default logger.
func Setup(cfg Config, out io.Writer) error {
	if out == nil {
		if cfg.Destination == "" {
			out = os.Stderr
		} else {
			var err error
			out, err = os.OpenFile(cfg.Destination, os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				return err
			}
		}
	}
	var handler slog.Handler
	if cfg.Debug {
		if f, ok := out.(*os.File); ok && (isatty.IsTerminal(f.Fd()) || testing.Testing()) {
			handler = tint.NewHandler(out, &tint.Options{
				AddSource:  true,
				Level:      slog.LevelDebug,
				TimeFormat: time.Kitchen,
			})
		} else {
			// out JSON if not inTerminal
			handler = slog.NewJSONHandler(out, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			})
		}
	} else {
		handler = slog.NewJSONHandler(out, nil)
	}
	// Work when OTEL_EXPORTER_OTLP_ENDPOINT is set
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
