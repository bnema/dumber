package logging

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GenerateSessionID creates a unique session identifier.
// Format: YYYYMMDD_HHMMSS_xxxx (timestamp + 4 random hex chars)
// Example: 20251217_205106_a7b3
func GenerateSessionID() string {
	now := time.Now()
	random := make([]byte, 2)
	_, _ = rand.Read(random)
	return now.Format("20060102_150405") + "_" + hex.EncodeToString(random)
}

// ShortSessionID extracts the short ID (last 4 hex chars) from a full session ID.
// Example: "20251217_205106_a7b3" -> "a7b3"
func ShortSessionID(sessionID string) string {
	if len(sessionID) < 4 {
		return sessionID
	}
	return sessionID[len(sessionID)-4:]
}

// ParseSessionFilename extracts session info from a log filename.
// Example: "session_20251217_205106_a7b3.log" -> "20251217_205106_a7b3", true
func ParseSessionFilename(filename string) (sessionID string, ok bool) {
	const prefix = "session_"
	const suffix = ".log"

	if len(filename) < len(prefix)+len(suffix) {
		return "", false
	}
	if filename[:len(prefix)] != prefix {
		return "", false
	}
	if filename[len(filename)-len(suffix):] != suffix {
		return "", false
	}

	return filename[len(prefix) : len(filename)-len(suffix)], true
}

// SessionFilename generates the log filename for a session ID.
// Example: "20251217_205106_a7b3" -> "session_20251217_205106_a7b3.log"
func SessionFilename(sessionID string) string {
	return "session_" + sessionID + ".log"
}

type sessionLogFile struct {
	path      string
	filename  string
	sessionID string
	modTime   time.Time
}

// CleanupSessionLogFiles removes old session log files when their count exceeds maxFiles.
// It keeps the newest files by modification time and never removes the supplied session IDs.
// A maxFiles value of 0 disables count-based cleanup.
func CleanupSessionLogFiles(logDir string, maxFiles int, keepSessionIDs ...string) (int, error) {
	if logDir == "" || maxFiles <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read log directory: %w", err)
	}

	keep := make(map[string]struct{}, len(keepSessionIDs))
	for _, sessionID := range keepSessionIDs {
		if sessionID == "" {
			continue
		}
		keep[sessionID] = struct{}{}
	}

	files := make([]sessionLogFile, 0, len(entries))
	operationErrs := make([]error, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		sessionID, ok := ParseSessionFilename(entry.Name())
		if !ok {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			operationErrs = append(operationErrs, fmt.Errorf("stat %s: %w", filepath.Join(logDir, entry.Name()), infoErr))
			continue
		}

		files = append(files, sessionLogFile{
			path:      filepath.Join(logDir, entry.Name()),
			filename:  entry.Name(),
			sessionID: sessionID,
			modTime:   info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].filename > files[j].filename
		}
		return files[i].modTime.After(files[j].modTime)
	})

	kept := 0
	removed := 0
	for _, file := range files {
		if _, shouldKeep := keep[file.sessionID]; shouldKeep {
			kept++
			continue
		}
		if kept < maxFiles {
			kept++
			continue
		}

		if err := os.Remove(file.path); err != nil {
			operationErrs = append(operationErrs, fmt.Errorf("remove %s: %w", file.path, err))
			continue
		}
		removed++
	}

	return removed, errors.Join(operationErrs...)
}
