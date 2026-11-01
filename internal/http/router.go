package http

import (
	api "github.com/Pkhmv16/tripGO/internal/generated"
	"github.com/Pkhmv16/tripGO/internal/http/probe/health"
	"github.com/Pkhmv16/tripGO/internal/http/probe/ready"
	"github.com/Pkhmv16/tripGO/internal/http/trip/create"
	"github.com/Pkhmv16/tripGO/internal/http/trip/finish"
	"github.com/Pkhmv16/tripGO/internal/http/trip/get"
)

type CreateHandler = create.Handler
type GetHandler = get.Handler
type FinishHandler = finish.Handler
type HealthHandler = health.Handler
type ReadyHandler = ready.Handler

type Handler struct {
	*CreateHandler
	*GetHandler
	*FinishHandler
	*HealthHandler
	*ReadyHandler
}

var _ api.ServerInterface = (*Handler)(nil)
