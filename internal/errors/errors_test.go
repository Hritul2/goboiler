package errors_test

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructors(t *testing.T) {
	sentinel := stderrors.New("root cause")

	tests := []struct {
		name        string
		err         *errs.AppError
		wantCode    string
		wantStatus  int
		wantMsg     string
		hasInternal bool
	}{
		{
			name:       "NotFound",
			err:        errs.NotFound("user not found"),
			wantCode:   errs.CodeNotFound,
			wantStatus: http.StatusNotFound,
			wantMsg:    "user not found",
		},
		{
			name:       "Unauthorized",
			err:        errs.Unauthorized("invalid token"),
			wantCode:   errs.CodeUnauthorized,
			wantStatus: http.StatusUnauthorized,
			wantMsg:    "invalid token",
		},
		{
			name:       "Forbidden",
			err:        errs.Forbidden("access denied"),
			wantCode:   errs.CodeForbidden,
			wantStatus: http.StatusForbidden,
			wantMsg:    "access denied",
		},
		{
			name:       "BadRequest",
			err:        errs.BadRequest("invalid input"),
			wantCode:   errs.CodeBadRequest,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid input",
		},
		{
			name:       "Conflict",
			err:        errs.Conflict("already exists"),
			wantCode:   errs.CodeConflict,
			wantStatus: http.StatusConflict,
			wantMsg:    "already exists",
		},
		{
			name:        "Internal",
			err:         errs.Internal(sentinel),
			wantCode:    errs.CodeInternal,
			wantStatus:  http.StatusInternalServerError,
			wantMsg:     "an internal server error occurred",
			hasInternal: true,
		},
		{
			name:       "Unprocessable",
			err:        errs.Unprocessable("validation failed"),
			wantCode:   errs.CodeUnprocessable,
			wantStatus: http.StatusUnprocessableEntity,
			wantMsg:    "validation failed",
		},
		{
			name:       "TooManyRequests",
			err:        errs.TooManyRequests("slow down"),
			wantCode:   errs.CodeTooManyRequests,
			wantStatus: http.StatusTooManyRequests,
			wantMsg:    "slow down",
		},
		{
			name:       "ServiceUnavailable",
			err:        errs.ServiceUnavailable("down for maintenance"),
			wantCode:   errs.CodeServiceUnavailable,
			wantStatus: http.StatusServiceUnavailable,
			wantMsg:    "down for maintenance",
		},
		{
			name:       "PayloadTooLarge",
			err:        errs.PayloadTooLarge("request body too large"),
			wantCode:   errs.CodePayloadTooLarge,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantMsg:    "request body too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.err.Code)
			assert.Equal(t, tt.wantStatus, tt.err.StatusCode)
			assert.Equal(t, tt.wantMsg, tt.err.Message)
			if tt.hasInternal {
				assert.Equal(t, sentinel, tt.err.Internal)
			} else {
				assert.Nil(t, tt.err.Internal)
			}
		})
	}
}

func TestAppError_Error(t *testing.T) {
	t.Run("no_internal", func(t *testing.T) {
		err := errs.NotFound("not here")
		assert.Equal(t, "[NOT_FOUND] not here", err.Error())
	})

	t.Run("with_internal", func(t *testing.T) {
		inner := stderrors.New("db down")
		err := errs.Internal(inner)
		s := err.Error()
		assert.Contains(t, s, "[INTERNAL]")
		assert.Contains(t, s, "db down")
	})
}

func TestAppError_Is(t *testing.T) {
	t.Run("same_code_is_true", func(t *testing.T) {
		a := errs.NotFound("first")
		b := errs.NotFound("second")
		assert.True(t, stderrors.Is(a, b), "same Code should match via errors.Is")
	})

	t.Run("different_code_is_false", func(t *testing.T) {
		a := errs.NotFound("x")
		b := errs.Unauthorized("x")
		assert.False(t, stderrors.Is(a, b))
	})

	t.Run("non_app_error_target_is_false", func(t *testing.T) {
		a := errs.NotFound("x")
		plain := stderrors.New("plain")
		assert.False(t, stderrors.Is(a, plain))
	})

	t.Run("wrapped_with_fmt_errorf", func(t *testing.T) {
		original := errs.NotFound("item")
		wrapped := fmt.Errorf("layer: %w", original)
		assert.True(t, stderrors.Is(wrapped, errs.NotFound("any")),
			"errors.Is should traverse wrap chain using Code comparison")
	})
}

