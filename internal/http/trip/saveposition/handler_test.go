package saveposition_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	"github.com/Pkhmv16/tripGO/internal/http/trip/saveposition"
)

const (
	tripID         = "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111"
	path           = "/api/v1/trips/" + tripID + "/positions"
	body           = `{"coordinates":{"latitude":0,"longitude":30.335},"recorded_at":"2026-09-01T10:05:00Z"}`
	invalidRequest = "invalid_request"
	privateError   = "private database details"
)

type testServer struct {
	api.Unimplemented
	handler *saveposition.Handler
}

func (s testServer) CreateTripPosition(w http.ResponseWriter, r *http.Request, id api.TripId) {
	s.handler.CreateTripPosition(w, r, id)
}

func request(uc *MockusecaseContract, path, payload string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router := api.HandlerWithOptions(testServer{handler: saveposition.New(uc, 1024)}, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
		},
	})
	router.ServeHTTP(recorder, req)
	return recorder
}

func testPosition() trip.Position {
	return trip.Position{TripID: tripID, Longitude: 30.335, RecordedAt: time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)}
}

func TestHandlerCreateTripPosition(t *testing.T) {
	uc := NewMockusecaseContract(gomock.NewController(t))
	input := testPosition()
	saved := input
	saved.ID = 42
	uc.EXPECT().SavePosition(gomock.Any(), input).Return(saved, nil)

	w := request(uc, path, body)
	if w.Code != http.StatusCreated || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d content-type=%q body=%s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	var got api.TripPosition
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Id != saved.ID || got.TripId != uuid.MustParse(tripID) || !got.RecordedAt.Equal(saved.RecordedAt) ||
		got.Coordinates != (api.Coordinates{Latitude: saved.Latitude, Longitude: saved.Longitude}) {
		t.Errorf("unexpected saved position: %+v", got)
	}
}

func TestHandlerCreateTripPositionErrors(t *testing.T) {
	noCalls := func(*MockusecaseContract) {}
	failSave := func(err error) func(*MockusecaseContract) {
		return func(uc *MockusecaseContract) {
			uc.EXPECT().SavePosition(gomock.Any(), testPosition()).Return(trip.Position{}, err)
		}
	}
	tests := []struct {
		name, path, payload string
		setup               func(*MockusecaseContract)
		status              int
		code                string
	}{
		{name: "malformed JSON", path: path, payload: "{", setup: noCalls, status: http.StatusBadRequest, code: invalidRequest},
		{name: "missing latitude", path: path, payload: strings.Replace(body, `"latitude":0,`, "", 1), setup: noCalls, status: http.StatusBadRequest, code: invalidRequest},
		{name: "invalid time", path: path, payload: strings.Replace(body, "2026-09-01T10:05:00Z", "invalid", 1), setup: noCalls, status: http.StatusBadRequest, code: invalidRequest},
		{name: "invalid trip ID", path: "/api/v1/trips/invalid/positions", payload: body, setup: noCalls, status: http.StatusBadRequest, code: invalidRequest},
		{name: "trip not found", path: path, payload: body, setup: failSave(trip.ErrTripNotFound), status: http.StatusNotFound, code: "trip_not_found"},
		{name: "trip completed", path: path, payload: body, setup: failSave(trip.ErrTripAlreadyCompleted), status: http.StatusConflict, code: "trip_completed"},
		{name: "internal error", path: path, payload: body, setup: failSave(errors.New(privateError)), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecaseContract(gomock.NewController(t))
			tt.setup(uc)
			w := request(uc, tt.path, tt.payload)
			if w.Code != tt.status || w.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("status=%d content-type=%q body=%s", w.Code, w.Header().Get("Content-Type"), w.Body)
			}
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != tt.code || int(problem.Status) != tt.status {
				t.Errorf("problem=%+v, want code=%q status=%d", problem, tt.code, tt.status)
			}
			if strings.Contains(w.Body.String(), privateError) {
				t.Error("response contains private error details")
			}
		})
	}
}
