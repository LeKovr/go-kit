package logger

import (
	"context"
	"io"
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
)

// Config holds package configuration
type Config struct {
	Debug       bool   `long:"debug" description:"Show debug info"`
	Destination string `long:"dest" description:"Log destination (defailt: STDERR)"`
}

// New creates new logger according to Config
func New(cfg Config, out io.Writer) logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"
	zerolog.CallerMarshalFunc = shortCallerPath
	if out == nil {
		if cfg.Destination == "" {
			out = os.Stderr
		} else {
			// TODO
		}

	}
	var zl zerolog.Logger
	if cfg.Debug {
		zl = zerolog.New(zerolog.ConsoleWriter{Out: out}).Level(zerolog.DebugLevel)
	} else {
		zl = zerolog.New(out).Level(zerolog.InfoLevel)
	}
	zl = zl.With().Caller().Timestamp().Logger()
	var log logr.Logger = zerologr.New(&zl)
	return log
}

func shortCallerPath(pc uintptr, file string, line int) string {
		short := file
		slashes := 2
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				if slashes == 0 {
					break
				}
				slashes--
			}
		}
		file = short
		return file + ":" + strconv.Itoa(line)
	}

// NewContext calls logr.NewContext so ypu don't need to import logr for it.
func NewContext(ctx context.Context, logger logr.Logger) context.Context {
	return logr.NewContext(ctx, logger)
}
