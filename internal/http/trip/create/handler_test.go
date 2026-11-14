package create_test

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
	"github.com/Pkhmv16/tripGO/internal/http/trip/create"
	"github.com/Pkhmv16/tripGO/internal/model"
)

const tripsPath = "/api/v1/trips"

const validBody = `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.929,"longitude":30.3626},"price":1450}`

type testServer struct {
	api.Unimplemented
	handler *create.Handler
}

const (
	internalErrorCode = "internal_error"
	priceField        = "price"
	invalidPrivateID  = "invalid-private-id"
	bodyField         = "body"
	startPointField   = "start_point"
)

func (s testServer) CreateTrip(w http.ResponseWriter, r *http.Request, p api.CreateTripParams) {
	s.handler.CreateTrip(w, r, p)
}

func routerFor(uc *MockusecaseContract, limit int64) http.Handler {
	return api.HandlerWithOptions(testServer{handler: create.New(uc, limit, slog.New(slog.DiscardHandler))}, api.ChiServerOptions{ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
		domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
	}})
}

func TestHandlerCreateTrip(t *testing.T) {
	id := uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
	userID := uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c")
	driverID := uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
	key := "7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1"
	path := tripsPath
	location := path + "/" + id.String()
	params := model.CreateTripParams{UserId: domaintrip.UserId(userID.String()), DriverId: domaintrip.DriverId(driverID.String()), StartLatitude: 59.9398, StartLongitude: 30.3146, EndLatitude: 59.929, EndLongitude: 30.3626, Price: 1450}
	keyed := params
	keyed.IdempotencyKey = &key
	free := params
	free.Price = 0
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	result := domaintrip.NewTrip(domaintrip.TripId(id.String()), params.UserId, params.DriverId, params.StartLatitude, params.StartLongitude, params.EndLatitude, params.EndLongitude, params.Price, now)
	freeTrip := *result
	freeTrip.Price = 0
	completed := *result
	if err := completed.Finish(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	badTrip := *result
	badTrip.Id = invalidPrivateID
	badUser := *result
	badUser.UserId = invalidPrivateID
	badDriver := *result
	badDriver.DriverId = invalidPrivateID
	want := api.Trip{Id: id, UserId: userID, DriverId: driverID, StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146}, EndPoint: api.Coordinates{Latitude: 59.929, Longitude: 30.3626}, Price: 1450, Status: api.TripStatus(domaintrip.StatusActive), StartedAt: now}
	wantFree := want
	wantFree.Price = 0
	wantCompleted := want
	wantCompleted.Status = api.TripStatus(domaintrip.StatusCompleted)
	wantCompleted.FinishedAt = completed.FinishedAt
	tests := []struct {
		name                   string
		body, key              string
		params                 model.CreateTripParams
		result                 *domaintrip.Trip
		created                bool
		err                    error
		wantStatus             int32
		wantCode, wantLocation string
		wantTrip               api.Trip
	}{
		{name: "created", body: validBody, params: params, result: result, created: true, wantStatus: 201, wantLocation: location, wantTrip: want},
		{name: "created with key", body: validBody, key: key, params: keyed, result: result, created: true, wantStatus: 201, wantLocation: location, wantTrip: want},
		{name: "free trip", body: strings.Replace(validBody, `"price":1450`, `"price":0`, 1), params: free, result: &freeTrip, created: true, wantStatus: 201, wantLocation: location, wantTrip: wantFree},
		{name: "replayed completed trip", body: validBody, key: key, params: keyed, result: &completed, wantStatus: 200, wantTrip: wantCompleted},
		{name: "busy driver", body: validBody, params: params, err: domaintrip.ErrDriverBusy, wantStatus: 409, wantCode: "driver_busy"},
		{name: "idempotency conflict", body: validBody, key: key, params: keyed, err: domaintrip.ErrIdempotencyConflict, wantStatus: 409, wantCode: "idempotency_conflict"},
		{name: "internal error", body: validBody, params: params, err: errors.New("private SQL details password=secret"), wantStatus: 500, wantCode: internalErrorCode},
		{name: "usecase validation", body: validBody, params: params, err: &domaintrip.ValidationError{Field: priceField, Reason: "must be non-negative"}, wantStatus: 400, wantCode: "invalid_request"},
		{name: "invalid returned trip id", body: validBody, params: params, result: &badTrip, created: true, wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned user id", body: validBody, params: params, result: &badUser, created: true, wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned driver id", body: validBody, params: params, result: &badDriver, created: true, wantStatus: 500, wantCode: internalErrorCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockusecaseContract(ctrl)
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.key != "" {
				req.Header.Set("Idempotency-Key", tt.key)
			}
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			uc.EXPECT().CreateTrip(gomock.Cond(func(actual context.Context) bool { return actual.Done() == ctx.Done() }), tt.params).Return(tt.result, tt.created, tt.err)
			w := httptest.NewRecorder()
			routerFor(uc, 1048576).ServeHTTP(w, req)
			if w.Code != int(tt.wantStatus) {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			if location := w.Header().Get("Location"); location != tt.wantLocation {
				t.Errorf("Location=%q want=%q", location, tt.wantLocation)
			}
			if tt.wantCode != "" {
				if w.Header().Get("Content-Type") != "application/problem+json" {
					t.Error("wrong problem content type")
				}
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tt.wantCode || problem.Status != tt.wantStatus || problem.Instance == nil || *problem.Instance != tripsPath {
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
			if !reflect.DeepEqual(got, tt.wantTrip) {
				t.Errorf("trip=%+v want=%+v", got, tt.wantTrip)
			}
		})
	}
}

type invalidRequestCase struct {
	name, body, header, field string
	limit                     int64
}

func TestHandlerCreateTripInvalidRequest(t *testing.T) {
	tests := make([]invalidRequestCase, 0, 40)
	tests = append(tests, []invalidRequestCase{
		{name: "empty body", field: bodyField},
		{name: "malformed JSON", body: `{`, field: bodyField},
		{name: "null", body: `null`, field: bodyField},
		{name: "array", body: `[]`, field: bodyField},
		{name: "two documents", body: validBody + validBody, field: bodyField},
		{name: "trailing garbage", body: validBody + `!`, field: bodyField},
		{name: "body too large", body: validBody, field: bodyField, limit: 16},
		{name: "invalid key", body: validBody, header: "invalid", field: "request"},
		{name: "unknown field", body: strings.TrimSuffix(validBody, "}") + `,"extra":1}`, field: "extra"},
		{name: "wrong case field", body: strings.TrimSuffix(validBody, "}") + `,"PRICE":1}`, field: "PRICE"},
		{name: "unknown nested field", body: strings.Replace(validBody, `"latitude":59.9398`, `"latitude":59.9398,"LATITUDE":1`, 1), field: "start_point.LATITUDE"},
		{name: "invalid user UUID", body: strings.Replace(validBody, "5cb72c04-7650-45c9-a79b-bcdba0631e0c", "invalid", 1), field: bodyField},
		{name: "zero user UUID", body: strings.Replace(validBody, "5cb72c04-7650-45c9-a79b-bcdba0631e0c", uuid.Nil.String(), 1), field: "user_id"},
		{name: "zero driver UUID", body: strings.Replace(validBody, "8860b315-ec86-42eb-a17c-7c163d721ff5", uuid.Nil.String(), 1), field: "driver_id"},
		{name: "negative price", body: strings.Replace(validBody, `"price":1450`, `"price":-1`, 1), field: priceField},
		{name: "fractional price", body: strings.Replace(validBody, `"price":1450`, `"price":1.5`, 1), field: bodyField},
		{name: "string price", body: strings.Replace(validBody, `"price":1450`, `"price":"1450"`, 1), field: bodyField},
		{name: "invalid start latitude", body: strings.Replace(validBody, `"latitude":59.9398`, `"latitude":91`, 1), field: "start_point.latitude"},
		{name: "invalid start longitude", body: strings.Replace(validBody, `"longitude":30.3146`, `"longitude":-181`, 1), field: "start_point.longitude"},
		{name: "invalid end latitude", body: strings.Replace(validBody, `"latitude":59.929`, `"latitude":-91`, 1), field: "end_point.latitude"},
		{name: "invalid end longitude", body: strings.Replace(validBody, `"longitude":30.3626`, `"longitude":181`, 1), field: "end_point.longitude"},
		{name: "start point array", body: strings.Replace(validBody, `{"latitude":59.9398,"longitude":30.3146}`, `[]`, 1), field: startPointField},
	}...)

	for _, field := range []string{"user_id", "driver_id", startPointField, "end_point", priceField} {
		for _, null := range []bool{false, true} {
			var object map[string]json.RawMessage
			if err := json.Unmarshal([]byte(validBody), &object); err != nil {
				t.Fatal(err)
			}
			name := "missing " + field
			if null {
				object[field] = json.RawMessage("null")
				name = "null " + field
			} else {
				delete(object, field)
			}
			body, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			tests = append(tests, invalidRequestCase{name: name, body: string(body), field: field})
		}
	}
	for _, point := range []string{startPointField, "end_point"} {
		for _, field := range []string{"latitude", "longitude"} {
			for _, null := range []bool{false, true} {
				var object map[string]json.RawMessage
				if err := json.Unmarshal([]byte(validBody), &object); err != nil {
					t.Fatal(err)
				}
				var coordinates map[string]json.RawMessage
				if err := json.Unmarshal(object[point], &coordinates); err != nil {
					t.Fatal(err)
				}
				name := "missing " + point + "." + field
				if null {
					coordinates[field] = json.RawMessage("null")
					name = "null " + point + "." + field
				} else {
					delete(coordinates, field)
				}
				value, err := json.Marshal(coordinates)
				if err != nil {
					t.Fatal(err)
				}
				object[point] = value
				body, err := json.Marshal(object)
				if err != nil {
					t.Fatal(err)
				}
				tests = append(tests, invalidRequestCase{name: name, body: string(body), field: point + "." + field})
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewMockusecaseContract(gomock.NewController(t))
			limit := tt.limit
			if limit == 0 {
				limit = 1048576
			}
			req := httptest.NewRequest(http.MethodPost, tripsPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.header != "" {
				req.Header.Set("Idempotency-Key", tt.header)
			}
			w := httptest.NewRecorder()
			routerFor(uc, limit).ServeHTTP(w, req)
			if w.Code != 400 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Type") != "application/problem+json" {
				t.Error("wrong content type")
			}
			var got api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Code != "invalid_request" || got.Status != 400 || got.Instance == nil || *got.Instance != tripsPath {
				t.Errorf("problem=%+v", got)
			}
			if got.Detail == nil || !strings.HasPrefix(*got.Detail, tt.field+":") {
				t.Errorf("detail=%v want field %q", got.Detail, tt.field)
			}
		})
	}
}
