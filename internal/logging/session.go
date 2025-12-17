package logging

import (
	"crypto/rand"
	"encoding/hex"
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
