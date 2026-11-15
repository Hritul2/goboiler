package repo

import (
	"context"

	"github.com/hritul2/goboiler/internal/models"
)

// UserRepository is the data-access contract for User entities. It is
// driver-agnostic: IDs are opaque models.ID strings and every implementation
// returns errs sentinels (errs.ErrRecordNotFound, errs.ErrDuplicateKey, ...)
// rather than driver errors.
//
// Contract every implementation must honour:
//   - Create assigns u.ID when it is empty (the ID format is the store's
//     decision) and fills any zero timestamp.
//   - GetByID/Update/Delete return errs.ErrRecordNotFound for an ID that does
//     not exist — including one that is malformed for this store (it cannot
//     exist there).
//   - A duplicate email on Create returns errs.ErrDuplicateKey.
type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id models.ID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
	Delete(ctx context.Context, id models.ID) error
	List(ctx context.Context, p models.Pagination) ([]models.User, error)
	// EnsureIndexes idempotently prepares the store for queries (indexes,
	// constraints). Called once at wiring time by NewRepositories. A SQL
	// implementation whose schema comes from migrations can make it a no-op.
	EnsureIndexes(ctx context.Context) error
}
