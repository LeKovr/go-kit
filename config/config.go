package config

import (
	"errors"
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
)

const (
	// ExitNormal means there is no errors
	ExitNormal = iota
	// ExitError returned on application error
	ExitError
	// ExitBadArgs returned on config parse error
	ExitBadArgs
	// ExitHelp returned when help was requested
	ExitHelp
)

var (
	// ErrHelpRequest returned when help requested
	ErrHelpRequest = errors.New("help requested")
	// ErrPrinted returned after showing error message
	ErrPrinted = errors.New("error printed")
	// ErrVersion returned after showing app version
	ErrVersion = errors.New("version printed")
)

// Config содержит базовые настройки приложения.
type Config struct {
	// Version - флаг для вывода версии приложения.
	Version bool `description:"Show version and exit" long:"version"`
}

// ErrBadArgsContainer holds config parse error
type ErrBadArgsContainer struct {
	err error
}

// Error returns inner Error()
func (e ErrBadArgsContainer) Error() string {
	return e.err.Error()
}

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
		return ErrBadArgsContainer{err}
	}
	return
}

// Close runs exit after deferred cleanups have run
func Close(e error, exitFunc func(code int)) {
	if e == nil {
		exitFunc(ExitNormal)
		return
	}
	code := ExitError
	if _, ok := e.(ErrBadArgsContainer); ok {
		code = ExitBadArgs
	}
	switch e {
	case ErrHelpRequest:
		// help was printed in Parse
		code = ExitHelp
	case ErrPrinted:
		// error was printed already
	case ErrVersion:
		code = ExitNormal
	default:
		fmt.Fprintln(os.Stderr, e.Error())
	}
	exitFunc(code)
}

func (cfg Config) VersionRequested(application, version string) error {
	if cfg.Version {
		fmt.Println(application, version)
		return ErrVersion
	}
	return nil
}
