package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllAndReport is a test handler that reads the full body and reports
// errors via c.Error, matching how a real handler's bind call would behave.
func readAllAndReport(c *gin.Context) {
	_, err := io.ReadAll(c.Request.Body)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusOK)
}

func TestMaxReqResSize_UnderLimitPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(MaxReqResSize(100, 0))
	engine.Use(ErrorHandler())
	engine.POST("/echo", readAllAndReport)

	req := httptest.NewRequest("POST", "/echo", strings.NewReader("small body"))
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMaxReqResSize_OverLimitRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(MaxReqResSize(5, 0))
	engine.Use(ErrorHandler())
	engine.POST("/echo", readAllAndReport)

	req := httptest.NewRequest("POST", "/echo", strings.NewReader("this body is way over five bytes"))
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "PAYLOAD_TOO_LARGE", body["code"])
}

func TestMaxReqResSizeOverride_RaisesLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	longBody := strings.Repeat("x", 50)

	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(MaxReqResSize(5, 0)) // global default: 5 bytes, would reject this body
	engine.POST("/upload", MaxReqResSizeOverride(1000), ErrorHandler(), readAllAndReport)

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(longBody))
	engine.ServeHTTP(w, req)

	// Proves the override actually re-wraps the ORIGINAL body rather than
	// nesting on top of the global's smaller wrap — naive nesting would
	// still reject this at 5 bytes regardless of the override's larger value.
	require.Equal(t, http.StatusOK, w.Code, "override must allow a body the global default alone would reject")
}

func TestMaxReqResSizeOverride_CanLowerLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(MaxReqResSize(1000, 0)) // global default: generous
	engine.POST("/strict", MaxReqResSizeOverride(5), ErrorHandler(), readAllAndReport)

	req := httptest.NewRequest("POST", "/strict", strings.NewReader("this is definitely over five bytes"))
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestMaxReqResSize_WarnsOnLargeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	base := zerolog.New(&buf)

	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(RequestID())
	engine.Use(RequestLogger(base))
	engine.Use(MaxReqResSize(1000, 10)) // response limit: 10 bytes
	engine.GET("/big", func(c *gin.Context) {
		c.String(http.StatusOK, strings.Repeat("y", 100))
	})

	req := httptest.NewRequest("GET", "/big", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 100, w.Body.Len(), "response must be delivered in full, not truncated")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "warn", line["level"])
}
