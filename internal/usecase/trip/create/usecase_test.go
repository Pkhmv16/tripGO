package create_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/model"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/create"
)

type transactionContextKey struct{}

const (
	failureCreate  = "create"
	failureHistory = "history"
	failureLookup  = "lookup"
)

func createParams() model.CreateTripParams {
	return model.CreateTripParams{
		UserId: "user-id", DriverId: "driver-id",
		StartLatitude: 59.9398, StartLongitude: 30.3146,
		EndLatitude: 59.929, EndLongitude: 30.3626, Price: 1450,
	}
}

func TestUsecaseCreateTrip(t *testing.T) {
	dependencyErr := errors.New("dependency failure")
	tests := []struct {
		name    string
		withKey bool
		failAt  string
		wantErr error
	}{
		{name: "success without key"},
		{name: "success with key", withKey: true},
		{name: "driver busy", failAt: failureCreate, wantErr: domaintrip.ErrDriverBusy},
		{name: "create error", failAt: failureCreate, wantErr: dependencyErr},
		{name: "history error", withKey: true, failAt: failureHistory, wantErr: dependencyErr},
		{name: "key lookup error", withKey: true, failAt: failureLookup, wantErr: dependencyErr},
		{name: "key save error", withKey: true, failAt: "key", wantErr: dependencyErr},
		{name: "transaction begin error", withKey: true, failAt: "begin", wantErr: dependencyErr},
		{name: "transaction commit error", withKey: true, failAt: "commit", wantErr: dependencyErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMocktripStatusChangeRepository(ctrl)
			keys := NewMockidempotencyRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			ids := NewMockidGenerator(ctrl)
			log := NewMocklogger(ctrl)
			tracer, recorder := newTestTracer(t)
			uc := create.New(repo, history, keys, tx, ids, log, tracer, metricnoop.Int64Counter{})
			ctx, parent := tracer.Start(context.Background(), "HTTP")
			defer parent.End()
			txCtx := context.WithValue(ctx, transactionContextKey{}, "transaction")
			txCompleted := false
			expectCreationLog(t, log, tt.wantErr, dependencyErr, &txCompleted)
			p := createParams()
			key := "7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1"
			if tt.withKey {
				p.IdempotencyKey = &key
			}
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
				var calls []any
				if tt.withKey {
					var lookupErr error
					if tt.failAt == failureLookup {
						lookupErr = tt.wantErr
					}
					calls = append(calls, keys.EXPECT().FindForCreate(txCtx, key).Return(nil, lookupErr))
				}
				if tt.failAt != failureLookup {
					calls = append(calls, ids.EXPECT().Generate().Return("trip-id"))
					var stored *domaintrip.Trip
					calls = append(calls, repo.EXPECT().Create(txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domaintrip.Trip) error {
						assertCreatedTrip(t, got, p)
						stored = got
						if tt.failAt == failureCreate {
							return tt.wantErr
						}
						return nil
					}))
					if tt.failAt != failureCreate {
						calls = append(calls, history.EXPECT().Save(txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, change *domaintrip.TripStatusChange) error {
							if change.TripId != stored.Id || change.FromStatus != nil || change.ToStatus != domaintrip.StatusActive || !change.ChangedAt.Equal(stored.StartedAt) {
								t.Errorf("initial history = %+v", change)
							}
							if tt.failAt == failureHistory {
								return tt.wantErr
							}
							return nil
						}))
						if tt.withKey && tt.failAt != failureHistory {
							calls = append(calls, keys.EXPECT().Save(txCtx, key, gomock.Any(), domaintrip.TripId("trip-id")).DoAndReturn(func(_ context.Context, _ string, hash []byte, _ domaintrip.TripId) error {
								if len(hash) != 32 {
									t.Errorf("request hash length = %d", len(hash))
								}
								if tt.failAt == "key" {
									return tt.wantErr
								}
								return nil
							}))
						}
					}
				}
				gomock.InOrder(calls...)
			}
			before := time.Now()
			got, created, err := uc.CreateTrip(ctx, p)
			assertUsecaseSpan(t, recorder, parent, tt.wantErr)
			after := time.Now()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateTrip() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if got != nil || created {
					t.Fatalf("failure returned trip=%+v created=%v", got, created)
				}
				return
			}
			if got == nil || !created {
				t.Fatalf("success returned trip=%+v created=%v", got, created)
			}
			if got.Id != "trip-id" || got.StartedAt.Before(before) || got.StartedAt.After(after) {
				t.Errorf("unexpected result: %+v", got)
			}
		})
	}
}

