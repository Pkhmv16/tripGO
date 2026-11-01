package create

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
	triphttp "github.com/Pkhmv16/tripGO/internal/http/trip"
	"github.com/Pkhmv16/tripGO/internal/model"
	"github.com/google/uuid"
)

type Handler struct {
	usecase      usecaseContract
	maxBodyBytes int64
}

func New(uc usecaseContract, maxBodyBytes int64) *Handler {
	return &Handler{usecase: uc, maxBodyBytes: maxBodyBytes}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)

	var raw json.RawMessage
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&raw); err != nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "must be valid JSON within the size limit"})
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "must contain one JSON object"})
		return
	}

	if err := validateCreateTripFields(raw); err != nil {
		domainerror.Write(w, r, err)
		return
	}

	var body api.CreateTripJSONRequestBody
	bodyDecoder := json.NewDecoder(bytes.NewReader(raw))
	bodyDecoder.DisallowUnknownFields()

	if err := bodyDecoder.Decode(&body); err != nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "body", Reason: "invalid field type or format"})
		return
	}

	if body.UserId == uuid.Nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "user_id", Reason: "must be a non-zero UUID"})
		return
	}
	if body.DriverId == uuid.Nil {
		domainerror.Write(w, r, &trip.ValidationError{Field: "driver_id", Reason: "must be a non-zero UUID"})
		return
	}
	if err := validateCoordinate("start_point", body.StartPoint); err != nil {
		domainerror.Write(w, r, err)
		return
	}
	if err := validateCoordinate("end_point", body.EndPoint); err != nil {
		domainerror.Write(w, r, err)
		return
	}
	if body.Price < 0 {
		domainerror.Write(w, r, &trip.ValidationError{Field: "price", Reason: "must be non-negative"})
		return
	}

	var idempotencyKey *string

	if params.IdempotencyKey != nil {
		idempotencyKey = new(params.IdempotencyKey.String())
	}

	trip, created, err := h.usecase.CreateTrip(r.Context(), model.CreateTripParams{
		UserId:         trip.UserId(body.UserId.String()),
		DriverId:       trip.DriverId(body.DriverId.String()),
		StartLatitude:  body.StartPoint.Latitude,
		StartLongitude: body.StartPoint.Longitude,
		EndLatitude:    body.EndPoint.Latitude,
		EndLongitude:   body.EndPoint.Longitude,
		Price:          body.Price,
		IdempotencyKey: idempotencyKey,
	})

	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response, err := triphttp.ToApiTrip(trip)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	status := http.StatusOK

	if created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/trips/"+response.Id.String())
	}

	httpresponse.JSON(w, r, status, "application/json", response)
}
