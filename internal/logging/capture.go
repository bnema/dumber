package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

type OutputCapture struct {
	originalStdout *os.File
	originalStderr *os.File
	stdoutRead     *os.File
	stdoutWrite    *os.File
	stderrRead     *os.File
	stderrWrite    *os.File
	logger         *Logger
	stopChan       chan struct{}
	started        bool
}

func NewOutputCapture(logger *Logger) *OutputCapture {
	return &OutputCapture{
		originalStdout: os.Stdout,
		originalStderr: os.Stderr,
		logger:         logger,
		stopChan:       make(chan struct{}),
	}
}

func (c *OutputCapture) Start() error {
	if c.started {
		return nil
	}

	// Create pipes for stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return err
	}

	// Create pipes for stderr
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		stdoutW.Close()
		return err
	}

	// Store pipe handles
	c.stdoutRead = stdoutR
	c.stdoutWrite = stdoutW
	c.stderrRead = stderrR
	c.stderrWrite = stderrW

	// Redirect stdout and stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW

	// Also redirect file descriptors at syscall level for C code
	syscall.Dup2(int(stdoutW.Fd()), 1)
	syscall.Dup2(int(stderrW.Fd()), 2)

	// Start goroutines to read and log
	go c.pipeToLogger(stdoutR, "STDOUT")
	go c.pipeToLogger(stderrR, "STDERR")

	c.started = true
	return nil
}

func (c *OutputCapture) Stop() {
	if !c.started {
		return
	}

	close(c.stopChan)

	// Restore original stdout and stderr
	os.Stdout = c.originalStdout
	os.Stderr = c.originalStderr

	// Restore file descriptors
	syscall.Dup2(int(c.originalStdout.Fd()), 1)
	syscall.Dup2(int(c.originalStderr.Fd()), 2)

	// Close pipes
	if c.stdoutWrite != nil {
		c.stdoutWrite.Close()
	}
	if c.stderrWrite != nil {
		c.stderrWrite.Close()
	}
	if c.stdoutRead != nil {
		c.stdoutRead.Close()
	}
	if c.stderrRead != nil {
		c.stderrRead.Close()
	}

	c.started = false
}

func (c *OutputCapture) pipeToLogger(r io.Reader, prefix string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-c.stopChan:
			return
		default:
			line := scanner.Text()
			if line != "" && c.logger != nil {
				// Avoid feedback loops by not logging our own log messages
				if !strings.Contains(line, "[STDOUT]") && !strings.Contains(line, "[STDERR]") {
					// Write directly to original stdout/stderr instead of going through logger
					// to avoid capture recursion
					timestamp := time.Now().Format("2006-01-02 15:04:05")
					logLine := fmt.Sprintf("[%s] INFO [%s] %s\n", timestamp, prefix, line)
					if prefix == "STDERR" {
						c.originalStderr.WriteString(logLine)
					} else {
						c.originalStdout.WriteString(logLine)
					}
				}
			}
		}
	}
}