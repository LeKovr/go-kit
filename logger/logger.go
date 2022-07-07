package logger

import (
	"context"
	"io"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
)

type Log struct {
	zerolog.Logger
}

func New(out io.Writer, isDebug bool) logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"
	zerolog.CallerMarshalFunc = func(file string, line int) string {
		short := file
		slashes := 1
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

	var zl zerolog.Logger
	if isDebug {
		zl = zerolog.New(zerolog.ConsoleWriter{Out: out}).Level(zerolog.DebugLevel)
	} else {
		zl = zerolog.New(out).Level(zerolog.InfoLevel)
	}
	zl = zl.With().Caller().Timestamp().Logger()
	var log logr.Logger = zerologr.New(&zl)
	return log
}

// NewContext calls logr.NewContext so ypu don't need to import logr for it.
func NewContext(ctx context.Context, logger logr.Logger) context.Context {
	return logr.NewContext(ctx, logger)
}
