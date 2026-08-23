package repository

import (
	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	*activitydb.Queries
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		Queries: activitydb.New(pool),
		pool:    pool,
	}
}
