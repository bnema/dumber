package xdg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_DownloadDir(t *testing.T) {
	adapter := New(runtimeprofile.Profile{})

	t.Run("returns XDG_DOWNLOAD_DIR when set", func(t *testing.T) {
		t.Setenv("XDG_DOWNLOAD_DIR", "/custom/downloads")

		dir, err := adapter.DownloadDir()

		require.NoError(t, err)
		assert.Equal(t, "/custom/downloads", dir)
	})

	t.Run("falls back to ~/Downloads when XDG_DOWNLOAD_DIR not set", func(t *testing.T) {
		t.Setenv("XDG_DOWNLOAD_DIR", "")

		dir, err := adapter.DownloadDir()

		require.NoError(t, err)

		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expected := filepath.Join(home, "Downloads")
		assert.Equal(t, expected, dir)
	})
}

func TestAdapter_RuntimeDir(t *testing.T) {
	t.Run("returns XDG_RUNTIME_DIR when set outside dev", func(t *testing.T) {
		adapter := New(runtimeprofile.Profile{Mode: runtimeprofile.ModeProd})
		t.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")

		dir, err := adapter.RuntimeDir()

		require.NoError(t, err)
		assert.Equal(t, "/custom/runtime", dir)
	})

	t.Run("falls back to injected prod runtime dir when XDG_RUNTIME_DIR not set", func(t *testing.T) {
		adapter := New(runtimeprofile.Profile{
			Mode: runtimeprofile.ModeProd,
			Shared: runtimeprofile.SharedPaths{
				StateDir: "/tmp/dumber-state/dumber",
			},
		})
		t.Setenv("XDG_RUNTIME_DIR", "")

		dir, err := adapter.RuntimeDir()

		require.NoError(t, err)
		assert.Equal(t, "/tmp/dumber-state/dumber/runtime", dir)
	})

	t.Run("uses injected sandbox runtime dir in dev even when XDG_RUNTIME_DIR is set", func(t *testing.T) {
		wd := t.TempDir()
		adapter := New(runtimeprofile.Profile{
			Mode: runtimeprofile.ModeDev,
			Shared: runtimeprofile.SharedPaths{
				RootDir: filepath.Join(wd, ".dev", "dumber"),
			},
		})
		t.Setenv("XDG_RUNTIME_DIR", "/shared/runtime")

		dir, err := adapter.RuntimeDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(wd, ".dev", "dumber", "runtime"), dir)
	})
}

func TestNew(t *testing.T) {
	adapter := New(runtimeprofile.Profile{})

	assert.NotNil(t, adapter)
}
