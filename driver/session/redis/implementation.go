// Package redis provides recoverable opaque sessions for one tenant.
package redis

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"time"

	"github.com/not-for-prod/proterror/proterror"
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

type SessionManager struct {
	client   goredis.UniversalClient
	tenant   entity.TenantID
	prefix   string
	lifetime time.Duration
}

var _ gateway.SessionManager = (*SessionManager)(nil)

// NewSessionManager uses host-owned Redis connections. Use a persistent, noeviction deployment.
func NewSessionManager(
	client goredis.UniversalClient,
	tenant entity.TenantID,
	lifetime time.Duration,
) (*SessionManager, error) {
	if client == nil || (reflect.ValueOf(client).Kind() == reflect.Pointer && reflect.ValueOf(client).IsNil()) ||
		tenant == "" ||
		lifetime < time.Millisecond {
		return nil, errors.Join(
			&proterror.InvalidArgument{},
			errors.New("redis client, tenant and positive millisecond session lifetime are required"),
		)
	}
	return &SessionManager{
		client:   client,
		tenant:   tenant,
		prefix:   "authorization:session:v1:{" + digest(string(tenant)) + "}:",
		lifetime: lifetime,
	}, nil
}
func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
