package get_test

import (
	"context"
	"reflect"
	"testing"

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
		Get(ctx, id).
		Return(want, nil).
		Times(1)

	uc := get.New(repo)

	got, err := uc.GetTrip(ctx, string(id))
	if err != nil {
		t.Fatalf("GetTrip() returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetTrip() = %+v, want %+v", got, want)
	}
}
