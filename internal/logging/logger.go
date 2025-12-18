package logging

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// Config holds logging configuration
type Config struct {
	Level      zerolog.Level
	Format     string // "json" or "console"
	TimeFormat string
}

// FileConfig holds file logging configuration
type FileConfig struct {
	Enabled       bool
	LogDir        string
	SessionID     string
	WriteToStderr bool // if false, logs go to file only (not stderr)
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

// NewWithFile creates a logger that writes to stderr and/or a session log file.
// The session file uses JSON format for easy parsing by the CLI logs command.
// LogDir must exist before calling this function (handled by config.EnsureDirectories).
// Returns the logger and a cleanup function to close the file.
func NewWithFile(cfg Config, fileCfg FileConfig) (zerolog.Logger, func(), error) {
	var writers []io.Writer
	var cleanup func() = func() {}

	// Stderr output (only if enabled)
	if fileCfg.WriteToStderr {
		switch cfg.Format {
		case "console", "text", "":
			writers = append(writers, zerolog.ConsoleWriter{
				Out:        os.Stderr,
				TimeFormat: cfg.TimeFormat,
			})
		case "json":
			writers = append(writers, os.Stderr)
		default:
			writers = append(writers, zerolog.ConsoleWriter{
				Out:        os.Stderr,
				TimeFormat: cfg.TimeFormat,
			})
		}
	}

	// Session file output (JSON format for parsing)
	if fileCfg.Enabled && fileCfg.LogDir != "" && fileCfg.SessionID != "" {
		// Ensure log directory exists (LogDir is from config, may not have logs subdir)
		if err := os.MkdirAll(fileCfg.LogDir, 0755); err != nil {
			return zerolog.Logger{}, nil, err
		}

		filename := filepath.Join(fileCfg.LogDir, SessionFilename(fileCfg.SessionID))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return zerolog.Logger{}, nil, err
		}

		writers = append(writers, file)
		cleanup = func() { file.Close() }
	}

	// Fallback to discard if no writers configured
	if len(writers) == 0 {
		writers = append(writers, io.Discard)
	}

	multi := zerolog.MultiLevelWriter(writers...)
	ctx := zerolog.New(multi).
		Level(cfg.Level).
		With().
		Timestamp()

	// Add session ID to all log entries if provided
	if fileCfg.SessionID != "" {
		ctx = ctx.Str("session", fileCfg.SessionID)
	}

	return ctx.Logger(), cleanup, nil
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
