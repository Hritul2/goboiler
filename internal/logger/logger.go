package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// New builds a zerolog.Logger for the lifetime of the application and returns
// a closer that must be called on shutdown to flush and release the log file.
//
// LogLevel=="debug" → colourised ConsoleWriter to stderr.
// All other levels  → JSON to stderr + log/app.log.
//
// The returned logger is pre-seeded with service, env, and version fields.
func New(appCfg *config.AppConfig) (zerolog.Logger, func() error) {
	zerolog.TimeFieldFormat = time.RFC3339

	isDebug := appCfg.LogLevel == "debug"

	var (
		level  zerolog.Level
		writer io.Writer
		closer func() error
	)

	if isDebug {
		level = zerolog.DebugLevel
		writer = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
		closer = func() error { return nil }
	} else {
		level = zerolog.InfoLevel
		f, err := openLogFile("log/app.log")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: cannot open log file, falling back to stderr: %v\n", err)
			writer = os.Stderr
			closer = func() error { return nil }
		} else {
			writer = io.MultiWriter(os.Stderr, f)
			closer = f.Close
		}
	}

	zerolog.SetGlobalLevel(level)

	log := zerolog.New(writer).
		With().
		Timestamp().
		Str("service", appCfg.Name).
		Str("env", appCfg.Env).
		Str("version", appCfg.Version).
		Logger()

	return log, closer
}

// openLogFile creates the log directory if needed and opens the file for appending.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll("log", 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}