func TestUsecaseCreateTripIdempotency(t *testing.T) {
	dependencyErr := errors.New("get saved trip failed")
	tests := []struct {
		name     string
		change   func(*model.CreateTripParams)
		getErr   error
		conflict bool
	}{
		{name: "same parameters"},
		{name: "same parameters with another key", change: func(p *model.CreateTripParams) { p.IdempotencyKey = new("87925466-6354-4ae0-b084-c41263216ea7") }},
		{name: "equivalent negative zero", change: func(p *model.CreateTripParams) { p.StartLatitude = math.Copysign(0, -1) }},
		{name: "changed price", change: func(p *model.CreateTripParams) { p.Price++ }, conflict: true},
		{name: "changed driver", change: func(p *model.CreateTripParams) { p.DriverId = "another-driver" }, conflict: true},
		{name: "changed coordinates", change: func(p *model.CreateTripParams) { p.EndLongitude++ }, conflict: true},
		{name: "saved trip read error", getErr: dependencyErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			history := NewMocktripStatusChangeRepository(ctrl)
			keys := NewMockidempotencyRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			ids := NewMockidGenerator(ctrl)
			log := NewMocklogger(ctrl)
			uc := create.New(repo, history, keys, tx, ids, log, noop.NewTracerProvider().Tracer("test"), metricnoop.Int64Counter{})
			ctx := context.Background()
			txCtx := context.WithValue(ctx, transactionContextKey{}, "transaction")
			p := createParams()
			p.StartLatitude = 0
			p.IdempotencyKey = new("7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1")
			log.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", "trip-id")
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error { return fn(txCtx) }).Times(2)
			var savedHash []byte
			gomock.InOrder(
				keys.EXPECT().FindForCreate(txCtx, *p.IdempotencyKey).Return(nil, nil),
				ids.EXPECT().Generate().Return("trip-id"),
				repo.EXPECT().Create(txCtx, gomock.Any()).Return(nil),
				history.EXPECT().Save(txCtx, gomock.Any()).Return(nil),
				keys.EXPECT().Save(txCtx, *p.IdempotencyKey, gomock.Any(), domaintrip.TripId("trip-id")).DoAndReturn(func(_ context.Context, _ string, hash []byte, _ domaintrip.TripId) error {
					savedHash = bytes.Clone(hash)
					return nil
				}),
			)
			original, created, err := uc.CreateTrip(ctx, p)
			if err != nil || !created || original == nil {
				t.Fatalf("initial creation: trip=%+v created=%v err=%v", original, created, err)
			}
			if tt.change != nil {
				tt.change(&p)
			}
			original.Status = domaintrip.StatusCompleted
			original.FinishedAt = new(original.StartedAt.Add(time.Hour))
			record := &domaintrip.IdempotencyRecord{RequestHash: savedHash, TripId: original.Id}
			lookup := keys.EXPECT().FindForCreate(txCtx, *p.IdempotencyKey).Return(record, nil)
			wantErr := tt.getErr
			if tt.getErr != nil {
				log.EXPECT().ErrorContext(gomock.Any(), "trip creation failed", "error", tt.getErr)
			}
			if tt.conflict {
				wantErr = domaintrip.ErrIdempotencyConflict
			} else {
				repo.EXPECT().Get(txCtx, original.Id).Return(original, tt.getErr).After(lookup)
			}
			got, isNew, err := uc.CreateTrip(ctx, p)
			if !errors.Is(err, wantErr) {
				t.Fatalf("replay error = %v, want %v", err, wantErr)
			}
			if isNew {
				t.Error("replay reported a new trip")
			}
			if wantErr != nil {
				if got != nil {
					t.Errorf("failure returned trip: %+v", got)
				}
				return
			}
			if !reflect.DeepEqual(got, original) {
				t.Errorf("replay = %+v, want %+v", got, original)
			}
		})
	}
}

func assertCreatedTrip(t *testing.T, got *domaintrip.Trip, p model.CreateTripParams) {
	t.Helper()
	if got == nil {
		t.Fatal("Create received a nil trip")
	}
	expected := &domaintrip.Trip{
		Id: "trip-id", UserId: p.UserId, DriverId: p.DriverId,
		StartLatitude: p.StartLatitude, StartLongitude: p.StartLongitude,
		EndLatitude: p.EndLatitude, EndLongitude: p.EndLongitude, Price: p.Price,
		Status: domaintrip.StatusActive, StartedAt: got.StartedAt, CreatedAt: got.StartedAt, UpdatedAt: got.StartedAt,
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("created trip = %+v, want %+v", got, expected)
	}
	if got.StartedAt.IsZero() {
		t.Error("start time is zero")
	}
}

func expectCreationLog(t *testing.T, log *Mocklogger, wantErr, dependencyErr error, txCompleted *bool) {
	t.Helper()
	if wantErr == nil {
		log.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", "trip-id").Do(func(context.Context, string, ...any) {
			if !*txCompleted {
				t.Error("success was logged before transaction completed")
			}
		})
	} else if errors.Is(wantErr, dependencyErr) {
		log.EXPECT().ErrorContext(gomock.Any(), "trip creation failed", "error", wantErr)
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
	if span.Name() != "trip.create" || span.Parent().SpanID() != parent.SpanContext().SpanID() || span.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("usecase span is not a child of the HTTP span: %v", span)
	}
	if wantErr != nil && (span.Status().Code != codes.Error || len(span.Events()) == 0) {
		t.Error("failed usecase span has no error status or error event")
	}
	if wantErr == nil && span.Status().Code == codes.Error {
		t.Error("successful usecase span has error status")
	}
}
