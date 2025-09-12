package logging

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
)

// SetupCrashHandler sets up signal handlers to catch crashes and log them
func SetupCrashHandler() {
	c := make(chan os.Signal, 1)

	// Listen for crash signals
	signal.Notify(c,
		syscall.SIGSEGV, // Segmentation violation
		syscall.SIGABRT, // Abort signal
		syscall.SIGFPE,  // Floating point exception
		syscall.SIGBUS,  // Bus error
		syscall.SIGILL,  // Illegal instruction
	)

	go func() {
		sig := <-c
		handleCrash(sig)
	}()
}

// SetupPanicRecovery sets up a global panic recovery handler
// This should be called with defer at the start of main functions
func SetupPanicRecovery() {
	if r := recover(); r != nil {
		logPanic(r)
	}
}

// handleCrash logs crash information and exits
func handleCrash(sig os.Signal) {
	logger := GetLogger()
	if logger == nil {
		fmt.Fprintf(os.Stderr, "CRASH: Caught signal %v but no logger available\n", sig)
		os.Exit(1)
	}

	// Log the crash
	crashMsg := fmt.Sprintf("CRASH: Caught fatal signal %v", sig)
	logger.writeLog(FATAL, crashMsg, "CRASH")

	// Log stack trace
	stack := debug.Stack()
	stackMsg := fmt.Sprintf("Stack trace:\n%s", string(stack))
	logger.writeLog(FATAL, stackMsg, "CRASH")

	// Log system info
	sysInfo := fmt.Sprintf("Go version: %s, OS: %s, Arch: %s, NumCPU: %d",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	logger.writeLog(FATAL, sysInfo, "CRASH")

	// Log memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memInfo := fmt.Sprintf("Memory - Alloc: %d KB, TotalAlloc: %d KB, Sys: %d KB, NumGC: %d",
		m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)
	logger.writeLog(FATAL, memInfo, "CRASH")

	// Force flush logs
	if logger.rotator != nil {
		logger.rotator.Close()
	}

	// Exit with error code
	os.Exit(128 + int(sig.(syscall.Signal)))
}

// logPanic logs panic information
func logPanic(r any) {
	logger := GetLogger()
	if logger == nil {
		fmt.Fprintf(os.Stderr, "PANIC: %v but no logger available\n", r)
		return
	}

	// Log the panic
	panicMsg := fmt.Sprintf("PANIC: %v", r)
	logger.writeLog(FATAL, panicMsg, "PANIC")

	// Log stack trace
	stack := debug.Stack()
	stackMsg := fmt.Sprintf("Stack trace:\n%s", string(stack))
	logger.writeLog(FATAL, stackMsg, "PANIC")

	// Re-panic to maintain normal panic behavior
	panic(r)
}

// LogCrashInfo logs information that might be helpful for crash analysis
func LogCrashInfo(context string) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	logger.writeLog(DEBUG, fmt.Sprintf("Crash context checkpoint: %s", context), "CRASH_DEBUG")
}
