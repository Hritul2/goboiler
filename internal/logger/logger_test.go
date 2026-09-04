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

func cfg(env, logLevel string) *config.AppConfig {
	return &config.AppConfig{
		Name:     "test-service",
		Env:      env,
		Version:  "0.0.1",
		LogLevel: logLevel,
	}
}

func TestNew_levelFollowsLogLevel_notEnv(t *testing.T) {
	// os.Chdir is process-wide — do not run in parallel.
	original, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(t.TempDir()))

	cases := []struct {
		name  string
		env   string
		level string
		want  zerolog.Level
	}{
		{"dev+debug", "dev", "debug", zerolog.DebugLevel},
		{"dev+warn", "dev", "warn", zerolog.WarnLevel},
		{"prod+debug", "prod", "debug", zerolog.DebugLevel},
		{"prod+info", "prod", "info", zerolog.InfoLevel},
		{"stage+error", "stage", "error", zerolog.ErrorLevel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, closer := logger.New(cfg(tc.env, tc.level))
			t.Cleanup(func() { _ = closer() })
			assert.Equal(t, tc.want, zerolog.GlobalLevel())
		})
	}
}

func TestNew_timeFieldFormat(t *testing.T) {
	_, closer := logger.New(cfg("dev", "debug"))
	t.Cleanup(func() { _ = closer() })
	assert.Equal(t, time.RFC3339, zerolog.TimeFieldFormat)
}

func TestNew_stage_closerNoop(t *testing.T) {
	_, closer := logger.New(cfg("stage", "info"))
	err := closer()
	require.NoError(t, err)
	// Calling twice should not panic or error.
	err = closer()
	require.NoError(t, err)
}

func TestNew_prod_closerNoop(t *testing.T) {
	_, closer := logger.New(cfg("prod", "info"))
	err := closer()
	require.NoError(t, err)
	err = closer()
	require.NoError(t, err)
}

func TestNew_dev_createsLogFile(t *testing.T) {
	// os.Chdir is process-wide — do not run in parallel.
	original, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(tmpDir))

	_, closer := logger.New(cfg("dev", "info"))
	t.Cleanup(func() { _ = closer() })

	logPath := filepath.Join(tmpDir, "log", "app.log")
	_, statErr := os.Stat(logPath)
	assert.NoError(t, statErr, "log/app.log should be created in dev mode")
	require.NoError(t, closer())
}

func TestNew_stageAndProd_noLogFile(t *testing.T) {
	for _, env := range []string{"stage", "prod"} {
		t.Run(env, func(t *testing.T) {
			// os.Chdir is process-wide — do not run in parallel.
			original, err := os.Getwd()
			require.NoError(t, err)
			tmpDir := t.TempDir()
			t.Cleanup(func() { _ = os.Chdir(original) })
			require.NoError(t, os.Chdir(tmpDir))

			_, closer := logger.New(cfg(env, "info"))
			t.Cleanup(func() { _ = closer() })

			logPath := filepath.Join(tmpDir, "log", "app.log")
			_, statErr := os.Stat(logPath)
			assert.True(t, os.IsNotExist(statErr), "log/app.log should not be created outside dev mode")
		})
	}
}

func TestNew_dev_fallsBackWhenDirUnwritable(t *testing.T) {
	// Point cwd to a read-only dir so openLogFile fails gracefully.
	original, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(original) })
	require.NoError(t, os.Chdir(tmpDir))

	// Make the directory itself read-only so MkdirAll("log") fails.
	require.NoError(t, os.Chmod(tmpDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0o755) })

	// New should not panic — it falls back to stdout-only with a warning.
	log, closer := logger.New(cfg("dev", "info"))
	t.Cleanup(func() { _ = closer() })

	assert.NotNil(t, log)
	require.NoError(t, closer())
}
