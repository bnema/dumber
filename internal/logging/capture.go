package logging

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
		if closeErr := stdoutR.Close(); closeErr != nil {
			log.Printf("failed to close stdout read pipe: %v", closeErr)
		}
		if closeErr := stdoutW.Close(); closeErr != nil {
			log.Printf("failed to close stdout write pipe: %v", closeErr)
		}
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
	if err := unix.Dup3(int(stdoutW.Fd()), 1, 0); err != nil {
		log.Printf("failed to redirect stdout: %v", err)
	}
	if err := unix.Dup3(int(stderrW.Fd()), 2, 0); err != nil {
		log.Printf("failed to redirect stderr: %v", err)
	}

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
	if err := unix.Dup3(int(c.originalStdout.Fd()), 1, 0); err != nil {
		log.Printf("failed to restore stdout: %v", err)
	}
	if err := unix.Dup3(int(c.originalStderr.Fd()), 2, 0); err != nil {
		log.Printf("failed to restore stderr: %v", err)
	}

	// Close pipes
	if c.stdoutWrite != nil {
		if err := c.stdoutWrite.Close(); err != nil {
			log.Printf("failed to close stdout write pipe: %v", err)
		}
	}
	if c.stderrWrite != nil {
		if err := c.stderrWrite.Close(); err != nil {
			log.Printf("failed to close stderr write pipe: %v", err)
		}
	}
	if c.stdoutRead != nil {
		if err := c.stdoutRead.Close(); err != nil {
			log.Printf("failed to close stdout read pipe: %v", err)
		}
	}
	if c.stderrRead != nil {
		if err := c.stderrRead.Close(); err != nil {
			log.Printf("failed to close stderr read pipe: %v", err)
		}
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
						if _, err := c.originalStderr.WriteString(logLine); err != nil {
							log.Printf("failed to write to stderr: %v", err)
						}
					} else {
						if _, err := c.originalStdout.WriteString(logLine); err != nil {
							log.Printf("failed to write to stdout: %v", err)
						}
					}
				}
			}
		}
	}
}
