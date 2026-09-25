package pgx_test

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization"
	eventpgx "github.com/not-for-prod/authorization/driver/event/pgx"
	identitypgx "github.com/not-for-prod/authorization/driver/identity/pgx"
	sessionredis "github.com/not-for-prod/authorization/driver/session/redis"
	transactionpgx "github.com/not-for-prod/authorization/driver/transaction/pgx"
	managerpgx "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
	userpgx "github.com/not-for-prod/authorization/driver/user/pgx"
)

func ExampleNew() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), ContextTimeoutEnabled: true})
	defer func() {
		if err := redisClient.Close(); err != nil {
			panic(err)
		}
	}()
	manager, err := managerpgx.New(pool)
	if err != nil {
		panic(err)
	}
	events, err := eventpgx.New(ctx, manager)
	if err != nil {
		panic(err)
	}
	sessions, err := sessionredis.NewSessionManager(redisClient, "tenant", 24*time.Hour)
	if err != nil {
		panic(err)
	}
	deps := authorization.Dependencies{
		TransactionManager: manager,
		Transactions:       transactionpgx.New(manager),
		Users:              userpgx.New(manager),
		Identities:         identitypgx.New(manager),
		Events:             events,
		// Add the host's Access and Eligible callbacks before calling authorization.New.
	}
	tenantSessions := authorization.WithSessionManager(sessions)
	// Pass deps and the tenant session option into the host's existing engine configuration.
	_, _ = deps, tenantSessions
}
