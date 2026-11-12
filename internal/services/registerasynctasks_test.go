package services

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// The skeleton registers no jobs; the job router must still build (it is
// passed to asyncprocessor.Init at boot) without needing SMTP or any config.
func TestNewJobRouter_EmptySkeleton(t *testing.T) {
	router, err := NewJobRouter(&config.Config{}, zerolog.Nop())

	require.NoError(t, err)
	require.NotNil(t, router)
}
