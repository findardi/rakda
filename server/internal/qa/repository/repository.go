package repository

import (
	"context"
	"fmt"

	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	*qadb.Queries
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		Queries: qadb.New(pool),
		pool:    pool,
	}
}

func (r *Repository) ExecTxTx(ctx context.Context, fn func(*qadb.Queries, pgx.Tx) error) error {
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
