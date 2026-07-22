package errors

// DB-scenario sentinels. Repositories return these (or wrap them with fmt.Errorf)
// so that services and handlers never see raw driver errors.
//
// Note: ErrDBUnavailable and ErrDBTimeout both carry Code = "SERVICE_UNAVAILABLE",
// so errors.Is(ErrDBTimeout, ErrDBUnavailable) returns true. Callers that need to
// distinguish the two should compare sentinel identity directly.
var (
	// ErrRecordNotFound is returned when a query matches zero rows.
	ErrRecordNotFound = NotFound("record not found")

	// ErrDuplicateKey is returned on unique-constraint violations.
	ErrDuplicateKey = Conflict("a record with that identifier already exists")

	// ErrDBUnavailable is returned when the database pool cannot be reached.
	ErrDBUnavailable = ServiceUnavailable("database is currently unavailable")

	// ErrDBTimeout is returned when a DB operation exceeds its context deadline.
	ErrDBTimeout = ServiceUnavailable("database operation timed out")

	// ErrDBConstraint is returned for foreign-key and check-constraint violations.
	ErrDBConstraint = Unprocessable("database constraint violation")
)

// Driver-error mapping lives in internal/db/errors.go (db.MapDBError), which
// imports this package aliased as errs and normalises Mongo driver errors to
// the sentinels above before they reach the service layer.
//
// Future Postgres mapping (add next to MapDBError when pgx is adopted):
//
//	pgerrcode.UniqueViolation      → errs.ErrDuplicateKey
//	pgerrcode.ForeignKeyViolation  → errs.ErrDBConstraint
//	pgerrcode.CheckViolation       → errs.ErrDBConstraint
//	pgx.ErrNoRows                  → errs.ErrRecordNotFound
//	context.DeadlineExceeded       → errs.ErrDBTimeout
//	*pgconn.ConnectError           → errs.ErrDBUnavailable
