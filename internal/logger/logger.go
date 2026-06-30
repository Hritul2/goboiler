package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/utils/config"
)

// New builds a zerolog.Logger for the lifetime of the application.
// Dev/development env: colourised console output at DEBUG level.
// All other envs: JSON to stderr + log/app.log at INFO level.
// The returned logger is pre-seeded with service, env, and version fields.
func New(appCfg *config.AppConfig) zerolog.Logger {
	isDebug := appCfg.LogLevel == "debug"

	var (
		level  zerolog.Level
		writer io.Writer
	)

	if isDebug {
		level = zerolog.DebugLevel
		writer = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339} // RFC3339 is a standard format for representing date and time in a human-readable way. It is often used in logging to provide a consistent timestamp format. for example, 2023-06-15T14:30:00Z represents June 15, 2023, at 14:30 UTC.
	} else {
		level = zerolog.InfoLevel
		f, err := openLogFile("log/app.log")
		if err != nil {
			writer = os.Stderr
		} else {
			writer = io.MultiWriter(os.Stderr, f)
		}
	}

	zerolog.SetGlobalLevel(level)

	return zerolog.New(writer).
		With().
		Timestamp().
		Str("service", appCfg.Name).
		Str("env", appCfg.Env).
		Str("version", appCfg.Version).
		Logger()
}

// openLogFile creates the log directory if needed and opens the file for appending.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll("log", 0o755); err != nil { // 0o755 is the octal representation of the permission bits for the directory 755 is the permission for the directory, which means that the owner can read, write, and execute, while others can only read and execute.
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) // 0o644 is the octal representation of the permission bits for the file 644 is the permission for the file, which means that the owner can read and write, while others can only read.
}
