package logger

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		debug   bool
		matches []string
	}{
		{"Debug", true, []string{"test_log", "test_debug"}},
		{"NoDebug", false, []string{"test_log"}},
	}
	for _, tt := range tests {
		buf := new(bytes.Buffer)
		log := New(Config{Debug: tt.debug}, buf)
		log.Info("test_log")
		log.V(1).Info("test_debug")
		for _, str := range tt.matches {
			assert.Contains(t, buf.String(), str)
		}
	}
}
