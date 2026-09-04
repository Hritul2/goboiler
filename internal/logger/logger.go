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
// Output routing is env-based, not level-based:
//
//   - Env=="dev"          → colourised ConsoleWriter to stdout + log/app.log.
//   - Env=="stage"/"prod" → colourised ConsoleWriter to stdout only; the OTel
//     logs bridge (see internal/observability) ships records to the
//     collector directly, independent of this writer.
//
// LogLevel controls verbosity only (debug/info/warn/error), decoupled from
// where output goes.
//
// The returned logger is pre-seeded with service, env, and version fields.
func New(appCfg *config.AppConfig) (zerolog.Logger, func() error) {
	zerolog.TimeFieldFormat = time.RFC3339

	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}

	var (
		writer io.Writer
		closer func() error
	)

	if appCfg.Env == "dev" {
		f, err := openLogFile("log/app.log")
		if err != nil {
			fmt.Fprintf(os.Stdout, "warn: cannot open log file, falling back to stdout only: %v\n", err)
			writer = consoleWriter
			closer = func() error { return nil }
		} else {
			writer = io.MultiWriter(consoleWriter, f)
			closer = f.Close
		}
	} else {
		writer = consoleWriter
		closer = func() error { return nil }
	}

	level, err := zerolog.ParseLevel(appCfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
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
