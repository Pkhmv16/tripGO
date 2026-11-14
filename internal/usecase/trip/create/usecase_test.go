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

const testKey = "7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1"

func createParams() model.CreateTripParams {
	return model.CreateTripParams{
		UserId: "user-id", DriverId: "driver-id",
		StartLatitude: 59.9398, StartLongitude: 30.3146,
		EndLatitude: 59.929, EndLongitude: 30.3626, Price: 1450,
	}
}

type fixture struct {
	t           *testing.T
	repo        *MocktripRepository
	history     *MocktripStatusChangeRepository
	keys        *MockidempotencyRepository
	tx          *MocktxManager
	ids         *MockidGenerator
	log         *Mocklogger
	txCtx       context.Context
	params      model.CreateTripParams
	stored      *domaintrip.Trip
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

func (f *fixture) insertTrip(result error) *gomock.Call {
	return f.repo.EXPECT().Create(f.txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domaintrip.Trip) error {
		assertCreatedTrip(f.t, got, f.params)
		f.stored = got
		return result
	})
}

func (f *fixture) saveHistory(result error) *gomock.Call {
	return f.history.EXPECT().Save(f.txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, change *domaintrip.TripStatusChange) error {
		if change.TripId != f.stored.Id || change.FromStatus != nil || change.ToStatus != domaintrip.StatusActive || !change.ChangedAt.Equal(f.stored.StartedAt) {
			f.t.Errorf("initial history = %+v", change)
		}
		return result
	})
}

func (f *fixture) saveKey(result error) *gomock.Call {
	return f.keys.EXPECT().Save(f.txCtx, testKey, gomock.Any(), domaintrip.TripId("trip-id")).DoAndReturn(func(_ context.Context, _ string, hash []byte, _ domaintrip.TripId) error {
		if len(hash) != 32 {
			f.t.Errorf("request hash length = %d", len(hash))
		}
		return result
	})
}

func (f *fixture) logSuccess() {
	f.log.EXPECT().InfoContext(gomock.Any(), "trip created", "trip_id", "trip-id").Do(func(context.Context, string, ...any) {
		if !f.txCompleted {
			f.t.Error("success was logged before transaction completed")
		}
	})
}

func (f *fixture) logFailure(err error) {
	f.log.EXPECT().ErrorContext(gomock.Any(), "trip creation failed", "error", err)
}

