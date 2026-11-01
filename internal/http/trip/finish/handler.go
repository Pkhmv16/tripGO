package finish

import (
	"net/http"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
	triphttp "github.com/Pkhmv16/tripGO/internal/http/trip"
)

type Handler struct{ usecase usecaseContract }

func New(uc usecaseContract) *Handler { return &Handler{usecase: uc} }

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.usecase.FinishTrip(r.Context(), tripId.String())
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	response, err := triphttp.ToApiTrip(trip)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	httpresponse.JSON(w, r, http.StatusOK, "application/json", response)
}
