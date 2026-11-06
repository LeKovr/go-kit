package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/LeKovr/go-kit/server/example/api"
)

func healthHTTP(svc *service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.Health(r.Context())
		if err != nil {
			http.Error(w, "health check failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(&api.HealthResponse{Status: result}); err != nil {
			slog.Error("Write health response", "err", err)
		}
	}
}

type grpcHandler struct {
	api.UnimplementedExampleServiceServer
	service *service
}

func (h *grpcHandler) Health(ctx context.Context, _ *api.HealthRequest) (*api.HealthResponse, error) {
	result, err := h.service.Health(ctx)
	if err != nil {
		return nil, err
	}
	return &api.HealthResponse{Status: result}, nil
}
