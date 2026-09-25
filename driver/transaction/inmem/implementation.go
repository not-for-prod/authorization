package inmem

import (
	"github.com/not-for-prod/authorization/domain/repository"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
)

// Repository stores transaction records in the supplied shared in-memory transaction manager.
type Repository struct{ manager *manager.Manager }

var _ repository.Transaction = (*Repository)(nil)

func New(manager *manager.Manager) *Repository { return &Repository{manager: manager} }
