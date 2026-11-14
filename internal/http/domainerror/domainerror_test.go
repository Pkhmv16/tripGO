package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
)

type problemCase struct {
	name   string
	err    error
	status int32
	kind   string
	code   string
	title  string
	detail string
}

func problemCases() []problemCase {
	return []problemCase{
		{
			name: "validation", err: &trip.ValidationError{Field: "price", Reason: "must be non-negative"},
			status: http.StatusBadRequest, kind: "invalid-request", code: "invalid_request",
			title: "Invalid request", detail: "price: must be non-negative",
		},
		{
			name: "nested field validation", err: &trip.ValidationError{Field: "start_point.latitude", Reason: "must be between -90 and 90"},
			status: http.StatusBadRequest, kind: "invalid-request", code: "invalid_request",
			title: "Invalid request", detail: "start_point.latitude: must be between -90 and 90",
		},
		{
			name: "trip not found", err: trip.ErrTripNotFound,
			status: http.StatusNotFound, kind: "trip-not-found", code: "trip_not_found",
			title: "Trip not found", detail: "Trip does not exist",
		},
		{
			name: "trip completed", err: trip.ErrTripAlreadyCompleted,
			status: http.StatusConflict, kind: "trip-completed", code: "trip_completed",
			title: "Trip completed", detail: "Trip is already completed",
		},
		{
			name: "driver busy", err: trip.ErrDriverBusy,
			status: http.StatusConflict, kind: "driver-busy", code: "driver_busy",
			title: "Driver busy", detail: "Driver already has an active trip",
		},
		{
			name: "idempotency conflict", err: trip.ErrIdempotencyConflict,
			status: http.StatusConflict, kind: "idempotency-conflict", code: "idempotency_conflict",
			title: "Idempotency conflict", detail: "Idempotency-Key was already used with a different request body",
		},
		{
			name: "internal error", err: errors.New("private SQL details: password=secret"),
			status: http.StatusInternalServerError, kind: "internal-error", code: "internal_error",
			title: "Internal server error", detail: "An unexpected error has occurred",
		},
	}
}

func (tc problemCase) expected() api.Problem {
	return api.Problem{
		Type:  "https://tripgo.example/problems/" + tc.kind,
		Title: tc.title, Status: tc.status, Code: tc.code, Detail: &tc.detail,
	}
}

func TestProblem(t *testing.T) {
	for _, tc := range problemCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, variant := range []struct {
				name string
				err  error
			}{
				{name: "direct", err: tc.err},
				{name: "wrapped", err: fmt.Errorf("private wrapper: %w", tc.err)},
				{name: "nested wrapper", err: fmt.Errorf("private outer wrapper: %w", fmt.Errorf("private inner wrapper: %w", tc.err))},
			} {
				t.Run(variant.name, func(t *testing.T) {
					status, got := domainerror.Problem(variant.err)
					if status != int(tc.status) {
						t.Errorf("status = %d, want %d", status, tc.status)
					}
					if want := tc.expected(); !reflect.DeepEqual(got, want) {
						t.Errorf("Problem() = %+v, want %+v", got, want)
					}
				})
			}
		})
	}
}
