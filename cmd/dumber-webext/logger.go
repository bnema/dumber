package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// logToFile writes directly to the webext log file for initialization logs
// that occur before we have a page context for IPC logging.
func logToFile(format string, args ...interface{}) {
	// Get XDG state dir for logs
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home := os.Getenv("HOME")
		if home != "" {
			stateDir = filepath.Join(home, ".local", "state")
		}
	}
	if stateDir == "" {
		// Fallback to stderr
		log.Printf(format, args...)
		return
	}

	logPath := filepath.Join(stateDir, "dumber", "logs", "dumber-webext.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf(format, args...)
		return
	}
	defer f.Close()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}

// logMessage represents a structured log entry sent to the UI process
type logMessage struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// sendLogToUI sends a log message to the UI process via UserMessage
// Falls back to log.Printf if page is unavailable
func sendLogToUI(page *webkitwebprocessextension.WebPage, level, message string) {
	// Drop noisy debug/info logs to avoid flooding the UI process.
	// Metrics are allowed through when enabled.
	if level == "debug" || level == "info" {
		return
	}

	// Create structured log entry
	entry := logMessage{
		Level:     level,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[webext-logger] Failed to marshal log entry: %v", err)
		return
	}

	// If no page context, fallback to standard logging
	if page == nil {
		log.Printf("[WebExt-%s] %s", level, message)
		return
	}

	// Send message to UI process
	variant := glib.NewVariantString(string(jsonData))
	msg := webkitwebprocessextension.NewUserMessage("extension:log", variant)

	// Fire-and-forget (no callback needed for logging)
	page.SendMessageToView(context.Background(), msg, nil)
}

// LogDebug sends a debug-level log message to the UI process
func LogDebug(page *webkitwebprocessextension.WebPage, format string, args ...interface{}) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	sendLogToUI(page, "debug", message)
}

// LogInfo sends an info-level log message to the UI process
func LogInfo(page *webkitwebprocessextension.WebPage, format string, args ...interface{}) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	sendLogToUI(page, "info", message)
}

// LogWarn sends a warning-level log message to the UI process
func LogWarn(page *webkitwebprocessextension.WebPage, format string, args ...interface{}) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	sendLogToUI(page, "warn", message)
}

// LogError sends an error-level log message to the UI process
func LogError(page *webkitwebprocessextension.WebPage, format string, args ...interface{}) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	sendLogToUI(page, "error", message)
}

// LogMetrics sends a metrics-level log message when webRequest metrics are enabled.
// Uses warn level to ensure delivery (debug/info are dropped by default).
func LogMetrics(page *webkitwebprocessextension.WebPage, format string, args ...interface{}) {
	if !enableWebRequestMetrics {
		return
	}
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	sendLogToUI(page, "metrics", message)
}
