package http

import (
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/probe/health"
	"github.com/Pkhmv16/tripGO/internal/http/probe/ready"
	"github.com/Pkhmv16/tripGO/internal/http/trip/create"
	"github.com/Pkhmv16/tripGO/internal/http/trip/finish"
	"github.com/Pkhmv16/tripGO/internal/http/trip/get"
	"github.com/Pkhmv16/tripGO/internal/http/trip/listpositions"
	"github.com/Pkhmv16/tripGO/internal/http/trip/saveposition"
)

type (
	CreateHandler        = create.Handler
	GetHandler           = get.Handler
	FinishHandler        = finish.Handler
	HealthHandler        = health.Handler
	ReadyHandler         = ready.Handler
	SavePositionHandler  = saveposition.Handler
	ListPositionsHandler = listpositions.Handler
)

type Handler struct {
	*CreateHandler
	*GetHandler
	*FinishHandler
	*HealthHandler
	*ReadyHandler
	*SavePositionHandler
	*ListPositionsHandler
}

var _ api.ServerInterface = (*Handler)(nil)
