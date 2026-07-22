package redis

import (
	"context"
	stderrors "errors"
	"net"

	goredis "github.com/redis/go-redis/v9"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// NormalizeRedisError normalises go-redis errors onto the cache sentinels in
// internal/errors/redis.go so services never see raw driver errors.
// Classification order matters: specific error classes first, transport
// classes last, unknown errors become a safe Internal.
func NormalizeRedisError(err error) error {
	var appErr *errs.AppError
	var netErr net.Error

	switch {
	case err == nil:
		return nil

	case stderrors.As(err, &appErr):
		// Already normalised — pass through untouched.
		return err

	case stderrors.Is(err, goredis.Nil):
		return errs.ErrCacheMiss

	case stderrors.Is(err, context.DeadlineExceeded):
		return errs.ErrCacheTimeout

	case stderrors.As(err, &netErr):
		if netErr.Timeout() {
			return errs.ErrCacheTimeout
		}
		return errs.ErrCacheUnavailable

	default:
		return errs.Internal(err)
	}
}
