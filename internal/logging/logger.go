package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Config holds logging configuration
type Config struct {
	Level      zerolog.Level
	Format     string // "json" or "console"
	TimeFormat string
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Level:      zerolog.InfoLevel,
		Format:     "console",
		TimeFormat: time.RFC3339,
	}
}

// New creates a new zerolog logger with the given configuration
func New(cfg Config) zerolog.Logger {
	var output io.Writer = os.Stderr

	switch cfg.Format {
	case "console":
		output = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: cfg.TimeFormat,
		}
	case "json":
		// JSON is the default zerolog format
		output = os.Stderr
	}

	return zerolog.New(output).
		Level(cfg.Level).
		With().
		Timestamp().
		Logger()
}

// NewFromEnv creates a logger based on environment variables
// DUMBER_LOG_LEVEL: trace, debug, info, warn, error (default: info)
// DUMBER_LOG_FORMAT: json, console (default: console)
func NewFromEnv() zerolog.Logger {
	cfg := DefaultConfig()

	if level := os.Getenv("DUMBER_LOG_LEVEL"); level != "" {
		cfg.Level = ParseLevel(level)
	}

	if format := os.Getenv("DUMBER_LOG_FORMAT"); format != "" {
		switch format {
		case "json", "console", "text":
			cfg.Format = format
		}
	}

	return New(cfg)
}

// NewFromConfigValues creates a logger from level and format strings.
// This is used by main.go to create a logger from the config package's LoggingConfig
// without creating an import cycle.
func NewFromConfigValues(level, format string) zerolog.Logger {
	cfg := DefaultConfig()
	cfg.Level = ParseLevel(level)

	switch format {
	case "json":
		cfg.Format = "json"
	case "console", "text", "":
		cfg.Format = "console"
	default:
		cfg.Format = "console"
	}

	return New(cfg)
}

// ParseLevel converts a level string to zerolog.Level
func ParseLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info", "":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}
