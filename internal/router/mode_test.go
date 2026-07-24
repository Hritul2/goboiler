package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGinMode(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"prod", gin.ReleaseMode},
		{"stage", gin.ReleaseMode},
		{"dev", gin.DebugMode},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			assert.Equal(t, tt.want, ginMode(tt.env))
		})
	}
}
