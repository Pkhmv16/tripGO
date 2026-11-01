package domainerror

import (
	"errors"
	"net/http"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	httpresponse "github.com/Pkhmv16/tripGO/internal/http/response"
)

func Problem(err error) (int, api.Problem) {
	status := http.StatusInternalServerError
	code := "internal_error"
	kind := "internal-error"
	title := "Internal server error"
	detail := "An unexpected error has occurred"
	var validationErr *trip.ValidationError

	switch {
	case errors.As(err, &validationErr):
		status = http.StatusBadRequest
		code = "invalid_request"
		kind = "invalid-request"
		title = "Invalid request"
		detail = validationErr.Error()

	case errors.Is(err, trip.ErrTripNotFound):
		status = http.StatusNotFound
		code = "trip_not_found"
		kind = "trip-not-found"
		title = "Trip not found"
		detail = "Trip does not exist"

	case errors.Is(err, trip.ErrTripAlreadyCompleted):
		status = http.StatusConflict
		code = "trip_completed"
		kind = "trip-completed"
		title = "Trip completed"
		detail = "Trip is already completed"

	case errors.Is(err, trip.ErrDriverBusy):
		status = http.StatusConflict
		code = "driver_busy"
		kind = "driver-busy"
		title = "Driver busy"
		detail = "Driver already has an active trip"

	case errors.Is(err, trip.ErrIdempotencyConflict):
		status = http.StatusConflict
		code = "idempotency_conflict"
		kind = "idempotency-conflict"
		title = "Idempotency conflict"
		detail = "Idempotency-Key was already used with a different request body"
	}

	return status, api.Problem{
		Type:   "https://tripgo.example/problems/" + kind,
		Title:  title,
		Status: int32(status),
		Code:   code,
		Detail: &detail,
	}
}

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, body := Problem(err)
	body.Instance = new(r.URL.Path)
	httpresponse.JSON(w, r, status, "application/problem+json", body)
}

func WriteInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	Write(w, r, &trip.ValidationError{Field: "request", Reason: detail})
}
