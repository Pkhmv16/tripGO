package handler

import (
	"context"
	"net/http"
	"time"

	api "github.com/Pkhmv16/tripGO/internal/generated"
)

type ReadinessChecker interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	checker ReadinessChecker
	timeout time.Duration
}

func NewHealthHandler(checker ReadinessChecker, timeout time.Duration) *HealthHandler {
	return &HealthHandler{
		checker: checker,
		timeout: timeout,
	}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJson(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.checker.Ping(ctx); err != nil {
		writeJson(w, r, http.StatusServiceUnavailable, "application/json",
			api.HealthResponse{Status: api.Unavailable})
		return
	}

	writeJson(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}
