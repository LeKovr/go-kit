package config

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MyConfig struct {
	MyVar int `long:"var" description:"App config var"`
}

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

func TestClose(t *testing.T) {
	extErr := errors.New("external error")
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"Normal", nil, ExitNormal},
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
