package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestRedactURLUsesHostOnlyForAbsoluteURLs(t *testing.T) {
	t.Parallel()

	got := RedactURL("https://user:pass@example.com/private/path?token=secret#frag")
	if got != "example.com" {
		t.Fatalf("RedactURL returned %q, want %q", got, "example.com")
	}
}

func TestRedactURLRedactsLocalOrOpaqueInputs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"/home/user/private/file.html?token=secret", "not a url"} {
		if got := RedactURL(raw); got != RedactedURLPlaceholder {
			t.Fatalf("RedactURL(%q) returned %q, want %q", raw, got, RedactedURLPlaceholder)
		}
	}
}

func TestTruncateURLRedactsBeforeTruncating(t *testing.T) {
	t.Parallel()

	got := TruncateURL("https://example.com/private/path?token=secret#frag", 96)
	if got != "example.com" {
		t.Fatalf("TruncateURL returned %q, want %q", got, "example.com")
	}
	if strings.Contains(got, "token") || strings.Contains(got, "/private") || strings.Contains(got, "frag") {
		t.Fatalf("TruncateURL leaked sensitive URL data: %q", got)
	}
}

func TestSafeURLHostDropsPathQueryAndFragment(t *testing.T) {
	t.Parallel()

	got := SafeURLHost("https://example.com/private/path?token=secret#frag")
	if got != "example.com" {
		t.Fatalf("SafeURLHost returned %q, want %q", got, "example.com")
	}
}

func TestSafeURLHostReturnsEmptyForOpaqueOrInvalidInput(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"about:blank", "not a url"} {
		if got := SafeURLHost(raw); got != "" {
			t.Fatalf("SafeURLHost(%q) returned %q, want empty host", raw, got)
		}
	}
}

func TestRedactURLStripsQueryFragmentFromAboutOpaqueURLs(t *testing.T) {
	t.Parallel()

	got := RedactURL("about:blank?token=secret#frag")
	if want := "about:blank"; got != want {
		t.Fatalf("RedactURL(about:blank?token=secret#frag) = %q, want %q", got, want)
	}
	if strings.Contains(got, "token") || strings.Contains(got, "secret") || strings.Contains(got, "frag") {
		t.Fatalf("RedactURL leaked query/fragment from about: opaque URL: %q", got)
	}
}

func TestRedactURLRedactsFileURLPathQueryFragment(t *testing.T) {
	t.Parallel()

	got := RedactURL("file:///home/user/private?token=secret#frag")
	if got != "file:"+RedactedURLPlaceholder {
		t.Fatalf("RedactURL(file:///home/user/private?token=secret#frag) = %q, want %q", got, "file:"+RedactedURLPlaceholder)
	}
	if strings.Contains(got, "home") || strings.Contains(got, "user") || strings.Contains(got, "private") {
		t.Fatalf("RedactURL leaked file path from file:// URL: %q", got)
	}
	if strings.Contains(got, "token") || strings.Contains(got, "secret") || strings.Contains(got, "frag") {
		t.Fatalf("RedactURL leaked query/fragment from file:// URL: %q", got)
	}
}

func TestRedactURLReturnsPlaceholderForInvalidParse(t *testing.T) {
	t.Parallel()

	got := RedactURL("http://[::1")
	if got != RedactedURLPlaceholder {
		t.Fatalf("RedactURL(http://[::1) = %q, want %q", got, RedactedURLPlaceholder)
	}
}

func TestNewWithFileCreatesUserPrivateLogDirectoryAndFile(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	logDir := filepath.Join(parent, "logs")
	logger, cleanup, err := NewWithFile(Config{Level: zerolog.InfoLevel, Format: logFormatJSON}, FileConfig{
		Enabled:   true,
		LogDir:    logDir,
		SessionID: "test-session",
	})
	if err != nil {
		t.Fatalf("NewWithFile: %v", err)
	}
	defer cleanup()

	logger.Info().Msg("hello")
	cleanup()

	dirInfo, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("stat log dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("log dir permissions = %o, want 700", got)
	}

	fileInfo, err := os.Stat(filepath.Join(logDir, SessionFilename("test-session")))
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", got)
	}
}

func TestSessionFileWriter_DropsWritesAfterClose(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp log file: %v", err)
	}

	writer := newSessionFileWriter(file)

	n, err := writer.Write([]byte("first\n"))
	if err != nil {
		t.Fatalf("write before close: %v", err)
	}
	if n != len("first\n") {
		t.Fatalf("write before close returned %d, want %d", n, len("first\n"))
	}

	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("close writer: %v", closeErr)
	}

	n, err = writer.Write([]byte("second\n"))
	if err != nil {
		t.Fatalf("write after close: %v", err)
	}
	if n != len("second\n") {
		t.Fatalf("write after close returned %d, want %d", n, len("second\n"))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if got, want := string(data), "first\n"; got != want {
		t.Fatalf("log file contents = %q, want %q", got, want)
	}
}

func TestSessionFileWriter_IgnoresExternallyClosedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session.log")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp log file: %v", err)
	}

	writer := newSessionFileWriter(file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close underlying file: %v", closeErr)
	}

	n, err := writer.Write([]byte("late\n"))
	if err != nil {
		t.Fatalf("write after external close: %v", err)
	}
	if n != len("late\n") {
		t.Fatalf("write after external close returned %d, want %d", n, len("late\n"))
	}
}
