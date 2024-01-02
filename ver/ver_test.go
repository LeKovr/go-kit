package ver_test

import (
	"bytes"
	"log/slog"
	"os"
	"testing"

	"github.com/LeKovr/go-kit/ver"
	ass "github.com/alecthomas/assert"
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

func replace(groups []string, a slog.Attr) slog.Attr {
	// Remove time from the output for predictable test output.
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

func ExampleCheck() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: replace}))
	slog.SetDefault(logger)
	ver.Check(repo, version)
	// Output:
	// {"level":"INFO","msg":"App version is outdated","appVersion":"0.0-dev","sourceVersion":"v0.31","sourceUpdated":"2017-10-17T08:56:03Z","sourceLink":" See https://github.com/LeKovr/dbrpc/releases/tag/v0.31"}
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
		{false, "v0.30", "https://github.com/LeKovr/dbrpc.git", "{\"level\":\"INFO\",\"msg\":\"App version is outdated\",\"appVersion\":\"v0.30\",\"sourceVersion\":\"v0.31\",\"sourceUpdated\":\"2017-10-17T08:56:03Z\",\"sourceLink\":\" See https://github.com/LeKovr/dbrpc/releases/tag/v0.31\"}\n"},
		{false, "v0.0", "https://localhost:10", "Get \"https://localhost:10/releases.atom\": dial tcp 127.0.0.1:10: connect: connection refused"},
	}
	for _, tt := range tests {
		buf := new(bytes.Buffer)
		h := slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: replace})
		slog.SetDefault(slog.New(h))
		ok, err := ver.IsCheckOk(tt.repo, tt.version)
		ass.Equal(t, tt.isOk, ok)
		if !tt.isOk {
			if err != nil {
				ass.EqualError(t, err, tt.err)
			} else {
				ass.Equal(t, tt.err, buf.String())
			}
		}
	}
}
