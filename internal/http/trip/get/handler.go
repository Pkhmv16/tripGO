package get

import (
	"log/slog"
	"net/http"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
	triphttp "github.com/Pkhmv16/tripGO/internal/http/trip"
)

type Handler struct {
	usecase usecaseContract
	logger  *slog.Logger
}

func New(uc usecaseContract, logger *slog.Logger) *Handler {
	return &Handler{usecase: uc, logger: logger}
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.usecase.GetTrip(r.Context(), tripId.String())
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	response, err := triphttp.ToApiTrip(trip)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "trip response conversion failed", "trip_id", trip.Id, "error", err)
		domainerror.Write(w, r, err)
		return
	}

	httpresponse.JSON(w, r, http.StatusOK, "application/json", response)
}
