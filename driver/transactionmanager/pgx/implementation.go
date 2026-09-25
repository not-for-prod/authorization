// Package pgx provides the shared PostgreSQL transaction manager.
package pgx

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
)

type Manager = postgres.Store

var _ repository.TransactionManager = (*Manager)(nil)

func New(pool *pgxpool.Pool) (*Manager, error) { return postgres.New(pool) }
