package main

import (
	"os"

	"github.com/LeKovr/go-kit/config"
)

// Config holds all config vars.
type Config struct {
	config.EnableShowVersion
	config.EnableConfigDefGen
	config.EnableConfigDump

	// ... other config options ...
}

const (
	// Application name
	application = "myapp"
)

var (
	// App version, actual value will be set at build time.
	version = "0.0-dev"

	// Repository address, actual value will be set at build time.
	repo = "repo.git"
)

func main() {

	config.SetApplicationVersion(application, version)
	var cfg Config
	err := config.Open(&cfg)

	defer func() {
		config.Close(err, os.Exit)
	}()

	if err != nil {
		return
	}

	// Do other application work ..
}
