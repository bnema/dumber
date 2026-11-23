package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
)

// FatalOutputPath returns the path of the Go fatal output file for a process.
func FatalOutputPath(logDir string, pid int) string {
	return filepath.Join(logDir, crashReportsDirName, fmt.Sprintf("pid_%d.fatal.txt", pid))
}

// EnableCrashOutput routes Go runtime fatal output (panics, throws) to a
// per-PID file under <logDir>/crashes. The cleanup disables the redirect and
// removes the file when nothing was written. An empty logDir is a no-op.
func EnableCrashOutput(logDir string) (cleanup func(), err error) {
	noop := func() {}
	if logDir == "" {
		return noop, nil
	}
	if mkErr := os.MkdirAll(filepath.Join(logDir, crashReportsDirName), dirPerm); mkErr != nil {
		return noop, fmt.Errorf("create crash dir: %w", mkErr)
	}
	path := FatalOutputPath(logDir, os.Getpid())
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		return noop, fmt.Errorf("open fatal output file: %w", err)
	}
	setErr := debug.SetCrashOutput(f, debug.CrashOptions{})
	// The runtime dups the fd, so the original can be closed.
	closeErr := f.Close()
	if setErr != nil {
		_ = os.Remove(path)
		return noop, fmt.Errorf("set crash output: %w", setErr)
	}
	if closeErr != nil {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		_ = os.Remove(path)
		return noop, fmt.Errorf("close fatal output file: %w", closeErr)
	}
	return func() {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		if info, statErr := os.Stat(path); statErr == nil && info.Size() == 0 {
			_ = os.Remove(path)
		}
	}, nil
}
