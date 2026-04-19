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

func TestNew(t *testing.T) {
	adapter := New()

	assert.NotNil(t, adapter)
}
