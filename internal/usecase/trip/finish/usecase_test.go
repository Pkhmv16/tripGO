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

const (
	failureGet    = "get"
	failureUpdate = "update"
)

func TestUsecaseFinishTrip(t *testing.T) {
	dependencyErr := errors.New("dependency failure")
	tests := []struct {
		name    string
		status  domaintrip.TripStatus
		failAt  string
		wantErr error
	}{
		{name: "active trip", status: domaintrip.StatusActive},
		{name: "completed trip", status: domaintrip.StatusCompleted, wantErr: domaintrip.ErrTripAlreadyCompleted},
		{name: "unknown status", status: domaintrip.TripStatus("cancelled"), wantErr: domaintrip.ErrTripAlreadyCompleted},
		{name: "trip not found", failAt: failureGet, wantErr: domaintrip.ErrTripNotFound},
		{name: "read error", failAt: failureGet, wantErr: dependencyErr},
		{name: "update error", status: domaintrip.StatusActive, failAt: failureUpdate, wantErr: dependencyErr},
		{name: "history error", status: domaintrip.StatusActive, failAt: "history", wantErr: dependencyErr},
		{name: "transaction begin error", failAt: "begin", wantErr: dependencyErr},
		{name: "transaction commit error", status: domaintrip.StatusActive, failAt: "commit", wantErr: dependencyErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMocktripStatusChangeRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			log := NewMocklogger(ctrl)
			tracer, recorder := newTestTracer(t)
			uc := finish.New(repo, history, tx, log, tracer, metricnoop.Int64Counter{})
			ctx, parent := tracer.Start(context.Background(), "HTTP")
			defer parent.End()
			txCtx := context.WithValue(ctx, transactionContextKey{}, "transaction")
			txCompleted := false
			expectCompletionLog(t, log, tt.wantErr, dependencyErr, &txCompleted)
			startedAt := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
			original := &domaintrip.Trip{
				Id: "trip-id", UserId: "user-id", DriverId: "driver-id",
				StartLatitude: 59, StartLongitude: 30, EndLatitude: 60, EndLongitude: 31, Price: 1450,
				Status: tt.status, StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
			}
			if tt.status == domaintrip.StatusCompleted {
				original.FinishedAt = new(startedAt.Add(time.Hour))
			}
			snapshot := *original
			var updated *domaintrip.Trip
			if tt.failAt == "begin" {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(tt.wantErr)
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					err := fn(txCtx)
					txCompleted = true
					if tt.failAt == "commit" {
						if err != nil {
							t.Fatalf("callback error: %v", err)
						}
						return tt.wantErr
					}
					if !errors.Is(err, tt.wantErr) {
						t.Errorf("callback error = %v, want %v", err, tt.wantErr)
					}
					return err
				})
				if tt.failAt == failureGet {
					repo.EXPECT().GetForUpdate(txCtx, original.Id).Return(nil, tt.wantErr)
				} else {
					read := repo.EXPECT().GetForUpdate(txCtx, original.Id).Return(original, nil)
					if tt.status == domaintrip.StatusActive {
						update := repo.EXPECT().Update(txCtx, gomock.Any()).After(read).DoAndReturn(func(_ context.Context, got *domaintrip.Trip) error {
							if got.FinishedAt == nil {
								t.Fatal("Update received a trip without finished_at")
							}
							want := snapshot
							want.Status = domaintrip.StatusCompleted
							want.FinishedAt = got.FinishedAt
							want.UpdatedAt = *got.FinishedAt
							if !reflect.DeepEqual(got, &want) {
								t.Errorf("Update trip = %+v, want %+v", got, &want)
							}
							copied := *got
							updated = &copied
							if tt.failAt == failureUpdate {
								return tt.wantErr
							}
							return nil
						})
						if tt.failAt != failureUpdate {
							history.EXPECT().Save(txCtx, gomock.Any()).After(update).DoAndReturn(func(_ context.Context, change *domaintrip.TripStatusChange) error {
								if change.FromStatus == nil || *change.FromStatus != domaintrip.StatusActive || change.ToStatus != domaintrip.StatusCompleted {
									t.Errorf("wrong transition: %+v", change)
								}
								if change.TripId != original.Id || !change.ChangedAt.Equal(*updated.FinishedAt) {
									t.Errorf("history does not match update: %+v", change)
								}
								if tt.failAt == "history" {
									return tt.wantErr
								}
								return nil
							})
						}
					}
				}
			}
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
				if tt.status != domaintrip.StatusActive && !reflect.DeepEqual(original, &snapshot) {
					t.Error("rejected operation changed the trip")
				}
				return
			}
			if got == nil || updated == nil {
				t.Fatalf("success without result or update: got=%+v updated=%+v", got, updated)
			}
			if !reflect.DeepEqual(got, updated) {
				t.Errorf("result = %+v, saved trip = %+v", got, updated)
			}
			if got.FinishedAt.Before(before) || got.FinishedAt.After(after) {
				t.Errorf("finished_at = %v is outside the operation interval", got.FinishedAt)
			}
		})
	}
}

func expectCompletionLog(t *testing.T, log *Mocklogger, wantErr, dependencyErr error, txCompleted *bool) {
	t.Helper()
	if wantErr == nil {
		log.EXPECT().InfoContext(gomock.Any(), "trip completed", "trip_id", "trip-id").Do(func(context.Context, string, ...any) {
			if !*txCompleted {
				t.Error("success was logged before transaction completed")
			}
		})
	} else if errors.Is(wantErr, dependencyErr) {
		log.EXPECT().ErrorContext(gomock.Any(), "trip completion failed", "trip_id", "trip-id", "error", wantErr)
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
