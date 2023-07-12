package ver_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"

	"github.com/LeKovr/go-kit/ver"
	"github.com/stretchr/testify/assert"
)

var (
	// Actual version value will be set at build time
	version = "0.0-dev"
	/*
	   Filled by:
	      go build -ldflags "-X main.version=$(git describe --tags --always)"
	*/

	// App repo
	// repo = "https://github.com/LeKovr/dbrpc.git"
	repo = "git@github.com:LeKovr/dbrpc.git"
	/*
	   Filled by:
	      git config --get remote.origin.url
	*/
)

func ExampleCheck() {
	zl := zerolog.New(os.Stdout).Level(zerolog.InfoLevel)
	var log logr.Logger = zerologr.New(&zl)

	ver.Check(log, repo, version)
	// Output:
	// {"level":"info","v":0,"appVersion":"0.0-dev","sourceVersion":"v0.31","sourceUpdated":"2017-10-17T08:56:03Z","sourceLink":" See https://github.com/LeKovr/dbrpc/releases/tag/v0.31","message":"App version is outdated"}

}

func TestIsCheckOk(t *testing.T) {
	tests := []struct {
		isOk    bool
		version string
		repo    string
		err     string
	}{
		{true, "v0.31", "https://github.com/LeKovr/dbrpc.git", ""},
		{true, "v0.31", "git@github.com:LeKovr/dbrpc.git", ""},
		{true, "any version is ok", "git@github.com:LeKovr/golang-use.git", ""},
		{false, "v0.30", "https://github.com/LeKovr/dbrpc.git", "{\"level\":\"info\",\"v\":0,\"appVersion\":\"v0.30\",\"sourceVersion\":\"v0.31\",\"sourceUpdated\":\"2017-10-17T08:56:03Z\",\"sourceLink\":\" See https://github.com/LeKovr/dbrpc/releases/tag/v0.31\",\"message\":\"App version is outdated\"}\n"},
		{false, "v0.0", "https://localhost:10", "Get \"https://localhost:10/releases.atom\": dial tcp 127.0.0.1:10: connect: connection refused"},
	}
	for _, tt := range tests {
		buf := new(bytes.Buffer)
		zl := zerolog.New(buf).Level(zerolog.InfoLevel)
		var log logr.Logger = zerologr.New(&zl)
		ok, err := ver.IsCheckOk(log, tt.repo, tt.version)
		assert.Equal(t, tt.isOk, ok)
		if !tt.isOk {
			if err != nil {
				assert.EqualError(t, err, tt.err)
			} else {
				assert.Equal(t, tt.err, buf.String())
			}
		}
	}
}
