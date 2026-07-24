package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSecureHeaders_Map(t *testing.T) {
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-XSS-Protection":       "0",
	}
	assert.Equal(t, want, secureHeaders())
}

func TestSecureHeaders_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(SecureHeaders())
	engine.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest("GET", "/ping", nil)
	engine.ServeHTTP(w, req)

	for k, v := range secureHeaders() {
		assert.Equal(t, v, w.Header().Get(k), "header %s", k)
	}
}
