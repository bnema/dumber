package webext

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSOContent is fake .so content for testing
var mockSOContent = []byte("ELF_MOCK_SO_CONTENT_FOR_TESTING_12345")

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

// createMockFS creates an in-memory filesystem with mock .so content
func createMockFS() fstest.MapFS {
	return fstest.MapFS{
		"assets/webext/dumber-webext.so": &fstest.MapFile{
			Data:    mockSOContent,
			Mode:    0o755,
			ModTime: time.Now(),
		},
	}
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
				require.NoError(t, os.WriteFile(soPath, mockSOContent, 0o755))
			},
			expectExtract: false,
			expectError:   false,
		},
		{
			name: "replace_corrupt_file",
			setup: func(t *testing.T, soPath string) {
				// Write corrupt data
				require.NoError(t, os.WriteFile(soPath, []byte("corrupt"), 0o644))
			},
			expectExtract: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withTempXDG(t)
			defer cleanup()

			mockFS := createMockFS()

			// Get expected .so path
			dataDir, err := config.GetDataDir()
			require.NoError(t, err)

			libexecDir := filepath.Join(filepath.Dir(dataDir), "libexec", "dumber")
			soPath := filepath.Join(libexecDir, "dumber-webext.so")

			// Run setup if provided
			if tt.setup != nil {
				// Ensure directory exists for setup
				require.NoError(t, os.MkdirAll(libexecDir, 0755))
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
			dir, err := EnsureWebExtSO(mockFS)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify returned directory
			assert.Equal(t, libexecDir, dir)

			// Verify file exists and has correct content
			data, err := os.ReadFile(soPath)
			require.NoError(t, err)
			assert.Equal(t, mockSOContent, data)

			// Verify extraction behavior
			info, err := os.Stat(soPath)
			require.NoError(t, err)

			if tt.expectExtract {
				// File should be newly extracted
				if !modBefore.IsZero() {
					assert.NotEqual(t, modBefore, info.ModTime(), "expected file to be re-extracted")
				}
			} else {
				// File should be reused (modtime unchanged)
				assert.Equal(t, modBefore, info.ModTime(), "expected file reuse (modtime preserved)")
			}
		})
	}
}

func TestEnsureWebExtSO_MissingAsset(t *testing.T) {
	cleanup := withTempXDG(t)
	defer cleanup()

	// Empty filesystem - no .so file
	emptyFS := fstest.MapFS{}

	_, err := EnsureWebExtSO(emptyFS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read embedded .so")
}

func TestFileHashMatches(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("matching hash", func(t *testing.T) {
		path := filepath.Join(tmpDir, "match.bin")
		content := []byte("test content")
		require.NoError(t, os.WriteFile(path, content, 0644))

		// Calculate expected hash
		expected := sha256Sum(content)
		assert.True(t, fileHashMatches(path, expected))
	})

	t.Run("non-matching hash", func(t *testing.T) {
		path := filepath.Join(tmpDir, "nomatch.bin")
		require.NoError(t, os.WriteFile(path, []byte("test content"), 0644))

		// Different hash
		var wrongHash [32]byte
		assert.False(t, fileHashMatches(path, wrongHash))
	})

	t.Run("file does not exist", func(t *testing.T) {
		var anyHash [32]byte
		assert.False(t, fileHashMatches("/nonexistent/path", anyHash))
	})
}

func TestWriteAtomically(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("successful write", func(t *testing.T) {
		path := filepath.Join(tmpDir, "atomic.bin")
		content := []byte("atomic content")

		err := writeAtomically(path, content, 0o644)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("sets permissions", func(t *testing.T) {
		path := filepath.Join(tmpDir, "perms.bin")
		content := []byte("test")

		err := writeAtomically(path, content, 0o755)
		require.NoError(t, err)

		info, err := os.Stat(path)
		require.NoError(t, err)
		// Check executable bit is set (on Unix)
		assert.True(t, info.Mode()&0o100 != 0, "expected executable permission")
	})
}

// sha256Sum is a helper to compute SHA-256 hash
func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}
