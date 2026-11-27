package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestIsExpectedShutdown(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, true},
		{"context.Canceled", context.Canceled, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"net.ErrClosed", net.ErrClosed, true},
		{"http.ErrServerClosed", http.ErrServerClosed, true},
		{"cmux closed", errors.New("use of closed network connection"), true},
		{"grpc stopped", errors.New("gRPC serve: server stopped"), true},
		{"wrapped context.Canceled", fmt.Errorf("worker 0: %w", context.Canceled), true},
		{"joined errors", errors.Join(context.Canceled, net.ErrClosed), true},
		{"joined mixed", errors.Join(context.Canceled, errors.New("real failure")), false},
		{"real failure", errors.New("connection refused"), false},
		{"wrapped real failure", fmt.Errorf("serve: %w", errors.New("connection refused")), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExpectedShutdown(tt.err)
			if got != tt.want {
				t.Errorf("IsExpectedShutdown(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
