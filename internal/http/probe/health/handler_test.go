package health_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/probe/health"
)

func TestHandlerHealth(t *testing.T) {
	for _, tt := range []struct {
		name    string
		prepare func(context.CancelFunc)
	}{
		{name: "alive", prepare: func(context.CancelFunc) {}},
		{name: "cancelled request", prepare: func(cancel context.CancelFunc) { cancel() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			tt.prepare(cancel)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			health.New().Health(w, req)
			if w.Code != 200 {
				t.Errorf("status=%d want=200", w.Code)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("wrong content type")
			}
			var body api.HealthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != api.Ok {
				t.Errorf("health=%+v", body)
			}
		})
	}
}
