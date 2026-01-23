package logging

import (
	"context"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/rs/zerolog"
)

// glibLogger holds the logger for the GLib handler.
// We need this because the GLib callback doesn't support passing Go pointers directly.
var (
	glibLogger     zerolog.Logger
	glibLoggerOnce sync.Once
)

// InstallGLibLogHandler installs a custom GLib log handler that routes
// GTK4/WebKitGTK6/GLib messages to the provided zerolog logger.
// This must be called before GTK/WebKit initialization.
// The handler captures messages from all GLib-based libraries (GTK, GDK, WebKit, etc.)
// If enableDebug is true, GLib debug messages will also be captured (useful when log level is debug/trace).
func InstallGLibLogHandler(ctx context.Context, logger zerolog.Logger, enableDebug bool) {
	log := FromContext(ctx)

	glibLoggerOnce.Do(func() {
		glibLogger = logger

		// Only enable GLib debug messages if requested (matches app log level)
		if enableDebug {
			glib.LogSetDebugEnabled(true)
		}

		// Install our custom handler for all log messages
		handler := glib.LogFunc(glibLogHandler)
		glib.LogSetDefaultHandler(&handler, 0)

		log.Info().Bool("debug_enabled", enableDebug).Msg("GLib log handler installed")
	})
}

// glibLogHandler is the callback invoked by GLib for all log messages.
func glibLogHandler(domain string, level glib.LogLevelFlags, message string, _ uintptr) {
	// Map GLib log levels to zerolog levels
	var event *zerolog.Event

	switch {
	case level&glib.GLogLevelErrorValue != 0:
		event = glibLogger.Error()
	case level&glib.GLogLevelCriticalValue != 0:
		event = glibLogger.Error()
	case level&glib.GLogLevelWarningValue != 0:
		event = glibLogger.Warn()
	case level&glib.GLogLevelMessageValue != 0:
		event = glibLogger.Info()
	case level&glib.GLogLevelInfoValue != 0:
		event = glibLogger.Info()
	case level&glib.GLogLevelDebugValue != 0:
		event = glibLogger.Debug()
	default:
		event = glibLogger.Debug()
	}

	// Add domain if present
	if domain != "" {
		event = event.Str("glib_domain", domain)
	}

	event.Msg(message)
}
