// Package redisprogress implements optimistic, single-root storage scopes.
package redisprogress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"

	"github.com/not-for-prod/proterror/proterror"
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	"github.com/not-for-prod/authorization/internal/pkg/convert"
)

type contextKey struct{}
type scope struct {
	mu       sync.Mutex
	owner    *Store
	active   bool
	key      string
	previous string
	original *entity.Transaction
	next     *entity.Transaction
}
type Store struct{ client goredis.UniversalClient }

func New(client goredis.UniversalClient) (*Store, error) {
	if client == nil || (reflect.ValueOf(client).Kind() == reflect.Pointer && reflect.ValueOf(client).IsNil()) {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("redis client is required"))
	}
	return &Store{client: client}, nil
}
func key(tenant entity.TenantID, id entity.TransactionID) string {
	hash := sha256.Sum256([]byte(string(tenant) + "\x00" + string(id)))
	return "authorization:transaction:v1:{" + hex.EncodeToString(hash[:]) + "}"
}
func invalid(message string) error {
	return errors.Join(&proterror.FailedPrecondition{}, errors.New(message))
}
func fromRedis(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, goredis.Nil) {
		return errors.Join(&proterror.NotFound{}, repository.ErrNotFound)
	}
	return errors.Join(&proterror.Unavailable{}, err)
}
func (s *Store) access(ctx context.Context) (*scope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, ok := ctx.Value(contextKey{}).(*scope)
	if !ok {
		return nil, invalid("operation requires Redis storage scope")
	}
	tx.mu.Lock()
	if tx.owner != s || !tx.active {
		tx.mu.Unlock()
		return nil, invalid("foreign or closed Redis storage scope")
	}
	return tx, nil
}

func encode(t entity.Transaction) (string, error) {
	row, err := convert.TransactionToXO(t)
	return string(row.Snapshot), err
}
