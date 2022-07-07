package config

import (
	"errors"
	"io"

	"github.com/jessevdk/go-flags"
)

// -----------------------------------------------------------------------------
var (
	// ErrVersionRequest returned when version info requested
	ErrVersionRequest = errors.New("version requested")
	// ErrHelpRequest returned when help requested
	ErrHelpRequest = errors.New("help requested")
	// ErrBadArgs returned after showing command args error message
	ErrBadArgs = errors.New("option error printed")
)

// Config defines base config prameters
type Config struct {
	Debug bool `long:"debug"                         description:"Show debug data"`
}

type ConfigWithVersionRequest interface {
	ShowVersion() bool
}

// ConfigWithVersion defines Config with version flag
type ConfigWithVersion struct {
	Config
	ShowVersionAndExit bool `long:"version"                       description:"Show version and exit"`
}

func (cfg ConfigWithVersion) ShowVersion() bool { return cfg.ShowVersionAndExit }

// Open loads flags from args (if given) or command flags and ENV otherwise
func Open(cfg interface{}, args ...string) (err error) {
	p := flags.NewParser(cfg, flags.Default) //  HelpFlag | PrintErrors | PassDoubleDash
	if len(args) == 0 {
		_, err = p.Parse()
	} else {
		_, err = p.ParseArgs(args)
	}
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			return ErrHelpRequest
		}
		return ErrBadArgs
	}
	return
}

// OpenWithVersion does Open and returns error when version requested
func OpenWithVersion(cfg ConfigWithVersionRequest, args ...string) (err error) {
	err = Open(cfg, args...)
	if err != nil {
		return
	}
	if cfg.ShowVersion() {
		return ErrVersionRequest
	}
	return
}

// Close runs exit after deferred cleanups have run
func Close(exitFunc func(code int), e error, out io.Writer, version string) {
	if e != nil {
		var code int
		switch e {
		case ErrHelpRequest:
			out.Write([]byte(e.Error()))
			code = 3
		case ErrVersionRequest:
			out.Write([]byte(version))
		case ErrBadArgs:
			code = 2
		default:
			out.Write([]byte(e.Error()))
			code = 1
		}
		exitFunc(code)
	}
}
