package create_test

import (
	"context"
	"encoding/json"
	"errors"
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
	return api.HandlerWithOptions(testServer{handler: create.New(uc, limit)}, api.ChiServerOptions{ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
		domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
	}})
}

func TestHandlerCreateTrip(t *testing.T) {
	tests := []struct {
		name          string
		key           bool
		replay        bool
		err           error
		invalidResult string
		zeroPrice     bool
		wantStatus    int32
		wantCode      string
	}{
		{name: "created", wantStatus: 201},
		{name: "created with key", key: true, wantStatus: 201},
		{name: "free trip", zeroPrice: true, wantStatus: 201},
		{name: "replayed completed trip", key: true, replay: true, wantStatus: 200},
		{name: "busy driver", err: domaintrip.ErrDriverBusy, wantStatus: 409, wantCode: "driver_busy"},
		{name: "idempotency conflict", key: true, err: domaintrip.ErrIdempotencyConflict, wantStatus: 409, wantCode: "idempotency_conflict"},
		{name: "internal error", err: errors.New("private SQL details password=secret"), wantStatus: 500, wantCode: internalErrorCode},
		{name: "usecase validation", err: &domaintrip.ValidationError{Field: priceField, Reason: "must be non-negative"}, wantStatus: 400, wantCode: "invalid_request"},
		{name: "invalid returned trip id", invalidResult: "trip", wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned user id", invalidResult: "user", wantStatus: 500, wantCode: internalErrorCode},
		{name: "invalid returned driver id", invalidResult: "driver", wantStatus: 500, wantCode: internalErrorCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockusecaseContract(ctrl)
			id := uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
			userID := uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c")
			driverID := uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
			key := "7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1"
			expectedParams := model.CreateTripParams{UserId: domaintrip.UserId(userID.String()), DriverId: domaintrip.DriverId(driverID.String()), StartLatitude: 59.9398, StartLongitude: 30.3146, EndLatitude: 59.929, EndLongitude: 30.3626, Price: 1450}
			body := validBody
			if tt.zeroPrice {
				expectedParams.Price = 0
				body = strings.Replace(body, `"price":1450`, `"price":0`, 1)
			}
			if tt.key {
				expectedParams.IdempotencyKey = &key
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.key {
				req.Header.Set("Idempotency-Key", key)
			}
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
			result := domaintrip.NewTrip(domaintrip.TripId(id.String()), expectedParams.UserId, expectedParams.DriverId, 59.9398, 30.3146, 59.929, 30.3626, expectedParams.Price, now)
			if tt.replay {
				if err := result.Finish(now.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			want := api.Trip{Id: id, UserId: userID, DriverId: driverID, StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146}, EndPoint: api.Coordinates{Latitude: 59.929, Longitude: 30.3626}, Price: expectedParams.Price, Status: api.TripStatus(result.Status), StartedAt: now, FinishedAt: result.FinishedAt}
			switch tt.invalidResult {
			case "trip":
				result.Id = invalidPrivateID
			case "user":
				result.UserId = invalidPrivateID
			case "driver":
				result.DriverId = invalidPrivateID
			}
			if tt.err != nil {
				uc.EXPECT().CreateTrip(gomock.Cond(func(actual context.Context) bool { return actual.Done() == ctx.Done() }), expectedParams).Return(nil, false, tt.err)
			} else {
				uc.EXPECT().CreateTrip(gomock.Cond(func(actual context.Context) bool { return actual.Done() == ctx.Done() }), expectedParams).Return(result, !tt.replay, nil)
			}
			w := httptest.NewRecorder()
			routerFor(uc, 1048576).ServeHTTP(w, req)
			if w.Code != int(tt.wantStatus) {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			wantLocation := ""
			if tt.wantStatus == 201 {
				wantLocation = "/api/v1/trips/" + id.String()
			}
			if location := w.Header().Get("Location"); location != wantLocation {
				t.Errorf("Location=%q want=%q", location, wantLocation)
			}
			if tt.wantCode != "" {
				if w.Header().Get("Content-Type") != "application/problem+json" {
					t.Error("wrong problem content type")
				}
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tt.wantCode || problem.Status != tt.wantStatus || problem.Instance == nil || *problem.Instance != "/api/v1/trips" {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(tt.body))
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
			if got.Code != "invalid_request" || got.Status != 400 || got.Instance == nil || *got.Instance != "/api/v1/trips" {
				t.Errorf("problem=%+v", got)
			}
			if got.Detail == nil || !strings.HasPrefix(*got.Detail, tt.field+":") {
				t.Errorf("detail=%v want field %q", got.Detail, tt.field)
			}
		})
	}
}
