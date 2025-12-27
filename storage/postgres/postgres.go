package postgres

import (
	"context"
	"fmt"
	"wegugin/config"
	db "wegugin/storage/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

type sqlStore struct {
	DB *pgxpool.Pool
	*db.Queries
}

func ConnectionDb(ctx context.Context) (*sqlStore, error) {
	conf := config.Load()
	conDb := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		conf.Postgres.PDB_USER, conf.Postgres.PDB_PASSWORD, conf.Postgres.PDB_HOST, conf.Postgres.PDB_PORT, conf.Postgres.PDB_NAME)
	dbConn, err := pgxpool.New(ctx, conDb)
	if err != nil {
		return nil, err
	}

	if err := dbConn.Ping(ctx); err != nil {
		return nil, err
	}

	return &sqlStore{
		DB:      dbConn,
		Queries: db.New(dbConn),
	}, nil
}

type Store interface {
	db.Querier
}

func New(ctx context.Context) (Store, error) {
	return ConnectionDb(ctx)
}
