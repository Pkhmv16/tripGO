package ready

import (
	"context"
	"net/http"
	"time"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
)

type Handler struct {
	checker readinessChecker
	timeout time.Duration
}

func New(checker readinessChecker, timeout time.Duration) *Handler {
	return &Handler{checker: checker, timeout: timeout}
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.checker.Ping(ctx); err != nil {
		httpresponse.JSON(w, r, http.StatusServiceUnavailable, "application/json",
			api.HealthResponse{Status: api.Unavailable})
		return
	}

	httpresponse.JSON(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}
