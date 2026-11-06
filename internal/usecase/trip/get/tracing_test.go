package get_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/get"
)

func TestGetTripTracing(t *testing.T) {
	for _, wantErr := range []error{nil, domaintrip.ErrTripNotFound} {
		name := "success"
		if wantErr != nil {
			name = "not found"
		}
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() {
				if err := provider.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			tracer := provider.Tracer("test")
			ctx, parent := tracer.Start(context.Background(), "HTTP")
			defer parent.End()
			repo := NewMocktripRepository(gomock.NewController(t))
			repo.EXPECT().Get(gomock.Cond(func(ctx context.Context) bool {
				span := trace.SpanFromContext(ctx).SpanContext()
				return span.IsValid() && span.TraceID() == parent.SpanContext().TraceID() && span.SpanID() != parent.SpanContext().SpanID()
			}), domaintrip.TripId("trip-id")).Return(nil, wantErr)
			_, err := get.New(repo, tracer).GetTrip(ctx, "trip-id")
			if err != wantErr {
				t.Fatalf("GetTrip error = %v, want %v", err, wantErr)
			}
			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("ended spans = %d, want 1", len(spans))
			}
			span := spans[0]
			if span.Name() != "trip.get" || span.Parent().SpanID() != parent.SpanContext().SpanID() {
				t.Error("usecase span has wrong name or parent")
			}
			if (span.Status().Code == codes.Error) != (wantErr != nil) {
				t.Errorf("unexpected span status: %v", span.Status())
			}
		})
	}
}
