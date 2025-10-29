package config

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MyConfig struct {
	MyVar int `long:"var" description:"App config var"`
}
type BoolConfig struct {
	BoolVar bool `long:"bool" env:"BOOL" description:"App config var"`
}

const BoolName = "BOOL"

func TestOpen(t *testing.T) {
	tests := []struct {
		name string
		err  string
		args []string
	}{
		{"Help", ErrHelpRequest.Error(), []string{"-h"}},
		{"UnknownFlag", "unknown flag `0'", []string{"-0"}},
		{"KnownFlagErr", "invalid argument for flag `--var' (expected int): strconv.ParseInt: parsing \"xx\": invalid syntax", []string{"--var", "xx"}},
		{"KnownFlagOK", "", []string{"--var", "1"}},
	}
	for _, tt := range tests {
		cfg := &MyConfig{}
		err := Open(cfg, tt.args...)
		if tt.err == "" {
			assert.NoError(t, err, tt.name)
		} else {
			assert.Equal(t, tt.err, err.Error(), tt.name)
		}
	}
}

func TestOpenOsArgs(t *testing.T) {
	// Save original args
	a := os.Args

	os.Args = append([]string{a[0]}, "--var", "1")
	cfg := &MyConfig{}
	err := Open(cfg)
	assert.NoError(t, err)
	assert.Equal(t, 1, cfg.MyVar, "got os.Args")

	// Restore original args
	os.Args = a
}

func TestEnvBool(t *testing.T) {
	// Save original args
	a := os.Args
	e := os.Getenv(BoolName)
	//	os.Setenv(BoolName, "")
	os.Args = []string{a[0]}

	tests := []struct {
		name   string
		set    bool
		val    string
		result bool
	}{
		{"Not set => false", false, "", false},
		{"Set => true", true, "", true},
		{"Set true => true", true, "true", true},
		{"Set false => false", true, "false", false},
	}
	for _, tt := range tests {
		if tt.set {
			os.Setenv(BoolName, tt.val)
		}
		cfg := &BoolConfig{}
		err := Open(cfg)
		assert.NoError(t, err)
		assert.Equal(t, tt.result, cfg.BoolVar, tt.name)
	}

	// Restore original args
	os.Setenv(BoolName, e)
	os.Args = a

}

func TestClose(t *testing.T) {
	extErr := errors.New("external error")
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"Normal", nil, ExitNormal},
		{"Normal", ErrVersion, ExitNormal},
		{"External error", ErrBadArgsContainer{extErr}, ExitBadArgs},
		{"Help request", ErrHelpRequest, ExitHelp},
		{"Error printed", ErrPrinted, ExitError},
		{"Error unknown", extErr, ExitError},
	}
	for _, tt := range tests {
		var c int
		Close(tt.err, func(code int) { c = code })
		if c != tt.code {
			t.Errorf("'%s' failed: expected %d, actual %d", tt.name, tt.code, c)
		}
	}
}

func TestNoVersion(t *testing.T) {
	err := EnableShowVersion{}.GoKitConfigShowVersionRequested()
	assert.NoError(t, err)
}

func Example_versionRequested() {
	cfg := EnableShowVersion{GoKitConfigShowVersionOption: true}
	SetApplicationVersion("app", "version")
	err := cfg.GoKitConfigShowVersionRequested()
	fmt.Println(err)
	// Output:
	// app version
	// version printed
}
