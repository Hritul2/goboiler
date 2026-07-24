package middleware

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogging_LogsInfoOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	base := zerolog.New(&buf)

	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.Use(RequestLogger(base))
	engine.Use(RequestLogging())
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	engine.ServeHTTP(w, req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "info", line["level"])
	assert.EqualValues(t, 200, line["status"])
	assert.Equal(t, "GET", line["method"])
	assert.Equal(t, "/ping", line["path"])
	assert.Contains(t, line, "duration_ms")
}

func TestRequestLogging_LogsErrorOn5xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	base := zerolog.New(&buf)

	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.Use(RequestLogger(base))
	engine.Use(RequestLogging())
	engine.GET("/fail", func(c *gin.Context) { c.Status(500) })

	req := httptest.NewRequest("GET", "/fail", nil)
	engine.ServeHTTP(w, req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "error", line["level"])
	assert.EqualValues(t, 500, line["status"])
}
