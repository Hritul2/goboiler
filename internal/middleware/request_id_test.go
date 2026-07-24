package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	engine.ServeHTTP(w, req)

	got := w.Header().Get(RequestIDHeader)
	require.NotEmpty(t, got)
	_, err := uuid.Parse(got)
	assert.NoError(t, err, "generated request id must be a valid UUID")
}

func TestRequestID_PropagatesInbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set(RequestIDHeader, "inbound-id-123")
	engine.ServeHTTP(w, req)

	assert.Equal(t, "inbound-id-123", w.Header().Get(RequestIDHeader))
}

func TestRequestIDFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	var got string
	engine.Use(RequestID())
	engine.GET("/ping", func(c *gin.Context) {
		got = RequestIDFromContext(c)
		c.Status(200)
	})

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set(RequestIDHeader, "ctx-id-456")
	c.Request = req
	engine.ServeHTTP(w, req)

	assert.Equal(t, "ctx-id-456", got)
}
