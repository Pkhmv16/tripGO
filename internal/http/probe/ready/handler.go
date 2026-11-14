package ready

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
)

type Handler struct {
	checker readinessChecker
	timeout time.Duration
	logger  *slog.Logger
}

func New(checker readinessChecker, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{checker: checker, timeout: timeout, logger: logger}
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.checker.Ping(ctx); err != nil {
		h.logger.ErrorContext(ctx, "readiness check failed", "error", err)
		httpresponse.JSON(w, r, http.StatusServiceUnavailable, "application/json",
			api.HealthResponse{Status: api.Unavailable})
		return
	}

	httpresponse.JSON(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}
