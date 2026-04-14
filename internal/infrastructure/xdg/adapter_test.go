package xdg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_DownloadDir(t *testing.T) {
	adapter := New()

	t.Run("returns XDG_DOWNLOAD_DIR when set", func(t *testing.T) {
		// Save and restore original env.
		original := os.Getenv("XDG_DOWNLOAD_DIR")
		defer func() {
			if original == "" {
				os.Unsetenv("XDG_DOWNLOAD_DIR")
			} else {
				os.Setenv("XDG_DOWNLOAD_DIR", original)
			}
		}()

		os.Setenv("XDG_DOWNLOAD_DIR", "/custom/downloads")

		dir, err := adapter.DownloadDir()

		require.NoError(t, err)
		assert.Equal(t, "/custom/downloads", dir)
	})

	t.Run("falls back to ~/Downloads when XDG_DOWNLOAD_DIR not set", func(t *testing.T) {
		// Save and restore original env.
		original := os.Getenv("XDG_DOWNLOAD_DIR")
		defer func() {
			if original == "" {
				os.Unsetenv("XDG_DOWNLOAD_DIR")
			} else {
				os.Setenv("XDG_DOWNLOAD_DIR", original)
			}
		}()

		os.Unsetenv("XDG_DOWNLOAD_DIR")

		dir, err := adapter.DownloadDir()

		require.NoError(t, err)

		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expected := filepath.Join(home, "Downloads")
		assert.Equal(t, expected, dir)
	})
}

func TestAdapter_RuntimeDir(t *testing.T) {
	adapter := New()

	t.Run("returns XDG_RUNTIME_DIR when set", func(t *testing.T) {
		original := os.Getenv("XDG_RUNTIME_DIR")
		defer func() {
			if original == "" {
				os.Unsetenv("XDG_RUNTIME_DIR")
			} else {
				os.Setenv("XDG_RUNTIME_DIR", original)
			}
		}()

		os.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")

		dir, err := adapter.RuntimeDir()

		require.NoError(t, err)
		assert.Equal(t, "/custom/runtime", dir)
	})

	t.Run("falls back to state runtime dir when XDG_RUNTIME_DIR not set", func(t *testing.T) {
		originalRuntime := os.Getenv("XDG_RUNTIME_DIR")
		originalState := os.Getenv("XDG_STATE_HOME")
		defer func() {
			if originalRuntime == "" {
				os.Unsetenv("XDG_RUNTIME_DIR")
			} else {
				os.Setenv("XDG_RUNTIME_DIR", originalRuntime)
			}
			if originalState == "" {
				os.Unsetenv("XDG_STATE_HOME")
			} else {
				os.Setenv("XDG_STATE_HOME", originalState)
			}
		}()

		os.Unsetenv("XDG_RUNTIME_DIR")
		os.Setenv("XDG_STATE_HOME", "/tmp/dumber-state")

		dir, err := adapter.RuntimeDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/tmp/dumber-state", "dumber", "runtime"), dir)
	})
}

func TestNew(t *testing.T) {
	adapter := New()

	assert.NotNil(t, adapter)
}
