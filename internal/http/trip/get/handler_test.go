package get_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	"github.com/Pkhmv16/tripGO/internal/http/trip/get"
)

type testServer struct {
	api.Unimplemented
	handler *get.Handler
}

const (
	internalErrorCode = "internal_error"
	invalidPrivateID  = "invalid-private-id"
)

func (s testServer) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	s.handler.GetTrip(w, r, id)
}

func TestHandlerGetTrip(t *testing.T) {
	id := uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
	userID := uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c")
	driverID := uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
	started := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	result := domaintrip.NewTrip(domaintrip.TripId(id.String()), domaintrip.UserId(userID.String()), domaintrip.DriverId(driverID.String()), 59.9398, 30.3146, 59.929, 30.3626, 1450, started)

	want := api.Trip{Id: id, UserId: userID, DriverId: driverID, StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146}, EndPoint: api.Coordinates{Latitude: 59.929, Longitude: 30.3626}, Price: 1450, Status: api.TripStatus(domaintrip.StatusActive), StartedAt: started, FinishedAt: result.FinishedAt}
	validPath := "/api/v1/trips/" + id.String() + ""
	invalidPath := "/api/v1/trips/not-a-uuid"
	badTrip := *result
	badTrip.Id = invalidPrivateID
	badUser := *result
	badUser.UserId = invalidPrivateID
	badDriver := *result
	badDriver.DriverId = invalidPrivateID
	expect := func(trip *domaintrip.Trip, err error) func(*MockusecaseContract, context.Context) {
		return func(uc *MockusecaseContract, ctx context.Context) {
			uc.EXPECT().GetTrip(gomock.Cond(func(actual context.Context) bool { return actual.Done() == ctx.Done() }), id.String()).Return(trip, err)
		}
	}
	tests := []struct {
		name       string
		path       string
		setup      func(*MockusecaseContract, context.Context)
		wantStatus int32
		wantCode   string
	}{
		{name: "success", path: validPath, setup: expect(result, nil), wantStatus: 200},
		{name: "not found", path: validPath, setup: expect(nil, domaintrip.ErrTripNotFound), wantStatus: 404, wantCode: "trip_not_found"},

		{name: "internal error", path: validPath, setup: expect(nil, errors.New("private SQL details password=secret")), wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid request id", path: invalidPath, setup: func(*MockusecaseContract, context.Context) {}, wantStatus: 400, wantCode: "invalid_request"},
		{name: "invalid returned trip id", path: validPath, setup: expect(&badTrip, nil), wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned user id", path: validPath, setup: expect(&badUser, nil), wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned driver id", path: validPath, setup: expect(&badDriver, nil), wantStatus: 500, wantCode: internalErrorCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockusecaseContract(ctrl)
			path := tt.path
			req := httptest.NewRequest(http.MethodGet, path, nil)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			tt.setup(uc, ctx)
			router := api.HandlerWithOptions(testServer{handler: get.New(uc, slog.New(slog.DiscardHandler))}, api.ChiServerOptions{ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
			}})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != int(tt.wantStatus) {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantCode != "" {
				if w.Header().Get("Content-Type") != "application/problem+json" {
					t.Error("wrong problem content type")
				}
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tt.wantCode || problem.Status != tt.wantStatus || problem.Instance == nil || *problem.Instance != path {
					t.Errorf("problem=%+v", problem)
				}
				for _, private := range []string{"private SQL details", "password=secret", invalidPrivateID} {
					if strings.Contains(w.Body.String(), private) {
						t.Errorf("leaked %q", private)
					}
				}
				return
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("wrong success content type")
			}
			var got api.Trip
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("trip=%+v want=%+v", got, want)
			}
		})
	}
}
