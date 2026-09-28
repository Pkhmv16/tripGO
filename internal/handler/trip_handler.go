package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/Pkhmv16/tripGO/internal/domain"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/service"
	"github.com/google/uuid"
)

type TripService interface {
	GetTrip(ctx context.Context, tripId string) (*domain.Trip, error)
	FinishTrip(ctx context.Context, tripId string) (*domain.Trip, error)
	CreateTrip(ctx context.Context, params service.CreateTripParams) (*domain.Trip, error)
}

type TripHandler struct {
	tripService  TripService
	maxBodyBytes int64
}

func NewTripHandler(tripService TripService, maxBodyBytes int64) *TripHandler {
	return &TripHandler{
		tripService:  tripService,
		maxBodyBytes: maxBodyBytes,
	}
}

func (h *TripHandler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.GetTrip(r.Context(), tripId.String())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response, err := toApiTrip(trip)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJson(w, r, http.StatusOK, "application/json", response)
}

func (h *TripHandler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.FinishTrip(r.Context(), tripId.String())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response, err := toApiTrip(trip)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJson(w, r, http.StatusOK, "application/json", response)
}

func (h *TripHandler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	
	var raw json.RawMessage
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&raw); err != nil {
		writeInvalidRequest(w, r, "Request body must be a valid JSON")
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		writeInvalidRequest(w, r, "Request body must contain one JSON object")
		return
	}

	if err := validateCreateTripFields(raw); err != nil {
		writeInvalidRequest(w, r, err.Error())
		return
	}

	var body api.CreateTripJSONRequestBody
	bodyDecoder := json.NewDecoder(bytes.NewReader(raw))
	bodyDecoder.DisallowUnknownFields()

	if err := bodyDecoder.Decode(&body); err != nil {
		writeInvalidRequest(w, r, "Invalid request body")
		return
	}

	if body.UserId == uuid.Nil || body.DriverId == uuid.Nil {
		writeInvalidRequest(w, r, "user_id and driver_id must be non-zero UUIDs")
		return
	}

	if !validCoordinate(body.StartPoint) || !validCoordinate(body.EndPoint) {
		writeInvalidRequest(w, r, "Coordinates are outside allowed ranges")
		return
	}

	if body.Price < 0 {
		writeInvalidRequest(w, r, "Price must be non-negative")
		return
	}

	trip, err := h.tripService.CreateTrip(r.Context(), service.CreateTripParams{
		UserId:         domain.UserId(body.UserId.String()),
		DriverId:       domain.DriverId(body.DriverId.String()),
		StartLatitude:  body.StartPoint.Latitude,
		StartLongitude: body.StartPoint.Longitude,
		EndLatitude:    body.EndPoint.Latitude,
		EndLongitude:   body.EndPoint.Longitude,
		Price:          body.Price,
	})

	if err != nil {
		writeError(w, r, err)
		return
	}

	response, err := toApiTrip(trip)
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+response.Id.String())
	writeJson(w, r, http.StatusCreated, "application/json", response)
}

func validateCreateTripFields(raw json.RawMessage) error {
	fields, err := requireFields(
		raw,
		"user_id",
		"driver_id",
		"start_point",
		"end_point",
		"price",
	)
	if err != nil {
		return err
	}

	for _, name := range []string{"start_point", "end_point"} {
		if _, err := requireFields(fields[name], "latitude", "longitude"); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	return nil
}

func requireFields(raw json.RawMessage, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage

	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("expected a json object")
	}

	for _, name := range names {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("field %q is required and cannot be null", name)
		}
	}

	return fields, nil
}

func toApiTrip(trip *domain.Trip) (*api.Trip, error) {
	id, err := uuid.Parse(string(trip.Id))
	if err != nil {
		return nil, fmt.Errorf("convert trip id: %w", err)
	}

	userID, err := uuid.Parse(string(trip.UserId))
	if err != nil {
		return nil, fmt.Errorf("convert user id: %w", err)
	}

	driverID, err := uuid.Parse(string(trip.DriverId))
	if err != nil {
		return nil, fmt.Errorf("convert driver id: %w", err)
	}

	return &api.Trip{
		Id:       id,
		UserId:   userID,
		DriverId: driverID,
		StartPoint: api.Coordinates{
			Latitude:  trip.StartLatitude,
			Longitude: trip.StartLongitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndLatitude,
			Longitude: trip.EndLongitude,
		},
		Price:          trip.Price,
		Status:         api.TripStatus(trip.Status),
		StartedAt:      trip.StartedAt,
		FinishedAt:     trip.FinishedAt,
		LastPositionAt: nil,
	}, nil
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	kind := "internal-error"
	title := "Internal server error"
	detail := "An unexpected error has occurred"

	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		status = http.StatusNotFound
		code = "trip_not_found"
		kind = "trip-not-found"
		title = "Trip not found"
		detail = "Trip does not exist"

	case errors.Is(err, domain.ErrTripAlreadyCompleted):
		status = http.StatusConflict
		code = "trip_completed"
		kind = "trip-completed"
		title = "Trip completed"
		detail = "Trip is already completed"

	case errors.Is(err, domain.ErrDriverBusy):
		status = http.StatusConflict
		code = "driver_busy"
		kind = "driver-busy"
		title = "Driver busy"
		detail = "Driver already has an active trip"
	}

	writeJson(w, r, status, "application/problem+json", api.Problem{
		Type:     "https://tripgo.example/problems/" + kind,
		Title:    title,
		Status:   int32(status),
		Code:     code,
		Detail:   &detail,
		Instance: new(r.URL.Path),
	})
}

func writeJson(w http.ResponseWriter, r *http.Request, status int, contentType string, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		contentType = "application/problem+json"
		body, _ = json.Marshal(api.Problem{
			Type:     "https://tripgo.example/problems/internal-error",
			Title:    "Internal server error",
			Status:   int32(status),
			Code:     "internal_error",
			Detail:   new("Failed to encode response"),
			Instance: new(r.URL.Path),
		})
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	w.Write(body)
}

func validCoordinate(point api.Coordinates) bool {
	return !math.IsNaN(point.Latitude) && !math.IsNaN(point.Longitude) &&
		point.Latitude >= -90 && point.Latitude <= 90 &&
		point.Longitude >= -180 && point.Longitude <= 180
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	writeJson(w, r, http.StatusBadRequest, "application/problem+json", api.Problem{
		Type:     "https://tripgo.example/problems/invalid-request",
		Title:    "Invalid request",
		Status:   int32(http.StatusBadRequest),
		Code:     "invalid_request",
		Detail:   &detail,
		Instance: new(r.URL.Path),
	})
}
