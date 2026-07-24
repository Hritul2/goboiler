package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(_ context.Context) error {
	return f.err
}

func TestHealthHandler_Ping_AllHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ping", nil)

	h := NewHealthHandler(fakePinger{}, fakePinger{}, zerolog.Nop())
	h.Ping(c)

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "ok", checks["db"])
	assert.Equal(t, "ok", checks["redis"])
}

func TestHealthHandler_Ping_DBDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ping", nil)

	h := NewHealthHandler(fakePinger{err: errors.New("connection refused")}, fakePinger{}, zerolog.Nop())
	h.Ping(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "degraded", body["status"])
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "down", checks["db"])
	assert.Equal(t, "ok", checks["redis"])
}

func TestHealthHandler_Ping_RedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ping", nil)

	h := NewHealthHandler(fakePinger{}, fakePinger{err: errors.New("timeout")}, zerolog.Nop())
	h.Ping(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "ok", checks["db"])
	assert.Equal(t, "down", checks["redis"])
}

func TestHealthHandler_Ping_BothDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/ping", nil)

	h := NewHealthHandler(
		fakePinger{err: errors.New("db down")},
		fakePinger{err: errors.New("redis down")},
		zerolog.Nop(),
	)
	h.Ping(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "degraded", body["status"])
}
