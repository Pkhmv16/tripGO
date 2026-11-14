package finish_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/finish"
)

type transactionContextKey struct{}

type fixture struct {
	t           *testing.T
	repo        *MocktripRepository
	history     *MocktripStatusChangeRepository
	tx          *MocktxManager
	log         *Mocklogger
	txCtx       context.Context
	original    *domaintrip.Trip
	snapshot    domaintrip.Trip
	updated     *domaintrip.Trip
	txCompleted bool
}

func (f *fixture) transaction(wantCallbackErr, result error) {
	f.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
		err := fn(f.txCtx)
		if !errors.Is(err, wantCallbackErr) {
			f.t.Fatalf("callback error = %v, want %v", err, wantCallbackErr)
		}
		f.txCompleted = true
		return result
	})
}

func (f *fixture) readTrip() *gomock.Call {
	return f.repo.EXPECT().GetForUpdate(f.txCtx, f.original.Id).Return(f.original, nil)
}

func (f *fixture) updateTrip(result error) *gomock.Call {
	return f.repo.EXPECT().Update(f.txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domaintrip.Trip) error {
		if got == nil || got.FinishedAt == nil {
			f.t.Fatal("Update received a trip without finished_at")
		}
		want := f.snapshot
		want.Status = domaintrip.StatusCompleted
		want.FinishedAt = got.FinishedAt
		want.UpdatedAt = *got.FinishedAt
		if !reflect.DeepEqual(got, &want) {
			f.t.Errorf("Update trip = %+v, want %+v", got, &want)
		}
		copied := *got
		f.updated = &copied
		return result
	})
}

func (f *fixture) saveHistory(result error) *gomock.Call {
	return f.history.EXPECT().Save(f.txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, change *domaintrip.TripStatusChange) error {
		if change.FromStatus == nil || *change.FromStatus != domaintrip.StatusActive || change.ToStatus != domaintrip.StatusCompleted {
			f.t.Errorf("wrong transition: %+v", change)
		}
		if change.TripId != f.original.Id || !change.ChangedAt.Equal(*f.updated.FinishedAt) {
			f.t.Errorf("history does not match update: %+v", change)
		}
		return result
	})
}

func (f *fixture) logSuccess() {
	f.log.EXPECT().InfoContext(gomock.Any(), "trip completed", "trip_id", "trip-id").Do(func(context.Context, string, ...any) {
		if !f.txCompleted {
			f.t.Error("success was logged before transaction completed")
		}
	})
}

func (f *fixture) logFailure(err error) {
	f.log.EXPECT().ErrorContext(gomock.Any(), "trip completion failed", "trip_id", "trip-id", "error", err)
}

func TestUsecaseFinishTrip(t *testing.T) {
	dependencyErr := errors.New("dependency failure")
	startedAt := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	active := *domaintrip.NewTrip("trip-id", "user-id", "driver-id", 59, 30, 60, 31, 1450, startedAt)
	completed := active
	completed.Status = domaintrip.StatusCompleted
	completed.FinishedAt = new(startedAt.Add(time.Hour))
	unknown := active
	unknown.Status = "cancelled"
	rejectStatus := func(f *fixture) {
		f.transaction(domaintrip.ErrTripAlreadyCompleted, domaintrip.ErrTripAlreadyCompleted)
		f.readTrip()
	}
	tests := []struct {
		name      string
		trip      domaintrip.Trip
		setup     func(*fixture)
		wantErr   error
		unchanged bool
	}{
		{name: "active trip", trip: active, setup: func(f *fixture) {
			f.transaction(nil, nil)
			gomock.InOrder(f.readTrip(), f.updateTrip(nil), f.saveHistory(nil))
			f.logSuccess()
		}},
		{name: "completed trip", trip: completed, setup: rejectStatus, wantErr: domaintrip.ErrTripAlreadyCompleted, unchanged: true},
		{name: "unknown status", trip: unknown, setup: rejectStatus, wantErr: domaintrip.ErrTripAlreadyCompleted, unchanged: true},
		{name: "trip not found", trip: active, unchanged: true, wantErr: domaintrip.ErrTripNotFound, setup: func(f *fixture) {
			f.transaction(domaintrip.ErrTripNotFound, domaintrip.ErrTripNotFound)
			f.repo.EXPECT().GetForUpdate(f.txCtx, f.original.Id).Return(nil, domaintrip.ErrTripNotFound)
		}},
		{name: "read error", trip: active, unchanged: true, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			f.repo.EXPECT().GetForUpdate(f.txCtx, f.original.Id).Return(nil, dependencyErr)
			f.logFailure(dependencyErr)
		}},
		{name: "update error", trip: active, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			gomock.InOrder(f.readTrip(), f.updateTrip(dependencyErr))
			f.logFailure(dependencyErr)
		}},
		{name: "history error", trip: active, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			gomock.InOrder(f.readTrip(), f.updateTrip(nil), f.saveHistory(dependencyErr))
			f.logFailure(dependencyErr)
		}},
		{name: "transaction begin error", trip: active, unchanged: true, wantErr: dependencyErr, setup: func(f *fixture) {
			f.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(dependencyErr)
			f.logFailure(dependencyErr)
		}},
		{name: "transaction commit error", trip: active, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(nil, dependencyErr)
			gomock.InOrder(f.readTrip(), f.updateTrip(nil), f.saveHistory(nil))
			f.logFailure(dependencyErr)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tracer, recorder := newTestTracer(t)
			ctx, parent := tracer.Start(context.Background(), "HTTP")
			defer parent.End()
			original := tt.trip
			f := &fixture{
				t: t, repo: NewMocktripRepository(ctrl), history: NewMocktripStatusChangeRepository(ctrl),
				tx: NewMocktxManager(ctrl), log: NewMocklogger(ctrl), original: &original, snapshot: tt.trip,
				txCtx: context.WithValue(ctx, transactionContextKey{}, "transaction"),
			}
			tt.setup(f)
			uc := finish.New(f.repo, f.history, f.tx, f.log, tracer, metricnoop.Int64Counter{})
			before := time.Now()
			got, err := uc.FinishTrip(ctx, string(original.Id))
			assertUsecaseSpan(t, recorder, parent, tt.wantErr)
			after := time.Now()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FinishTrip() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("failure returned a trip: %+v", got)
				}
				if tt.unchanged && !reflect.DeepEqual(original, f.snapshot) {
					t.Error("rejected operation changed the trip")
				}
				return
			}
			if got == nil || f.updated == nil {
				t.Fatalf("success without result or update: got=%+v updated=%+v", got, f.updated)
			}
			if !reflect.DeepEqual(got, f.updated) {
				t.Errorf("result = %+v, saved trip = %+v", got, f.updated)
			}
			if got.FinishedAt.Before(before) || got.FinishedAt.After(after) {
				t.Errorf("finished_at = %v is outside the operation interval", got.FinishedAt)
			}
		})
	}
}

func newTestTracer(t *testing.T) (trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	tracer := provider.Tracer("test")
	return tracer, recorder
}

func assertUsecaseSpan(t *testing.T, recorder *tracetest.SpanRecorder, parent trace.Span, wantErr error) {
	t.Helper()
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "trip.finish" || span.Parent().SpanID() != parent.SpanContext().SpanID() || span.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("usecase span is not a child of the HTTP span: %v", span)
	}
	if wantErr != nil && (span.Status().Code != codes.Error || len(span.Events()) == 0) {
		t.Error("failed usecase span has no error status or error event")
	}
	if wantErr == nil && span.Status().Code == codes.Error {
		t.Error("successful usecase span has error status")
	}
}
