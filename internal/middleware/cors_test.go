package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowedOriginSet(t *testing.T) {
	set := newAllowedOriginSet([]string{"http://localhost:3000", "https://example.com"})

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{"allowed_exact", "http://localhost:3000", true},
		{"allowed_second_entry", "https://example.com", true},
		{"disallowed", "http://evil.example", false},
		{"empty_origin", "", false},
		{"case_sensitive_mismatch", "HTTP://LOCALHOST:3000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, set.Allowed(tt.origin))
		})
	}
}

func testCORSSettings() CORSSettings {
	return CORSSettings{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "X-Custom"},
		AllowCredentials: true,
		MaxAgeSeconds:    300,
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	engine.Use(CORS(testCORSSettings()))
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	c.Request = req
	engine.ServeHTTP(w, req)

	require.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-Custom", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "300", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(CORS(testCORSSettings()))
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Origin", "http://evil.example")
	engine.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Methods"))
}

func TestCORS_NoCredentialsWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	settings := testCORSSettings()
	settings.AllowCredentials = false
	engine.Use(CORS(settings))
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	engine.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}
