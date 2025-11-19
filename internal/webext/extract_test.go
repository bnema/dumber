package webext

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/config"
)

//go:embed testdata/assets/webext/dumber-webext.so
var testSo []byte

//go:embed testdata
var testDataFS embed.FS

func withTempXDG(t *testing.T) func() {
	t.Helper()
	base := t.TempDir()
	set := func(key, val string) {
		if err := os.Setenv(key, val); err != nil {
			t.Fatalf("set env %s: %v", key, err)
		}
	}
	set("XDG_DATA_HOME", filepath.Join(base, "data"))
	set("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	set("XDG_STATE_HOME", filepath.Join(base, "state"))
	return func() {
		os.Unsetenv("XDG_DATA_HOME")
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_STATE_HOME")
	}
}

func getTestFS(t *testing.T) fs.FS {
	t.Helper()
	// Create a sub-filesystem rooted at "testdata" so paths match production
	// (EnsureWebExtSO expects "assets/webext/dumber-webext.so", not "testdata/assets/...")
	subFS, err := fs.Sub(testDataFS, "testdata")
	if err != nil {
		t.Fatalf("failed to create sub-filesystem: %v", err)
	}
	return subFS
}

func TestEnsureWebExtSO(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, soPath string)
		expectExtract bool
		expectError   bool
	}{
		{
			name:          "first_extract",
			setup:         nil, // No setup, file doesn't exist
			expectExtract: true,
			expectError:   false,
		},
		{
			name: "reuse_existing_valid_file",
			setup: func(t *testing.T, soPath string) {
				// Pre-extract the file with correct content
				if err := os.WriteFile(soPath, testSo, 0o755); err != nil {
					t.Fatalf("pre-extract setup: %v", err)
				}
			},
			expectExtract: false,
			expectError:   false,
		},
		{
			name: "replace_corrupt_file",
			setup: func(t *testing.T, soPath string) {
				// Write corrupt data
				if err := os.WriteFile(soPath, []byte("corrupt"), 0o644); err != nil {
					t.Fatalf("corrupt file setup: %v", err)
				}
			},
			expectExtract: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withTempXDG(t)
			defer cleanup()

			testFS := getTestFS(t)

			// Get expected .so path
			dataDir, err := config.GetDataDir()
			if err != nil {
				t.Fatalf("get data dir: %v", err)
			}
			libexecDir := filepath.Join(filepath.Dir(dataDir), "libexec", "dumber")
			soPath := filepath.Join(libexecDir, "dumber-webext.so")

			// Run setup if provided
			if tt.setup != nil {
				// Ensure directory exists for setup
				if err := os.MkdirAll(libexecDir, 0755); err != nil {
					t.Fatalf("mkdir for setup: %v", err)
				}
				tt.setup(t, soPath)
			}

			// Get modtime before if file exists
			var modBefore time.Time
			if info, err := os.Stat(soPath); err == nil {
				modBefore = info.ModTime()
			}

			// Small sleep to ensure modtime would change if file is re-written
			if !modBefore.IsZero() {
				time.Sleep(10 * time.Millisecond)
			}

			// Call EnsureWebExtSO
			dir, err := EnsureWebExtSO(testFS)

			// Check error expectation
			if (err != nil) != tt.expectError {
				t.Fatalf("EnsureWebExtSO() error = %v, expectError %v", err, tt.expectError)
			}
			if tt.expectError {
				return
			}

			// Verify returned directory
			if dir != libexecDir {
				t.Errorf("returned dir = %v, want %v", dir, libexecDir)
			}

			// Verify file exists and has correct content
			data, err := os.ReadFile(soPath)
			if err != nil {
				t.Fatalf("read extracted .so: %v", err)
			}
			if string(data) != string(testSo) {
				t.Errorf("file content mismatch")
			}

			// Verify extraction behavior
			if tt.expectExtract {
				// File should be newly extracted
				info, err := os.Stat(soPath)
				if err != nil {
					t.Fatalf("stat .so: %v", err)
				}
				if !modBefore.IsZero() && modBefore.Equal(info.ModTime()) {
					t.Errorf("expected file to be re-extracted, but modtime unchanged")
				}
			} else {
				// File should be reused (modtime unchanged)
				info, err := os.Stat(soPath)
				if err != nil {
					t.Fatalf("stat .so: %v", err)
				}
				if !modBefore.Equal(info.ModTime()) {
					t.Errorf("expected file reuse (modtime preserved), got %v -> %v", modBefore, info.ModTime())
				}
			}
		})
	}
}
