package errors

// Redis-scenario sentinels. Cache access helpers return these so callers
// never see raw go-redis errors.
var (
	// ErrCacheMiss is returned when a key is absent from the cache (redis.Nil).
	ErrCacheMiss = NotFound("cache miss")

	// ErrCacheUnavailable is returned when the Redis server cannot be reached.
	ErrCacheUnavailable = ServiceUnavailable("cache is currently unavailable")

	// ErrCacheTimeout is returned when a Redis operation exceeds its deadline.
	ErrCacheTimeout = ServiceUnavailable("cache operation timed out")
)

// Driver-error normalisation lives in internal/redis/errors.go
// (redis.NormalizeRedisError), which imports this package aliased as errs and
// maps go-redis errors to the sentinels above before they reach callers:
//
//	redis.Nil                → errs.ErrCacheMiss
//	context.DeadlineExceeded → errs.ErrCacheTimeout
//	net.Error (timeout)      → errs.ErrCacheTimeout
//	net.Error (other)        → errs.ErrCacheUnavailable
