package container

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Pkhmv16/tripGO/internal/config"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	transport "github.com/Pkhmv16/tripGO/internal/http"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	"github.com/Pkhmv16/tripGO/internal/http/probe/health"
	"github.com/Pkhmv16/tripGO/internal/http/probe/ready"
	"github.com/Pkhmv16/tripGO/internal/http/trip/create"
	"github.com/Pkhmv16/tripGO/internal/http/trip/finish"
	"github.com/Pkhmv16/tripGO/internal/http/trip/get"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	"github.com/Pkhmv16/tripGO/internal/infra/uuid"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	tripidempotency "github.com/Pkhmv16/tripGO/internal/repository/postgres/tripidempotency"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripstatushistory"
	createusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/create"
	finishusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/finish"
	getusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/get"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Container struct {
	server *http.Server
	pool   *pgxpool.Pool
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	pool, err := postgres.NewPostgresPool(ctx,
		postgres.Config{
			URL:             cfg.DatabaseURL,
			MaxConns:        cfg.DatabaseMaxConns,
			MinConns:        cfg.DatabaseMinConns,
			MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
			ConnectTimeout:  cfg.DatabaseConnectTimeout,
		})
	if err != nil {
		return nil, err
	}

	tripRepo := triprepository.NewPgTripRepository(pool, cfg.DatabaseQueryTimeout)

	historyRepo := tripstatushistory.NewPgTripStatusChangeRepository(pool, cfg.DatabaseQueryTimeout)

	txManager := transaction.NewTxManager(pool,
		cfg.DatabaseRollbackTimeout, cfg.DatabaseQueryTimeout)

	idempotencyRepo := tripidempotency.NewPgIdempotencyRepository(pool, cfg.DatabaseQueryTimeout, cfg.IdempotencyTTL)

	createUsecase := createusecase.New(tripRepo, historyRepo, idempotencyRepo, txManager, uuid.Generator{})
	getUsecase := getusecase.New(tripRepo)
	finishUsecase := finishusecase.New(tripRepo, historyRepo, txManager)

	createHandler := create.New(createUsecase, cfg.HTTPMaxBodyBytes)
	getHandler := get.New(getUsecase)
	finishHandler := finish.New(finishUsecase)
	healthHandler := health.New()
	readyHandler := ready.New(pool, cfg.DatabaseQueryTimeout)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: api.HandlerWithOptions(&transport.Handler{
			CreateHandler: createHandler,
			GetHandler:    getHandler,
			FinishHandler: finishHandler,
			HealthHandler: healthHandler,
			ReadyHandler:  readyHandler,
		}, api.ChiServerOptions{
			ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
			},
		}),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	return &Container{server: server, pool: pool}, nil
}

func (c *Container) Run() error {
	err := c.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

func (c *Container) Shutdown(ctx context.Context) error {
	var shutdownErr error
	if err := c.server.Shutdown(ctx); err != nil {
		shutdownErr = fmt.Errorf("shutdown HTTP: %w", err)
		if closeErr := c.server.Close(); closeErr != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close HTTP: %w", closeErr))
		}
	}
	poolClosed := make(chan struct{})
	go func() {
		c.pool.Close()
		close(poolClosed)
	}()
	select {
	case <-poolClosed:
		return shutdownErr
	case <-ctx.Done():
		return errors.Join(shutdownErr, fmt.Errorf("close database pool: %w", ctx.Err()))
	}
}
