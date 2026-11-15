package listpositions

import (
	"log/slog"
	"net/http"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
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

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, id api.TripId) {
	ctx := r.Context()
	positions, err := h.usecase.ListPositions(ctx, trip.TripId(id.String()))
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(positions))}
	for _, position := range positions {
		item, err := triphttp.ToApiTripPosition(position)
		if err != nil {
			h.logger.ErrorContext(ctx, "trip position response conversion failed", "trip_id", id, "position_id", position.ID, "error", err)
			domainerror.Write(w, r, err)
			return
		}
		response.Positions = append(response.Positions, item)
	}
	httpresponse.JSON(w, r, http.StatusOK, "application/json", response)
}
