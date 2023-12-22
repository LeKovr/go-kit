package slogger

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// Config holds package configuration
type Config struct {
	Debug       bool   `long:"debug" description:"Show debug info"`
	Destination string `long:"dest" description:"Log destination (defailt: STDERR)"`
}

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
	//	var logger *slog.Logger
	var handler slog.Handler
	if cfg.Debug {
		/*
			lvl := new(slog.LevelVar)
			lvl.Set(slog.LevelDebug)
			logger = slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
				AddSource: true,
				Level:     lvl,
			}))
		*/
		handler = tint.NewHandler(out, &tint.Options{
			AddSource:  true,
			Level:      slog.LevelDebug,
			TimeFormat: time.Kitchen,
		})
	} else {
		handler = slog.NewJSONHandler(out, nil)
	}
	slog.SetDefault(slog.New(handler))
	return nil
}
