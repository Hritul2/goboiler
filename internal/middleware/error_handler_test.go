package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorHandler_MapsMaxBytesErrorTo413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(ErrorHandler())
	engine.POST("/echo", func(c *gin.Context) {
		buf := make([]byte, 4)
		_, err := c.Request.Body.Read(buf)
		if err != nil {
			_ = c.Error(err)
			return
		}
	})

	req := httptest.NewRequest("POST", "/echo", &oversizedBody{})
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "PAYLOAD_TOO_LARGE", body["code"])
}

func TestErrorHandler_GenericErrorMapsViaFromError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Use(ErrorHandler())
	engine.GET("/fail", func(c *gin.Context) {
		_ = c.Error(assertAnError{})
	})

	req := httptest.NewRequest("GET", "/fail", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "boom" }

// oversizedBody is an io.Reader standing in for a request body that
// http.MaxBytesReader has already flagged as over its limit — this test is
// only exercising ErrorHandler's error-to-status mapping, not
// http.MaxBytesReader's own counting logic (that's covered in
// max_size_test.go against the real thing).
type oversizedBody struct{}

func (o *oversizedBody) Read(_ []byte) (int, error) {
	return 0, &http.MaxBytesError{Limit: 2}
}
