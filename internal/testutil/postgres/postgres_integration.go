//go:build integration

package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	postgresinfra "github.com/Pkhmv16/tripGO/internal/infra/postgres"
)

func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	postgres, err := postgrescontainer.Run(ctx, "postgres:17-alpine",
		postgrescontainer.WithDatabase("tripgo_test"),
		postgrescontainer.WithUsername("test"),
		postgrescontainer.WithPassword("test"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if postgres != nil {
		testcontainers.CleanupContainer(t, postgres)
	}
	if err != nil {
		t.Fatalf("start test PostgreSQL: %v", err)
	}

	dsn, err := postgres.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get test database URL: %v", err)
	}
	pool, err := postgresinfra.NewPostgresPool(ctx, postgresinfra.Config{
		URL: dsn, MaxConns: 4, MinConns: 0,
		MaxConnLifetime: time.Hour, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	migrationDB := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := migrationDB.Close(); err != nil {
			t.Errorf("close migration connection: %v", err)
		}
	}()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("find project directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("find go.mod: %v", err)
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("project go.mod not found")
		}
		root = parent
	}
	migrations := os.DirFS(filepath.Join(root, "migrations"))
	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDB, migrations)
	if err != nil {
		t.Fatalf("initialize migrations: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}
