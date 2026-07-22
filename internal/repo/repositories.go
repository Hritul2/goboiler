package repo

import "context"

// Repositories aggregates every domain repository for injection into the
// service layer (CODING_SEQUENCE steps 15–16).
type Repositories struct {
	User UserRepository
}

// NewRepositories constructs every repository and runs its EnsureIndexes —
// fail-fast at wiring time rather than on first insert. Requires db.Init to
// have run. Not unit-tested: it needs a live Mongo (covered by the compose
// integration check).
func NewRepositories(ctx context.Context) (*Repositories, error) {
	users := NewUserRepository()
	if err := users.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	return &Repositories{User: users}, nil
}
