package db

import (
	"context"
	stderrors "errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// MapDBError normalises Mongo driver errors onto the sentinels in
// internal/errors/db.go so services and handlers never see raw driver
// errors. Classification order matters: specific error classes first,
// transport classes last, unknown errors become a safe Internal.
func MapDBError(err error) error {
	var appErr *errs.AppError

	switch {
	case err == nil:
		return nil

	case stderrors.As(err, &appErr):
		// Already normalised — pass through untouched.
		return err

	case stderrors.Is(err, mongo.ErrNoDocuments):
		return errs.ErrRecordNotFound

	case mongo.IsDuplicateKeyError(err):
		// Unique-index violation (code 11000 and friends).
		return errs.ErrDuplicateKey

	case stderrors.Is(err, context.DeadlineExceeded):
		return errs.ErrDBTimeout

	case mongo.IsNetworkError(err):
		// Dial/socket failures — the deployment cannot be reached.
		return errs.ErrDBUnavailable

	case mongo.IsTimeout(err):
		// Driver-internal timeouts, including server selection.
		return errs.ErrDBTimeout

	default:
		return errs.Internal(err)
	}
}
