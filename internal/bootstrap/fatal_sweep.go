package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/rs/zerolog"
)

// fatalFileRegex matches pid_<pid>.fatal.txt and the rotated
// pid_<pid>.<mtime-unix>.fatal.txt left after PID reuse.
var fatalFileRegex = regexp.MustCompile(`^pid_(\d+)(?:\.(\d+))?\.fatal\.txt$`)

// sweepFatalOutputs handles fatal output files that no PID file points to,
// such as a crash before the PID file was written. Files of live processes
// and the current process's own file are skipped. Empty files are deleted;
// non-empty ones get a crash report. It returns the report paths.
func sweepFatalOutputs(
	ctx context.Context,
	logDir string,
	currentPID int,
	currentSessionID string,
	previous previousProcessChecker,
	logger *zerolog.Logger,
) []string {
	matches, err := filepath.Glob(filepath.Join(logDir, crashReportsDirName, "pid_*.fatal.txt"))
	if err != nil {
		return nil
	}
	var reports []string
	for _, path := range matches {
		m := fatalFileRegex.FindStringSubmatch(filepath.Base(path))
		if m == nil {
			continue
		}
		pid, convErr := strconv.Atoi(m[1])
		if convErr != nil || pid <= 0 {
			continue
		}
		rotated := m[2] != ""
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		if !rotated && (pid == currentPID || previous.alive(ctx, pid, "", info.ModTime())) {
			continue
		}
		if info.Size() == 0 {
			_ = os.Remove(path)
			continue
		}

		sessionID := orphanFatalSessionID(logDir, currentSessionID, pid, m[2])
		if logger != nil {
			logger.Warn().
				Int("crashed_pid", pid).
				Str("session_id", sessionID).
				Msg("previous process crashed (orphaned fatal output)")
		}
		reportPath, reportErr := writeCrashReport(logDir, sessionID, pid, path)
		if reportErr != nil {
			if logger != nil {
				logger.Warn().Err(reportErr).Msg("failed to write crash report")
			}
			continue
		}
		reports = append(reports, reportPath)
	}
	return reports
}

// orphanFatalSessionID picks the report ID for an orphaned fatal file: the
// newest other session log, unless that session already has a report; then
// a pid_<pid>[_<mtime>] ID.
func orphanFatalSessionID(logDir, currentSessionID string, pid int, mtime string) string {
	if lastLog := findLastSessionLog(logDir, currentSessionID); lastLog != "" {
		id := sessionIDFromLogPath(lastLog)
		reportPath := filepath.Join(logDir, crashReportsDirName, fmt.Sprintf("session_%s.crash.json", id))
		if validSessionID(id) {
			if _, err := os.Stat(reportPath); os.IsNotExist(err) {
				return id
			}
		}
	}
	id := fmt.Sprintf("pid_%d", pid)
	if mtime != "" {
		id += "_" + mtime
	}
	return id
}
