package usecase

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/domain/entity"
)

// isSessionLocked checks if a session has an active lock file.
// Lock files are created in lockDir with format: session_<sessionID>.lock
// This matches the format used by bootstrap.sessionLockPath().
func isSessionLocked(lockDir string, sessionID entity.SessionID) bool {
	if lockDir == "" {
		return false
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf("session_%s.lock", sessionID))
	_, err := os.Stat(lockPath)
	return err == nil
}
