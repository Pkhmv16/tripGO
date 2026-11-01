package handler

import (
	"net/http"

	api "github.com/Pkhmv16/tripGO/internal/generated"
)

type Handler struct {
	*TripHandler
	*HealthHandler
}

var _ api.ServerInterface = (*Handler)(nil)

func NewRouter(tripHandler *TripHandler, healthHandler *HealthHandler) http.Handler {
	h := &Handler{tripHandler, healthHandler}

	return api.HandlerWithOptions(h, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeInvalidRequest(w, r, "Invalid request parameter")
		},
	})
}
