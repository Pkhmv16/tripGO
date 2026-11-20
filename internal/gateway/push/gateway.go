package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Gateway struct {
	endpoint  string
	client    *http.Client
	transport *http.Transport
}

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.StatusCode)
}

type positionRequest struct {
	RequestID   string        `json:"request_id"`
	RecipientID trip.DriverId `json:"recipient_id"`
	Kind        string        `json:"kind"`
	Data        positionData  `json:"data"`
}

type positionData struct {
	TripID trip.TripId `json:"trip_id"`
}

func New(baseURL string, timeout time.Duration, provider trace.TracerProvider) (*Gateway, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("PUSH_HTTP_URL must be an HTTP or HTTPS URL")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{
		Timeout: timeout,
		Transport: otelhttp.NewTransport(transport,
			otelhttp.WithTracerProvider(provider),
			otelhttp.WithPropagators(propagation.TraceContext{}),
		),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Gateway{
		endpoint:  base.JoinPath("/api/v1/push").String(),
		client:    client,
		transport: transport,
	}, nil
}

func (g *Gateway) RequestPosition(ctx context.Context, candidate trip.PositionCandidate) error {
	payload, err := json.Marshal(positionRequest{
		RequestID:   uuid.NewString(),
		RecipientID: candidate.DriverID,
		Kind:        "REQUEST_POSITION",
		Data:        positionData{TripID: candidate.TripID},
	})
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusAccepted {
		return &StatusError{StatusCode: resp.StatusCode}
	}
	return nil
}

func (g *Gateway) CloseIdleConnections() {
	g.transport.CloseIdleConnections()
}