func TestUsecaseCreateTrip(t *testing.T) {
	dependencyErr := errors.New("dependency failure")
	withoutKey := createParams()
	withKey := withoutKey
	withKey.IdempotencyKey = new(testKey)
	tests := []struct {
		name    string
		params  model.CreateTripParams
		setup   func(*fixture)
		wantErr error
	}{
		{name: "success without key", params: withoutKey, setup: func(f *fixture) {
			f.transaction(nil, nil)
			gomock.InOrder(f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(nil), f.saveHistory(nil))
			f.logSuccess()
		}},
		{name: "success with key", params: withKey, setup: func(f *fixture) {
			f.transaction(nil, nil)
			gomock.InOrder(f.keys.EXPECT().FindForCreate(f.txCtx, testKey).Return(nil, nil),
				f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(nil), f.saveHistory(nil), f.saveKey(nil))
			f.logSuccess()
		}},
		{name: "driver busy", params: withoutKey, wantErr: domaintrip.ErrDriverBusy, setup: func(f *fixture) {
			f.transaction(domaintrip.ErrDriverBusy, domaintrip.ErrDriverBusy)
			gomock.InOrder(f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(domaintrip.ErrDriverBusy))
		}},
		{name: "create error", params: withoutKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			gomock.InOrder(f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(dependencyErr))
			f.logFailure(dependencyErr)
		}},
		{name: "history error", params: withKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			gomock.InOrder(f.keys.EXPECT().FindForCreate(f.txCtx, testKey).Return(nil, nil),
				f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(nil), f.saveHistory(dependencyErr))
			f.logFailure(dependencyErr)
		}},
		{name: "key lookup error", params: withKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			f.keys.EXPECT().FindForCreate(f.txCtx, testKey).Return(nil, dependencyErr)
			f.logFailure(dependencyErr)
		}},
		{name: "key save error", params: withKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(dependencyErr, dependencyErr)
			gomock.InOrder(f.keys.EXPECT().FindForCreate(f.txCtx, testKey).Return(nil, nil),
				f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(nil), f.saveHistory(nil), f.saveKey(dependencyErr))
			f.logFailure(dependencyErr)
		}},
		{name: "transaction begin error", params: withKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(dependencyErr)
			f.logFailure(dependencyErr)
		}},
		{name: "transaction commit error", params: withKey, wantErr: dependencyErr, setup: func(f *fixture) {
			f.transaction(nil, dependencyErr)
			gomock.InOrder(f.keys.EXPECT().FindForCreate(f.txCtx, testKey).Return(nil, nil),
				f.ids.EXPECT().Generate().Return("trip-id"), f.insertTrip(nil), f.saveHistory(nil), f.saveKey(nil))
			f.logFailure(dependencyErr)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tracer, recorder := newTestTracer(t)
			ctx, parent := tracer.Start(context.Background(), "HTTP")
			defer parent.End()
			f := &fixture{
				t: t, repo: NewMocktripRepository(ctrl), history: NewMocktripStatusChangeRepository(ctrl),
				keys: NewMockidempotencyRepository(ctrl), tx: NewMocktxManager(ctrl), ids: NewMockidGenerator(ctrl),
				log: NewMocklogger(ctrl), txCtx: context.WithValue(ctx, transactionContextKey{}, "transaction"), params: tt.params,
			}
			tt.setup(f)
			uc := create.New(f.repo, f.history, f.keys, f.tx, f.ids, f.log, tracer, metricnoop.Int64Counter{})
			before := time.Now()
			got, created, err := uc.CreateTrip(ctx, tt.params)
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
	initial := createParams()
	initial.StartLatitude = 0
	initial.IdempotencyKey = new(testKey)
	replay := func(change func(*model.CreateTripParams)) model.CreateTripParams {
		p := initial
		change(&p)
		return p
	}
	readTrip := func(f *fixture, lookup *gomock.Call) {
		f.repo.EXPECT().Get(f.txCtx, f.stored.Id).Return(f.stored, nil).After(lookup)
	}
	skipRead := func(*fixture, *gomock.Call) {}
	tests := []struct {
		name    string
		params  model.CreateTripParams
		setup   func(*fixture, *gomock.Call)
		wantErr error
	}{
		{name: "same parameters", params: initial, setup: readTrip},
		{name: "same parameters with another key", params: replay(func(p *model.CreateTripParams) { p.IdempotencyKey = new("87925466-6354-4ae0-b084-c41263216ea7") }), setup: readTrip},
		{name: "equivalent negative zero", params: replay(func(p *model.CreateTripParams) { p.StartLatitude = math.Copysign(0, -1) }), setup: readTrip},
		{name: "changed price", params: replay(func(p *model.CreateTripParams) { p.Price++ }), setup: skipRead, wantErr: domaintrip.ErrIdempotencyConflict},
		{name: "changed driver", params: replay(func(p *model.CreateTripParams) { p.DriverId = "another-driver" }), setup: skipRead, wantErr: domaintrip.ErrIdempotencyConflict},
		{name: "changed coordinates", params: replay(func(p *model.CreateTripParams) { p.EndLongitude++ }), setup: skipRead, wantErr: domaintrip.ErrIdempotencyConflict},
		{name: "saved trip read error", params: initial, wantErr: dependencyErr, setup: func(f *fixture, lookup *gomock.Call) {
			f.repo.EXPECT().Get(f.txCtx, f.stored.Id).Return(nil, dependencyErr).After(lookup)
			f.logFailure(dependencyErr)
		}},
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
			p := initial
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
			original.Status = domaintrip.StatusCompleted
			original.FinishedAt = new(original.StartedAt.Add(time.Hour))
			record := &domaintrip.IdempotencyRecord{RequestHash: savedHash, TripId: original.Id}
			lookup := keys.EXPECT().FindForCreate(txCtx, *tt.params.IdempotencyKey).Return(record, nil)
			tt.setup(&fixture{repo: repo, log: log, txCtx: txCtx, stored: original}, lookup)
			wantErr := tt.wantErr
			got, isNew, err := uc.CreateTrip(ctx, tt.params)
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
