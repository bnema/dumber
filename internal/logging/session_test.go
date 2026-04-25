package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupSessionLogFiles_RemovesOldestSessionLogs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	oldest := writeSessionLog(t, dir, "20260424_120000_0001", base)
	secondOldest := writeSessionLog(t, dir, "20260424_120001_0002", base.Add(time.Second))
	secondNewest := writeSessionLog(t, dir, "20260424_120002_0003", base.Add(2*time.Second))
	newest := writeSessionLog(t, dir, "20260424_120003_0004", base.Add(3*time.Second))
	nonSession := filepath.Join(dir, "dumber.log")
	if err := os.WriteFile(nonSession, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write non-session log: %v", err)
	}

	removed, err := CleanupSessionLogFiles(dir, 2)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	assertMissing(t, oldest)
	assertMissing(t, secondOldest)
	assertExists(t, secondNewest)
	assertExists(t, newest)
	assertExists(t, nonSession)
}

func TestCleanupSessionLogFiles_DisabledWhenMaxFilesZero(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeSessionLog(t, dir, "20260424_120000_0001", time.Now())

	removed, err := CleanupSessionLogFiles(dir, 0)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	assertExists(t, path)
}

func TestCleanupSessionLogFiles_NoRemovalWhenUnderLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	first := writeSessionLog(t, dir, "20260424_120000_0001", base)
	second := writeSessionLog(t, dir, "20260424_120001_0002", base.Add(time.Second))

	removed, err := CleanupSessionLogFiles(dir, 3)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}

	assertExists(t, first)
	assertExists(t, second)
}

func TestCleanupSessionLogFiles_UsesFilenameTiebreakerForEqualModTimes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	oldestByName := writeSessionLog(t, dir, "20260424_120000_0001", base)
	keptByName := writeSessionLog(t, dir, "20260424_120000_0002", base)
	newestByName := writeSessionLog(t, dir, "20260424_120000_0003", base)

	removed, err := CleanupSessionLogFiles(dir, 2)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	assertMissing(t, oldestByName)
	assertExists(t, keptByName)
	assertExists(t, newestByName)
}

func TestCleanupSessionLogFiles_KeepsSpecifiedSessionWithinLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	keepID := "20260424_120000_0001"

	oldestKept := writeSessionLog(t, dir, keepID, base)
	middle := writeSessionLog(t, dir, "20260424_120001_0002", base.Add(time.Second))
	newest := writeSessionLog(t, dir, "20260424_120002_0003", base.Add(2*time.Second))

	removed, err := CleanupSessionLogFiles(dir, 1, keepID)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	assertExists(t, oldestKept)
	assertMissing(t, middle)
	assertMissing(t, newest)
}

func TestCleanupSessionLogFiles_BoundsSpecifiedSessionsToMaxFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	oldestKeptID := "20260424_120000_0001"
	middleKeptID := "20260424_120001_0002"
	newestKeptID := "20260424_120002_0003"
	oldestKept := writeSessionLog(t, dir, oldestKeptID, base)
	middleKept := writeSessionLog(t, dir, middleKeptID, base.Add(time.Second))
	newestKept := writeSessionLog(t, dir, newestKeptID, base.Add(2*time.Second))

	removed, err := CleanupSessionLogFiles(dir, 2, oldestKeptID, middleKeptID, newestKeptID)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	assertMissing(t, oldestKept)
	assertExists(t, middleKept)
	assertExists(t, newestKept)
}

func TestCleanupSessionLogFiles_MissingDirectoryIsNoop(t *testing.T) {
	t.Parallel()

	removed, err := CleanupSessionLogFiles(filepath.Join(t.TempDir(), "missing"), 2)
	if err != nil {
		t.Fatalf("cleanup session log files: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
}

func writeSessionLog(t *testing.T, dir, sessionID string, modTime time.Time) string {
	t.Helper()

	path := filepath.Join(dir, SessionFilename(sessionID))
	if err := os.WriteFile(path, []byte(sessionID), 0o644); err != nil {
		t.Fatalf("write session log %s: %v", sessionID, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set mod time for %s: %v", sessionID, err)
	}
	return path
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err = %v", path, err)
	}
}