func TestAppError_Unwrap(t *testing.T) {
	t.Run("returns_internal_error", func(t *testing.T) {
		inner := stderrors.New("root cause")
		err := errs.Internal(inner)
		assert.Equal(t, inner, err.Unwrap())
	})

	t.Run("returns_nil_when_no_internal", func(t *testing.T) {
		err := errs.NotFound("x")
		assert.Nil(t, err.Unwrap())
	})
}

func TestFromError(t *testing.T) {
	t.Run("nil_returns_nil", func(t *testing.T) {
		assert.Nil(t, errs.FromError(nil))
	})

	t.Run("app_error_returned_directly", func(t *testing.T) {
		original := errs.NotFound("x")
		result := errs.FromError(original)
		assert.Same(t, original, result)
	})

	t.Run("wrapped_app_error_unwrapped", func(t *testing.T) {
		original := errs.NotFound("item")
		wrapped := fmt.Errorf("middleware: %w", original)
		result := errs.FromError(wrapped)
		require.NotNil(t, result)
		assert.Equal(t, errs.CodeNotFound, result.Code)
	})

	t.Run("plain_error_wrapped_as_internal", func(t *testing.T) {
		plain := stderrors.New("database error")
		result := errs.FromError(plain)
		require.NotNil(t, result)
		assert.Equal(t, errs.CodeInternal, result.Code)
		assert.Equal(t, plain, result.Internal)
	})
}

func TestHTTPStatus(t *testing.T) {
	t.Run("nil_returns_200", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, errs.HTTPStatus(nil))
	})

	t.Run("app_error_returns_status_code", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, errs.HTTPStatus(errs.NotFound("x")))
		assert.Equal(t, http.StatusUnauthorized, errs.HTTPStatus(errs.Unauthorized("x")))
		assert.Equal(t, http.StatusInternalServerError, errs.HTTPStatus(errs.Internal(stderrors.New("x"))))
	})

	t.Run("plain_error_returns_500", func(t *testing.T) {
		assert.Equal(t, http.StatusInternalServerError, errs.HTTPStatus(stderrors.New("boom")))
	})
}

func TestToResponse(t *testing.T) {
	t.Run("fields_match", func(t *testing.T) {
		resp := errs.NotFound("item missing").ToResponse()
		assert.Equal(t, errs.CodeNotFound, resp.Code)
		assert.Equal(t, "item missing", resp.Message)
	})

	t.Run("internal_error_not_leaked", func(t *testing.T) {
		secret := stderrors.New("secret DB connection string")
		resp := errs.Internal(secret).ToResponse()
		assert.Equal(t, errs.CodeInternal, resp.Code)
		assert.NotContains(t, resp.Message, "secret")
		assert.NotContains(t, resp.Message, "DB connection string")
	})
}

func TestDBSentinels(t *testing.T) {
	tests := []struct {
		name       string
		sentinel   *errs.AppError
		wantCode   string
		wantStatus int
	}{
		{"ErrRecordNotFound", errs.ErrRecordNotFound, errs.CodeNotFound, http.StatusNotFound},
		{"ErrDuplicateKey", errs.ErrDuplicateKey, errs.CodeConflict, http.StatusConflict},
		{"ErrDBUnavailable", errs.ErrDBUnavailable, errs.CodeServiceUnavailable, http.StatusServiceUnavailable},
		{"ErrDBTimeout", errs.ErrDBTimeout, errs.CodeServiceUnavailable, http.StatusServiceUnavailable},
		{"ErrDBConstraint", errs.ErrDBConstraint, errs.CodeUnprocessable, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.sentinel.Code)
			assert.Equal(t, tt.wantStatus, tt.sentinel.StatusCode)
		})
	}

	t.Run("ErrDBTimeout_is_ErrDBUnavailable_same_code", func(t *testing.T) {
		// Both carry CodeServiceUnavailable, so Is returns true.
		// Callers that need to distinguish them must compare sentinel identity directly.
		assert.True(t, stderrors.Is(errs.ErrDBTimeout, errs.ErrDBUnavailable))
	})
}

func TestRedisSentinels(t *testing.T) {
	tests := []struct {
		name       string
		sentinel   *errs.AppError
		wantCode   string
		wantStatus int
	}{
		{"ErrCacheMiss", errs.ErrCacheMiss, errs.CodeNotFound, http.StatusNotFound},
		{"ErrCacheUnavailable", errs.ErrCacheUnavailable, errs.CodeServiceUnavailable, http.StatusServiceUnavailable},
		{"ErrCacheTimeout", errs.ErrCacheTimeout, errs.CodeServiceUnavailable, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.sentinel.Code)
			assert.Equal(t, tt.wantStatus, tt.sentinel.StatusCode)
		})
	}
}
