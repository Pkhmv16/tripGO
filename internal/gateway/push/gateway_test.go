package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/gateway/push"
)

func TestRequestPositionSendsCommand(t *testing.T) {
	candidate := trip.PositionCandidate{
		TripID:   "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111",
		DriverID: "8860b315-ec86-42eb-a17c-7c163d721ff5",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" {
			t.Errorf("request = %s %s, want POST /api/v1/push", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requestID, _ := payload["request_id"].(string)
		if _, err := uuid.Parse(requestID); err != nil {
			t.Errorf("request_id = %q, want UUID", requestID)
		}
		delete(payload, "request_id")
		want := map[string]any{
			"recipient_id": string(candidate.DriverID),
			"kind":         "REQUEST_POSITION",
			"data":         map[string]any{"trip_id": string(candidate.TripID)},
		}
		if !reflect.DeepEqual(payload, want) {
			t.Errorf("payload = %v, want %v", payload, want)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	gateway := newGateway(t, server.URL, time.Second)
	if err := gateway.RequestPosition(t.Context(), candidate); err != nil {
		t.Fatalf("RequestPosition: %v", err)
	}
}

func TestRequestPositionReturnsHTTPStatusAfterOneAttempt(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "bad request", status: http.StatusBadRequest},
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "unavailable", status: http.StatusServiceUnavailable},
		{name: "redirect", status: http.StatusTemporaryRedirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.Header().Set("Location", "/other")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)

			gateway := newGateway(t, server.URL, time.Second)
			err := gateway.RequestPosition(t.Context(), trip.PositionCandidate{})
			var statusErr *push.StatusError
			if !errors.As(err, &statusErr) || statusErr.StatusCode != tt.status {
				t.Fatalf("error = %v, want HTTP status %d", err, tt.status)
			}
			if count := attempts.Load(); count != 1 {
				t.Fatalf("attempts = %d, want 1", count)
			}
		})
	}
}

func TestRequestPositionUsesClientTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	gateway := newGateway(t, server.URL, 50*time.Millisecond)
	err := gateway.RequestPosition(t.Context(), trip.PositionCandidate{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func newGateway(t *testing.T, baseURL string, timeout time.Duration) *push.Gateway {
	t.Helper()
	gateway, err := push.New(baseURL, timeout, tracenoop.NewTracerProvider())
	if err != nil {
		t.Fatalf("create push gateway: %v", err)
	}
	t.Cleanup(gateway.CloseIdleConnections)
	return gateway
}
