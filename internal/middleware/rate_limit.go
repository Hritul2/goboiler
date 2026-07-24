package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// rateLimiterEntry pairs a per-IP token bucket with the last time it was
// used, so evictStale can bound the map's memory for many unique IPs.
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const evictionInterval = 10 * time.Minute

// evictStale removes entries idle longer than maxIdle. Exposed standalone
// (not just as an internal ticker goroutine) so it's unit-testable without
// sleeping in a test.
func evictStale(limiters map[string]*rateLimiterEntry, maxIdle time.Duration) {
	cutoff := time.Now().Add(-maxIdle)
	for ip, entry := range limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(limiters, ip)
		}
	}
}

// RateLimit is a per-IP token-bucket limiter. perMinute/burst come from
// config. A background goroutine evicts entries idle longer than
// evictionInterval to bound memory for many unique IPs — otherwise this map
// grows forever for a public-facing API.
func RateLimit(perMinute, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	limiters := make(map[string]*rateLimiterEntry)

	limit := rate.Every(time.Minute / time.Duration(perMinute))

	go func() {
		for range time.Tick(evictionInterval) {
			mu.Lock()
			evictStale(limiters, evictionInterval)
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		entry, ok := limiters[ip]
		if !ok {
			entry = &rateLimiterEntry{limiter: rate.NewLimiter(limit, burst)}
			limiters[ip] = entry
		}
		entry.lastSeen = time.Now()
		allowed := entry.limiter.Allow()
		mu.Unlock()

		if !allowed {
			appErr := errs.TooManyRequests("rate limit exceeded")
			c.AbortWithStatusJSON(errs.HTTPStatus(appErr), appErr.ToResponse())
			return
		}

		c.Next()
	}
}
