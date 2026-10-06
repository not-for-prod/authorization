// Package redis stores expiring authentication progress with atomic version checks.
package redis

import (
	"github.com/not-for-prod/authorization/domain/repository"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/redis"
)

// New uses exactly the manager passed to the engine's TransactionManager port.
func New(m *manager.Manager) repository.Transaction { return m }
