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

func TestRequestLogger_InjectsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	base := zerolog.New(&buf)

	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.Use(RequestLogger(base))
	engine.GET("/ping", func(c *gin.Context) {
		log := LoggerFromContext(c)
		log.Info().Msg("handler ran")
		c.Status(200)
	})

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set(RequestIDHeader, "fixed-id")
	engine.ServeHTTP(w, req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "fixed-id", line["request_id"])
	assert.Equal(t, "handler ran", line["message"])
}

func TestLoggerFromContext_FallsBackToNop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// No RequestLogger middleware ran — must not panic, must return a usable logger.
	require.NotPanics(t, func() {
		log := LoggerFromContext(c)
		log.Info().Msg("should be discarded silently")
	})
}
