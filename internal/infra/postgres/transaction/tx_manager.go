package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager struct {
	pool            *pgxpool.Pool
	rollbackTimeout time.Duration
	queryTimeout    time.Duration
}

type txKey struct{}

func NewTxManager(pool *pgxpool.Pool, rollbackTimeout time.Duration, queryTimeout time.Duration) *TxManager {
	return &TxManager{
		pool:            pool,
		rollbackTimeout: rollbackTimeout,
		queryTimeout:    queryTimeout,
	}
}

func (txManager *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := TxFromContext(ctx); ok {
		return fn(ctx)
	}

	beginCtx, cancelBegin := context.WithTimeout(ctx, txManager.queryTimeout)
	tx, err := txManager.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()

	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), txManager.rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	txCtx := context.WithValue(ctx, txKey{}, tx)

	if err = fn(txCtx); err != nil {
		return err
	}

	commitCtx, cancelCommit := context.WithTimeout(ctx, txManager.queryTimeout)
	defer cancelCommit()

	if err = tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Executor(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := TxFromContext(ctx); ok {
		return tx
	}

	return pool
}
