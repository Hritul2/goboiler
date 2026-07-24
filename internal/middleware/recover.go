package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// Recover replaces gin.Recovery(): a panic must produce the same uniform
// JSON {code,message} error shape every other error path produces via
// errs.ToResponse(), not gin's default plaintext body.
func Recover(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Bytes("stack", debug.Stack()).
					Msg("panic recovered")

				appErr := errs.Internal(fmt.Errorf("panic: %v", r))
				c.AbortWithStatusJSON(errs.HTTPStatus(appErr), appErr.ToResponse())
			}
		}()
		c.Next()
	}
}
