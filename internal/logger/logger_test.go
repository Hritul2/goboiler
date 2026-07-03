package logger_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/logger"
	"github.com/hritul2/goboiler/internal/utils/config"
)

func debugCfg() *config.AppConfig {
	return &config.AppConfig{
		Name:     "test-service",
		Env:      "development",
		Version:  "0.0.1",
		LogLevel: "debug",
	}
}

func infoCfg() *config.AppConfig {
	return &config.AppConfig{
		Name:     "test-service",
		Env:      "production",
		Version:  "0.0.1",
		LogLevel: "info",
	}
}

func TestNew_debug_setsDebugLevel(t *testing.T) {
	_, closer := logger.New(debugCfg())
	t.Cleanup(func() { _ = closer() })
	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
}

func TestNew_info_setsInfoLevel(t *testing.T) {
	// Prod mode will try to create log/app.log; redirect cwd to a temp dir.
	original, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(t.TempDir()))

	_, closer := logger.New(infoCfg())
	t.Cleanup(func() { _ = closer() })
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
}

func TestNew_timeFieldFormat(t *testing.T) {
	_, closer := logger.New(debugCfg())
	t.Cleanup(func() { _ = closer() })
	assert.Equal(t, time.RFC3339, zerolog.TimeFieldFormat)
}

func TestNew_debug_closerNoop(t *testing.T) {
	_, closer := logger.New(debugCfg())
	err := closer()
	require.NoError(t, err)
	// Calling twice should not panic or error.
	err = closer()
	require.NoError(t, err)
}

func TestNew_prod_createsLogFile(t *testing.T) {
	// os.Chdir is process-wide — do not run in parallel.
	original, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(tmpDir))

	_, closer := logger.New(infoCfg())
	t.Cleanup(func() { _ = closer() })

	logPath := filepath.Join(tmpDir, "log", "app.log")
	_, statErr := os.Stat(logPath)
	assert.NoError(t, statErr, "log/app.log should be created in prod mode")
	require.NoError(t, closer())
}

func TestNew_prod_fallsBackToStderrWhenDirUnwritable(t *testing.T) {
	// Point cwd to a read-only dir so openLogFile fails gracefully.
	original, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(tmpDir))

	// Make the directory itself read-only so MkdirAll("log") fails.
	require.NoError(t, os.Chmod(tmpDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o755) })

	// New should not panic — it falls back to stderr with a warning.
	log, closer := logger.New(infoCfg())
	t.Cleanup(func() { _ = closer() })

	assert.NotNil(t, log)
	require.NoError(t, closer())
}
