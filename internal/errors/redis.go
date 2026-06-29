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

// IsCacheNil reports whether err is a go-redis Nil reply (cache miss sentinel).
//
// Stub: always returns false until go-redis is wired in step 8.
//
// Step 8 integration point (internal/redis/redis.go):
//
//	import stderrors "errors"
//	import "github.com/redis/go-redis/v9"
//
//	return stderrors.Is(err, redis.Nil)
func IsCacheNil(_ error) bool {
	return false
}

// --- go-redis error normalisation (step 8, internal/redis/) ---
//
// internal/redis/ imports this package (aliased as errs) and exposes a helper
// (e.g. NormalizeRedisError) that maps go-redis errors before they reach callers:
//
//	redis.Nil                → errs.ErrCacheMiss
//	*net.OpError / dial err  → errs.ErrCacheUnavailable
//	context.DeadlineExceeded → errs.ErrCacheTimeout
