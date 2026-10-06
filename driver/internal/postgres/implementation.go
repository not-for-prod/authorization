// Package postgres owns the shared pgx transaction scope for authentication drivers.
package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	proterrors "github.com/not-for-prod/proterror"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
)

type contextKey struct{}
type TransactionKey struct {
	Tenant entity.TenantID
	ID     entity.TransactionID
}

// Access is held exclusively until its release function is called. Do not retain it.
type Access struct {
	DB           trmpgx.Tr
	Transactions map[TransactionKey]entity.Transaction
	Completions  map[TransactionKey]bool
	Users        map[entity.UserID]entity.User
	Identities   map[entity.IdentityID]bool
}
type scope struct {
	mu     sync.Mutex
	owner  *Store
	active bool
	access Access
}

// Store owns one pool's transaction context. The host owns the pool lifetime.
type Store struct {
	pool    *pgxpool.Pool
	manager *manager.Manager
}

func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("PostgreSQL pool is required"))
	}
	// Avito's default pgx factory rolls back from a cancellation goroutine while
	// pgx may still be executing a query. Keep transaction finalization synchronous.
	m, err := manager.New(func(ctx context.Context, settings trm.Settings) (context.Context, trm.Transaction, error) {
		options := pgx.TxOptions{}
		if pgSettings, ok := settings.(trmpgx.Settings); ok {
			options = pgSettings.TxOpts()
		}
		tx, err := pool.BeginTx(ctx, options)
		if err != nil {
			return ctx, nil, err
		}
		return ctx, &scopedTransaction{tx: tx, active: true, closed: make(chan struct{})}, nil
	})
	if err != nil {
		return nil, errors.Join(&proterror.Internal{}, err)
	}
	return &Store{pool: pool, manager: m}, nil
}
func Invalid(message string) error {
	return errors.Join(&proterror.FailedPrecondition{}, errors.New(message))
}
func FromPG(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.Join(proterrors.FromPG(err), repository.ErrNotFound)
	}
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errors.Join(proterrors.FromPG(err), repository.ErrAlreadyExists)
	}
	return proterrors.FromPG(err)
}

// scopedTransaction implements Avito's context contract without asynchronous pgx access.
// The transaction manager alone finalizes it, after the callback has returned.
type scopedTransaction struct {
	tx     pgx.Tx
	active bool
	closed chan struct{}
}

func (t *scopedTransaction) Transaction() interface{} { return t.tx }
func (t *scopedTransaction) IsActive() bool           { return t.active }
func (t *scopedTransaction) Closed() <-chan struct{}  { return t.closed }
func (t *scopedTransaction) Commit(ctx context.Context) error {
	if !t.active {
		return pgx.ErrTxClosed
	}
	t.active = false
	defer close(t.closed)
	return t.tx.Commit(ctx)
}
func (t *scopedTransaction) Rollback(ctx context.Context) error {
	if !t.active {
		return pgx.ErrTxClosed
	}
	t.active = false
	defer close(t.closed)
	// A canceled request still needs bounded connection cleanup.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return t.tx.Rollback(cleanup)
}
