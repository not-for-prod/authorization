// Package pgx provides a PostgreSQL transaction repository.
package pgx

import (
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/driver/internal/postgres"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
)

type Repository struct{ manager *postgres.Store }

var _ repository.Transaction = (*Repository)(nil)

func New(m *manager.Manager) *Repository { return &Repository{manager: m} }
