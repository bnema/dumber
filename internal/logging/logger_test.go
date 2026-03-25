package logging

import (
	"os"
	"path/filepath"
	"testing"
)

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

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
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
	if err := file.Close(); err != nil {
		t.Fatalf("close underlying file: %v", err)
	}

	n, err := writer.Write([]byte("late\n"))
	if err != nil {
		t.Fatalf("write after external close: %v", err)
	}
	if n != len("late\n") {
		t.Fatalf("write after external close returned %d, want %d", n, len("late\n"))
	}
}
