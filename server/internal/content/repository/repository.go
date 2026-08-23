package repository

import (
	"context"
	"fmt"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	*contentdb.Queries
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		Queries: contentdb.New(pool),
		pool:    pool,
	}
}

func (r *Repository) ExecTx(ctx context.Context, fn func(*contentdb.Queries) error) error {
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

func (r *Repository) ExecTxTx(ctx context.Context, fn func(*contentdb.Queries, pgx.Tx) error) error {
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
