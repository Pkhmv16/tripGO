package ready_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/probe/ready"
)

type requestKey struct{}

func TestHandlerReady(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		cancelled  bool
		wantStatus int
		wantHealth api.HealthResponseStatus
	}{
		{name: "database available", wantStatus: 200, wantHealth: api.Ok},
		{name: "database unavailable", err: errors.New("private database details"), wantStatus: 503, wantHealth: api.Unavailable},
		{name: "database timeout", err: context.DeadlineExceeded, wantStatus: 503, wantHealth: api.Unavailable},
		{name: "request cancelled", err: context.Canceled, cancelled: true, wantStatus: 503, wantHealth: api.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewMockreadinessChecker(gomock.NewController(t))
			requestCtx, cancel := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "request-value"))
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			timeout := time.Second
			var pingCtx context.Context
			before := time.Now()
			checker.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				pingCtx = ctx
				if ctx.Value(requestKey{}) != "request-value" {
					t.Error("request context was not propagated")
				}
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("Ping context has no deadline")
				}
				if deadline.Before(before.Add(timeout)) || deadline.After(time.Now().Add(timeout)) {
					t.Errorf("unexpected deadline: %v", deadline)
				}
				if tt.cancelled && !errors.Is(ctx.Err(), context.Canceled) {
					t.Errorf("context error=%v", ctx.Err())
				}
				return tt.err
			})
			req := httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(requestCtx)
			w := httptest.NewRecorder()
			ready.New(checker, timeout).Ready(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("status=%d want=%d", w.Code, tt.wantStatus)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("wrong content type")
			}
			var body api.HealthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.wantHealth {
				t.Errorf("health=%v want=%v", body.Status, tt.wantHealth)
			}
			if pingCtx == nil || !errors.Is(pingCtx.Err(), context.Canceled) {
				t.Error("Ping context was not released after handler returned")
			}
		})
	}
}
