package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

var (

	// Values might be set from caller in SetApplicationVersion function.
	application = "app"
	version     = "0.0-dev"
)

// EnableShowVersion при включении в Config добавляет поддержку `--version`.
type EnableShowVersion struct {
	// GoKitConfigShowVersionOption - флаг для вывода версии приложения.
	GoKitConfigShowVersionOption bool `description:"Show version and exit" long:"version"`
}

// IsShowVersionRequested доступен, если в структуру встроен `EnableShowVersion`.
type IsShowVersionRequested interface {
	GoKitConfigShowVersionRequested() error
}

// Проверяем, что EnableShowVersion implements IsShowVersionRequested.
var _ IsShowVersionRequested = (*EnableShowVersion)(nil)

func (opt EnableShowVersion) GoKitConfigShowVersionRequested() error {
	if opt.GoKitConfigShowVersionOption {
		fmt.Println(application, version)
		return ErrVersion
	}
	return nil
}

func SetApplicationVersion(app, ver string) {
	application = app
	version = ver
}

// EnableConfigDefGen содержит настройки для поддержки `config_gen`.
type EnableConfigDefGen struct {
	GoKitConfigDefGenOption string `description:"Generate and print config definition in given format and exit (default: '', means skip)" long:"config_gen" env:"CONFIG_GEN" choice:"" choice:"json" choice:"md" choice:"mk"`
}

type IsDefGenRequested interface {
	GoKitConfigDefGenRequested(cfg interface{}) error
}

var _ IsDefGenRequested = (*EnableConfigDefGen)(nil)

func (opt EnableConfigDefGen) GoKitConfigDefGenRequested(cfg interface{}) error {
	if opt.GoKitConfigDefGenOption != "" {
		PrintConfig(cfg, opt.GoKitConfigDefGenOption)
		return ErrConfGen
	}
	return nil
}

// Generate содержит настройки для поддержки `config_gen`.
type EnableConfigDump struct {
	// GoKitConfigRequestForConfGen
	GoKitConfigDumpOption string `description:"Dump config dest filename" long:"config_dump" env:"CONFIG_DUMP"`
}

type IsDumpRequested interface {
	GoKitConfigDumpRequested(cfg interface{}) error
}

var _ IsDumpRequested = (*EnableConfigDump)(nil)

func (opt EnableConfigDump) GoKitConfigDumpRequested(cfg interface{}) error {
	if opt.GoKitConfigDumpOption != "" {
		if err := SaveJSON(opt.GoKitConfigDumpOption, cfg); err != nil {
			return err
		}
	}
	return nil
}

func ProcessOptions(cfg interface{}) error {

	if v, ok := cfg.(IsShowVersionRequested); ok {
		if err := v.GoKitConfigShowVersionRequested(); err != nil {
			return err
		}
	}
	if v, ok := cfg.(IsDefGenRequested); ok {
		if err := v.GoKitConfigDefGenRequested(cfg); err != nil {
			return err
		}
	}
	if v, ok := cfg.(IsDumpRequested); ok {
		if err := v.GoKitConfigDumpRequested(cfg); err != nil {
			return err
		}
	}
	return nil
}

const errStrClose = "failed to close file"

// SaveJSON сохраняет данные в файл в формате JSON.
func SaveJSON(fileName string, data interface{}) error {
	file, err := os.Create(fileName) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fileName, err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn(errStrClose, "file", fileName, "err", err)
		}
	}()

	val, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, err = file.Write(val)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %w", fileName, err)
	}

	return nil
}
