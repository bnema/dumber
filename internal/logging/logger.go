package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	mu        sync.RWMutex
	level     LogLevel
	outputs   []io.Writer
	rotator   *LogRotator
	formatter LogFormatter
}

type LogFormatter interface {
	Format(level LogLevel, message string, source string) string
}

type TextFormatter struct{}

func (f *TextFormatter) Format(level LogLevel, message string, source string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := levelNames[level]
	if source != "" {
		return fmt.Sprintf("[%s] %s [%s] %s\n", timestamp, levelName, source, message)
	}
	return fmt.Sprintf("[%s] %s %s\n", timestamp, levelName, message)
}

type JSONFormatter struct{}

func (f *JSONFormatter) Format(level LogLevel, message string, source string) string {
	timestamp := time.Now().Format(time.RFC3339)
	levelName := levelNames[level]
	if source != "" {
		return fmt.Sprintf(`{"timestamp":"%s","level":"%s","source":"%s","message":"%s"}`+"\n",
			timestamp, levelName, source, strings.ReplaceAll(message, `"`, `\"`))
	}
	return fmt.Sprintf(`{"timestamp":"%s","level":"%s","message":"%s"}`+"\n",
		timestamp, levelName, strings.ReplaceAll(message, `"`, `\"`))
}

func NewFormatter(format string) LogFormatter {
	switch strings.ToLower(format) {
	case "json":
		return &JSONFormatter{}
	default:
		return &TextFormatter{}
	}
}

func parseLevel(levelStr string) LogLevel {
	switch strings.ToLower(levelStr) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

var globalLogger *Logger
var globalLoggerMux sync.RWMutex

func Init(logDir string, level string, format string, enableFileLog bool, maxSize, maxBackups, maxAge int, compress bool) error {
	globalLoggerMux.Lock()
	defer globalLoggerMux.Unlock()

	err := os.MkdirAll(logDir, 0750)
	if err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	outputs := []io.Writer{}

	// Always include stdout
	outputs = append(outputs, os.Stdout)

	var rotator *LogRotator
	if enableFileLog {
		rotator, err = NewLogRotator(logDir, maxSize, maxBackups, maxAge, compress)
		if err != nil {
			return fmt.Errorf("failed to create log rotator: %w", err)
		}
		outputs = append(outputs, rotator)
	}

	globalLogger = &Logger{
		level:     parseLevel(level),
		outputs:   outputs,
		rotator:   rotator,
		formatter: NewFormatter(format),
	}

	// Redirect standard log package
	log.SetOutput(globalLogger)
	log.SetFlags(0) // We'll handle formatting

	return nil
}

func GetLogger() *Logger {
	globalLoggerMux.RLock()
	defer globalLoggerMux.RUnlock()
	return globalLogger
}

// Implement io.Writer interface for standard log package redirection
func (l *Logger) Write(p []byte) (n int, err error) {
	message := strings.TrimSpace(string(p))
	if message == "" {
		return len(p), nil
	}

	l.writeLog(INFO, message, "")
	return len(p), nil
}

func (l *Logger) writeLog(level LogLevel, message string, source string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if level < l.level {
		return
	}

	formatted := l.formatter.Format(level, message, source)

	for _, output := range l.outputs {
		if _, err := output.Write([]byte(formatted)); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log output: %v\n", err)
		}
	}
}

func (l *Logger) WriteTagged(source string, message string) {
	l.WriteFileOnly(INFO, message, source)
}

// WriteFileOnly writes a log entry only to the file (not stdout) to avoid interfering with CLI operations
func (l *Logger) WriteFileOnly(level LogLevel, message string, source string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if level < l.level {
		return
	}

	formatted := l.formatter.Format(level, message, source)

	// Write only to file outputs (skip stdout)
	for _, output := range l.outputs {
		if output != os.Stdout {
			if _, err := output.Write([]byte(formatted)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write log output: %v\n", err)
			}
		}
	}
}

func Debug(message string) {
	if l := GetLogger(); l != nil {
		l.writeLog(DEBUG, message, "")
	}
}

func Info(message string) {
	if l := GetLogger(); l != nil {
		l.writeLog(INFO, message, "")
	}
}

func Warn(message string) {
	if l := GetLogger(); l != nil {
		l.writeLog(WARN, message, "")
	}
}

func Error(message string) {
	if l := GetLogger(); l != nil {
		l.writeLog(ERROR, message, "")
	}
}

func Fatal(message string) {
	if l := GetLogger(); l != nil {
		l.writeLog(FATAL, message, "")
	}
	os.Exit(1)
}

func CaptureWebKitLog(message string) {
	if l := GetLogger(); l != nil {
		l.WriteTagged("CONSOLE", message)
	}
}

// StartConsoleCapture starts capturing stdout for console messages
// StartConsoleCapture starts capturing stdout for console messages and writing to console.log
// StartConsoleCapture - placeholder for console message capture
func StartConsoleCapture() {
	// Placeholder - console message capture will be implemented differently
}

// LogLevelInfo returns the INFO log level constant for external packages
func LogLevelInfo() LogLevel {
	return INFO
}
