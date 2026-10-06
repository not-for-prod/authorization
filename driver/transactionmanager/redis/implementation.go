// Package redis provides single-root optimistic authentication storage scopes.
package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/driver/internal/redisprogress"
)

type Manager = redisprogress.Store

func New(client goredis.UniversalClient) (*Manager, error) { return redisprogress.New(client) }
