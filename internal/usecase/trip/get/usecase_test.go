package get_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/get"
)

func TestUsecaseGetTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)

	ctx := context.Background()
	id := domaintrip.TripId("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")

	want := &domaintrip.Trip{
		Id:     id,
		Status: domaintrip.StatusActive,
		Price:  1450,
	}

	repo.EXPECT().
		Get(gomock.Any(), id).
		Return(want, nil).
		Times(1)

	uc := get.New(repo, noop.NewTracerProvider().Tracer("test"))

	got, err := uc.GetTrip(ctx, string(id))
	if err != nil {
		t.Fatalf("GetTrip() returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetTrip() = %+v, want %+v", got, want)
	}
}

func TestUsecaseGetTripError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "trip not found", err: domaintrip.ErrTripNotFound},
		{name: "repository error", err: errors.New("database unavailable")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			ctx := context.Background()
			id := domaintrip.TripId("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")

			repo.EXPECT().Get(gomock.Any(), id).Return(nil, tt.err).Times(1)
			uc := get.New(repo, noop.NewTracerProvider().Tracer("test"))

			got, err := uc.GetTrip(ctx, string(id))
			if !errors.Is(err, tt.err) {
				t.Errorf("GetTrip() error = %v, want %v", err, tt.err)
			}
			if got != nil {
				t.Errorf("GetTrip() returned a trip on error: %+v", got)
			}
		})
	}
}
