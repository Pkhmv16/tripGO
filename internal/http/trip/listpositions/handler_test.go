package listpositions_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	"github.com/Pkhmv16/tripGO/internal/http/trip/listpositions"
)

const (
	tripID       = "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111"
	path         = "/api/v1/trips/" + tripID + "/positions"
	privateError = "private database details"
)

type testServer struct {
	api.Unimplemented
	handler *listpositions.Handler
}

func (s testServer) ListTripPositions(w http.ResponseWriter, r *http.Request, id api.TripId) {
	s.handler.ListTripPositions(w, r, id)
}

func request(uc *MockusecaseContract, path string) *httptest.ResponseRecorder {
	router := api.HandlerWithOptions(testServer{handler: listpositions.New(uc, slog.New(slog.DiscardHandler))}, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
		},
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestListTripPositions(t *testing.T) {
	recorded := time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)
	points := []trip.Position{
		{ID: 8, TripID: tripID, Latitude: 59.93, Longitude: 30.33, RecordedAt: recorded},
		{ID: 3, TripID: tripID, Latitude: 59.94, Longitude: 30.34, RecordedAt: recorded.Add(time.Second)},
	}
	tests := []struct {
		name    string
		points  []trip.Position
		wantIDs []int64
	}{
		{name: "empty"},
		{name: "route", points: points, wantIDs: []int64{8, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecaseContract(gomock.NewController(t))
			uc.EXPECT().ListPositions(gomock.Any(), trip.TripId(tripID)).Return(tt.points, nil)
			w := request(uc, path)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d content-type=%q body=%s", w.Code, w.Header().Get("Content-Type"), w.Body)
			}
			var response api.TripPositionsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Positions == nil {
				t.Fatalf("expected positions array, got %s", w.Body)
			}
			ids := make([]int64, 0, len(response.Positions))
			for _, point := range response.Positions {
				ids = append(ids, point.Id)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("position IDs = %v, want %v", ids, tt.wantIDs)
			}
		})
	}
}

func TestListTripPositionsErrors(t *testing.T) {
	fail := func(err error) func(*MockusecaseContract) {
		return func(uc *MockusecaseContract) {
			uc.EXPECT().ListPositions(gomock.Any(), trip.TripId(tripID)).Return(nil, err)
		}
	}
	tests := []struct {
		name, path, code string
		setup            func(*MockusecaseContract)
		status           int
	}{
		{name: "invalid trip ID", path: "/api/v1/trips/invalid/positions", code: "invalid_request", setup: func(*MockusecaseContract) {}, status: http.StatusBadRequest},
		{name: "trip not found", path: path, code: "trip_not_found", setup: fail(trip.ErrTripNotFound), status: http.StatusNotFound},
		{name: "internal error", path: path, code: "internal_error", setup: fail(errors.New(privateError)), status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecaseContract(gomock.NewController(t))
			tt.setup(uc)
			w := request(uc, tt.path)
			if w.Code != tt.status || w.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("status=%d content-type=%q body=%s", w.Code, w.Header().Get("Content-Type"), w.Body)
			}
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != tt.code || int(problem.Status) != tt.status || strings.Contains(w.Body.String(), privateError) {
				t.Errorf("unexpected problem: %s", w.Body)
			}
		})
	}
}
