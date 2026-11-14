package health

import (
	"net/http"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
)

type Handler struct{}

func New() *Handler { return &Handler{} }
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	httpresponse.JSON(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}
