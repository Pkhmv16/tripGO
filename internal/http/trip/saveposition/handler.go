package saveposition

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
	triphttp "github.com/Pkhmv16/tripGO/internal/http/trip"
)

type Handler struct {
	usecase      usecaseContract
	maxBodyBytes int64
}

func New(uc usecaseContract, maxBodyBytes int64) *Handler {
	return &Handler{usecase: uc, maxBodyBytes: maxBodyBytes}
}

type positionRequest struct {
	Coordinates *struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"coordinates"`
	RecordedAt *time.Time `json:"recorded_at"`
}

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, id api.TripId) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	decoder.DisallowUnknownFields()

	var body positionRequest
	if err := decoder.Decode(&body); err != nil {
		domainerror.WriteInvalidRequest(w, r, "Invalid position payload")
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		domainerror.WriteInvalidRequest(w, r, "Expected exactly one JSON object")
		return
	}
	if body.Coordinates == nil || body.Coordinates.Latitude == nil || body.Coordinates.Longitude == nil || body.RecordedAt == nil {
		domainerror.WriteInvalidRequest(w, r, "Latitude, longitude and recorded_at are required")
		return
	}

	saved, err := h.usecase.SavePosition(r.Context(), trip.Position{
		TripID:     trip.TripId(id.String()),
		Latitude:   *body.Coordinates.Latitude,
		Longitude:  *body.Coordinates.Longitude,
		RecordedAt: *body.RecordedAt,
	})
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	response, err := triphttp.ToApiTripPosition(saved)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	httpresponse.JSON(w, r, http.StatusCreated, "application/json", response)
}
