package redis

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"os"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errs "github.com/hritul2/goboiler/internal/errors"
)

func TestNormalizeRedisError(t *testing.T) {
	// ErrCacheTimeout and ErrCacheUnavailable share Code SERVICE_UNAVAILABLE,
	// so sentinel rows assert identity (assert.Same), not errors.Is.
	tests := []struct {
		name         string
		in           error
		wantSentinel *errs.AppError
	}{
		{"nil_passthrough", nil, nil},
		{"redis_nil", goredis.Nil, errs.ErrCacheMiss},
		{"redis_nil_wrapped", fmt.Errorf("get key: %w", goredis.Nil), errs.ErrCacheMiss},
		{"deadline_exceeded", context.DeadlineExceeded, errs.ErrCacheTimeout},
		{"deadline_exceeded_wrapped", fmt.Errorf("setex: %w", context.DeadlineExceeded), errs.ErrCacheTimeout},
		{
			"net_op_error_dial",
			&net.OpError{Op: "dial", Net: "tcp", Err: stderrors.New("connection refused")},
			errs.ErrCacheUnavailable,
		},
		{
			// os.ErrDeadlineExceeded makes OpError.Timeout() return true.
			"net_timeout",
			&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded},
			errs.ErrCacheTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeRedisError(tt.in)
			if tt.in == nil {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.Same(t, tt.wantSentinel, got)
		})
	}

	t.Run("app_error_passthrough", func(t *testing.T) {
		got := NormalizeRedisError(errs.ErrCacheMiss)
		assert.Same(t, errs.ErrCacheMiss, got)
	})

	t.Run("wrapped_app_error_passthrough", func(t *testing.T) {
		wrapped := fmt.Errorf("cache layer: %w", errs.ErrCacheUnavailable)
		got := NormalizeRedisError(wrapped)
		assert.Equal(t, wrapped, got)
	})

	t.Run("unknown_becomes_internal", func(t *testing.T) {
		cause := stderrors.New("boom")
		got := NormalizeRedisError(cause)

		var appErr *errs.AppError
		require.ErrorAs(t, got, &appErr)
		assert.Equal(t, errs.CodeInternal, appErr.Code)
		assert.Same(t, cause, appErr.Internal)
	})
}
