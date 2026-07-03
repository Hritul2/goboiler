package testing

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/stretchr/testify/assert"
)

// AssertStringContains asserts that s contains every substring in subs.
func AssertStringContains(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		assert.True(t, strings.Contains(s, sub),
			fmt.Sprintf("expected string to contain %q, got: %s", sub, s))
	}
}

// AssertErrorCode asserts that err unwraps to an *AppError with wantCode.
func AssertErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	var appErr *errs.AppError
	if !assert.True(t, stderrors.As(err, &appErr), "expected *AppError, got %T: %v", err, err) {
		return
	}
	assert.Equal(t, wantCode, appErr.Code)
}
