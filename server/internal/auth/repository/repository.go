package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdb "github.com/findardi/rakda/server/internal/auth/repository/sqlc"
)

type Repository struct {
	*authdb.Queries
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		Queries: authdb.New(pool),
		pool:    pool,
	}
}

// ExecTx running fn in one transaction
func (r *Repository) ExecTx(ctx context.Context, fn func(*authdb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(r.Queries.WithTx(tx)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) ExecTxTx(ctx context.Context, fn func(*authdb.Queries, pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer tx.Rollback(ctx)

	if err := fn(r.Queries.WithTx(tx), tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
