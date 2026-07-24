package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit_AllowsWithinBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.Use(RateLimit(60, 3))
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/ping", nil)
		req.RemoteAddr = "1.2.3.4:1111"
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "request %d within burst should succeed", i+1)
	}
}

func TestRateLimit_RejectsOverBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.Use(RateLimit(60, 2))
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	var lastCode int
	for i := 0; i < 4; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/ping", nil)
		req.RemoteAddr = "5.6.7.8:2222"
		engine.ServeHTTP(w, req)
		lastCode = w.Code
	}
	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}

func TestRateLimit_PerIPIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.Use(RateLimit(60, 1))
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Exhaust IP A's single-token burst.
	w := httptest.NewRecorder()
	reqA := httptest.NewRequest("GET", "/ping", nil)
	reqA.RemoteAddr = "9.9.9.9:1"
	engine.ServeHTTP(w, reqA)
	require.Equal(t, http.StatusOK, w.Code)

	// IP A's second request should now be limited...
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqA)
	require.Equal(t, http.StatusTooManyRequests, w.Code)

	// ...but a different IP is unaffected.
	w = httptest.NewRecorder()
	reqB := httptest.NewRequest("GET", "/ping", nil)
	reqB.RemoteAddr = "1.1.1.1:1"
	engine.ServeHTTP(w, reqB)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEvictStaleIPLimiters(t *testing.T) {
	limiters := map[string]*rateLimiterEntry{
		"stale": {lastSeen: time.Now().Add(-20 * time.Minute)},
		"fresh": {lastSeen: time.Now()},
	}

	evictStale(limiters, 10*time.Minute)

	assert.NotContains(t, limiters, "stale")
	assert.Contains(t, limiters, "fresh")
}
