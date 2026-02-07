package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionExitClass string

const (
	SessionExitCleanExit                 SessionExitClass = "clean_exit"
	SessionExitMainProcessCrashOrAbrupt  SessionExitClass = "main_process_crash_or_abrupt"
	SessionExitExternalKillOrOOMInferred SessionExitClass = "external_kill_or_oom_inferred"
	SessionExitUnknown                   SessionExitClass = "unknown"
)

type SessionExitClassification struct {
	SessionID          string
	Class              SessionExitClass
	Inference          string
	Reason             string
	StartupAt          *time.Time
	ShutdownAt         *time.Time
	AbruptDetectedAt   *time.Time
	LastMarkerObserved time.Time
}

func ClassifySessionExitFromMarkers(lockDir, sessionID string) (SessionExitClassification, error) {
	classification := SessionExitClassification{
		SessionID: sessionID,
		Class:     SessionExitUnknown,
		Inference: "marker-missing",
		Reason:    "no known marker files found for session",
	}
	if lockDir == "" || sessionID == "" {
		return classification, fmt.Errorf("lockDir and sessionID are required")
	}

	startupAt, startupExists, startupModTime, err := readMarkerTime(startupMarkerPath(lockDir, sessionID), "")
	if err != nil {
		return classification, err
	}
	shutdownAt, shutdownExists, shutdownModTime, err := readMarkerTime(shutdownMarkerPath(lockDir, sessionID), "")
	if err != nil {
		return classification, err
	}
	abruptAt, abruptExists, abruptModTime, err := readMarkerTime(abruptMarkerPath(lockDir, sessionID), "detected_at=")
	if err != nil {
		return classification, err
	}

	// If the startup marker was already cleaned up (writeShutdownMarker removes
	// it), fall back to the started_at line embedded in the shutdown marker.
	if startupAt == nil && shutdownExists {
		startupAt, _, _, _ = readMarkerTime(shutdownMarkerPath(lockDir, sessionID), "started_at=")
	}
	classification.StartupAt = startupAt
	classification.ShutdownAt = shutdownAt
	classification.AbruptDetectedAt = abruptAt
	classification.LastMarkerObserved = latestNonZero(startupModTime, shutdownModTime, abruptModTime)

	switch {
	case shutdownExists:
		classification.Class = SessionExitCleanExit
		classification.Inference = "marker-confirmed"
		classification.Reason = "shutdown marker present"
	case abruptExists:
		classification.Class = SessionExitMainProcessCrashOrAbrupt
		classification.Inference = "marker-confirmed"
		classification.Reason = "abrupt marker present and no shutdown marker"
	case startupExists:
		classification.Class = SessionExitExternalKillOrOOMInferred
		classification.Inference = "best-effort"
		classification.Reason = "startup marker present without shutdown/abrupt markers"
	}

	return classification, nil
}

func BuildSessionExitReport(lockDir string) ([]SessionExitClassification, error) {
	if lockDir == "" {
		return nil, nil
	}

	sessionIDs, err := discoverSessionIDsFromMarkers(lockDir)
	if err != nil {
		return nil, err
	}
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	report := make([]SessionExitClassification, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		classification, classErr := ClassifySessionExitFromMarkers(lockDir, sessionID)
		if classErr != nil {
			return nil, classErr
		}
		report = append(report, classification)
	}

	sort.Slice(report, func(i, j int) bool {
		return report[i].LastMarkerObserved.After(report[j].LastMarkerObserved)
	})
	return report, nil
}

func discoverSessionIDsFromMarkers(lockDir string) ([]string, error) {
	pattern := filepath.Join(lockDir, "session_*.marker")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		base := filepath.Base(path)
		sessionID, ok := markerSessionID(base)
		if !ok {
			continue
		}
		ids[sessionID] = struct{}{}
	}

	sessionIDs := make([]string, 0, len(ids))
	for id := range ids {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)
	return sessionIDs, nil
}

func markerSessionID(base string) (string, bool) {
	if !strings.HasPrefix(base, "session_") || !strings.HasSuffix(base, ".marker") {
		return "", false
	}

	trimmed := strings.TrimSuffix(strings.TrimPrefix(base, "session_"), ".marker")
	switch {
	case strings.HasSuffix(trimmed, ".startup"):
		return strings.TrimSuffix(trimmed, ".startup"), true
	case strings.HasSuffix(trimmed, ".shutdown"):
		return strings.TrimSuffix(trimmed, ".shutdown"), true
	case strings.HasSuffix(trimmed, ".abrupt"):
		return strings.TrimSuffix(trimmed, ".abrupt"), true
	default:
		return "", false
	}
}

func readMarkerTime(path, keyPrefix string) (*time.Time, bool, time.Time, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, time.Time{}, nil
		}
		return nil, false, time.Time{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, true, stat.ModTime(), err
	}

	raw := strings.TrimSpace(string(content))
	if raw == "" {
		return nil, true, stat.ModTime(), nil
	}

	if keyPrefix != "" {
		lines := strings.Split(raw, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, keyPrefix) {
				parsed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimPrefix(line, keyPrefix))
				if parseErr != nil {
					return nil, true, stat.ModTime(), nil
				}
				return &parsed, true, stat.ModTime(), nil
			}
		}
		return nil, true, stat.ModTime(), nil
	}

	// When no key prefix is given, parse the first line as the timestamp.
	// Multi-line markers may contain additional metadata (e.g. started_at=).
	firstLine, _, _ := strings.Cut(raw, "\n")
	parsed, parseErr := time.Parse(time.RFC3339Nano, firstLine)
	if parseErr != nil {
		return nil, true, stat.ModTime(), nil
	}
	return &parsed, true, stat.ModTime(), nil
}

func latestNonZero(values ...time.Time) time.Time {
	var latest time.Time
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		if latest.IsZero() || value.After(latest) {
			latest = value
		}
	}
	return latest
}
