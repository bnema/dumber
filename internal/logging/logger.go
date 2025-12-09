package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// TODO should come from config package
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
		switch level {
		case "trace":
			cfg.Level = zerolog.TraceLevel
		case "debug":
			cfg.Level = zerolog.DebugLevel
		case "info":
			cfg.Level = zerolog.InfoLevel
		case "warn":
			cfg.Level = zerolog.WarnLevel
		case "error":
			cfg.Level = zerolog.ErrorLevel
		}
	}

	if format := os.Getenv("DUMBER_LOG_FORMAT"); format != "" {
		switch format {
		case "json", "console":
			cfg.Format = format
		}
	}

	return New(cfg)
}
