package observability_test

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/observability"
	"github.com/hritul2/goboiler/internal/utils/config"
)

func disabledCfg() *config.Config {
	return &config.Config{
		AppConfig: config.AppConfig{
			Name:    "test-service",
			Env:     "dev",
			Version: "0.0.1",
		},
		OpenTelConfig: config.OpenTelConfig{
			Enabled:    false,
			Endpoint:   "",
			SampleRate: 1.0,
		},
	}
}

func TestNew_disabled_returnsProvider(t *testing.T) {
	log := zerolog.Nop()
	provider, err := observability.New(context.Background(), disabledCfg(), log)
	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestNew_disabled_shutdownNil(t *testing.T) {
	log := zerolog.Nop()
	provider, err := observability.New(context.Background(), disabledCfg(), log)
	require.NoError(t, err)

	// Shutdown on a disabled provider (all internal sub-providers are nil)
	// must return nil and not panic.
	err = provider.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestZerologHook_disabled(t *testing.T) {
	log := zerolog.Nop()
	provider, err := observability.New(context.Background(), disabledCfg(), log)
	require.NoError(t, err)

	hook := provider.ZerologHook()
	assert.NotNil(t, hook)
}

func TestNew_disabled_multipleShutdownsSafe(t *testing.T) {
	log := zerolog.Nop()
	provider, err := observability.New(context.Background(), disabledCfg(), log)
	require.NoError(t, err)

	// Calling Shutdown twice should not panic.
	assert.NoError(t, provider.Shutdown(context.Background()))
	assert.NoError(t, provider.Shutdown(context.Background()))
}
