package db

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	errs "github.com/hritul2/goboiler/internal/errors"
)

func TestMapDBError(t *testing.T) {
	// ErrDBTimeout and ErrDBUnavailable share Code SERVICE_UNAVAILABLE, so
	// sentinel rows assert identity (assert.Same), not errors.Is.
	tests := []struct {
		name         string
		in           error
		wantSentinel *errs.AppError // exact sentinel identity, nil if not applicable
	}{
		{"nil_passthrough", nil, nil},
		{"no_documents", mongo.ErrNoDocuments, errs.ErrRecordNotFound},
		{"no_documents_wrapped", fmt.Errorf("find user: %w", mongo.ErrNoDocuments), errs.ErrRecordNotFound},
		{
			"duplicate_key_write_exception",
			mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000}}},
			errs.ErrDuplicateKey,
		},
		{"duplicate_key_command_error", mongo.CommandError{Code: 11000}, errs.ErrDuplicateKey},
		{"deadline_exceeded", context.DeadlineExceeded, errs.ErrDBTimeout},
		{"deadline_exceeded_wrapped", fmt.Errorf("insert: %w", context.DeadlineExceeded), errs.ErrDBTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapDBError(tt.in)
			if tt.in == nil {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.Same(t, tt.wantSentinel, got)
		})
	}

	t.Run("app_error_passthrough", func(t *testing.T) {
		got := MapDBError(errs.ErrRecordNotFound)
		assert.Same(t, errs.ErrRecordNotFound, got)
	})

	t.Run("wrapped_app_error_passthrough", func(t *testing.T) {
		wrapped := fmt.Errorf("repo layer: %w", errs.ErrDuplicateKey)
		got := MapDBError(wrapped)
		// Already-normalised errors pass through unchanged, wrapper intact.
		assert.Equal(t, wrapped, got)
	})

	t.Run("unknown_becomes_internal", func(t *testing.T) {
		cause := stderrors.New("boom")
		got := MapDBError(cause)

		var appErr *errs.AppError
		require.ErrorAs(t, got, &appErr)
		assert.Equal(t, errs.CodeInternal, appErr.Code)
		assert.Same(t, cause, appErr.Internal)
	})
}
