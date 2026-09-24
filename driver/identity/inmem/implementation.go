package inmem

import (
	"authorization/domain/repository"
	manager "authorization/driver/transactionmanager/inmem"
)

// Repository stores identity records in the supplied shared in-memory transaction manager.
type Repository struct{ manager *manager.Manager }

var _ repository.Identity = (*Repository)(nil)

func New(manager *manager.Manager) *Repository { return &Repository{manager: manager} }
