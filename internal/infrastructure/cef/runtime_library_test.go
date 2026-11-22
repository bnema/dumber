package cef

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

func TestRuntimeLibraryReplaced(t *testing.T) {
	writeLib := func(t *testing.T, path, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	t.Run("unchanged file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "libcef.so")
		writeLib(t, path, "v154")
		lib := newRuntimeLibrary(path)
		require.NotNil(t, lib)
		assert.False(t, lib.replaced())
	})

	t.Run("file replaced by rename like a package update", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "libcef.so")
		writeLib(t, path, "v154")
		lib := newRuntimeLibrary(path)
		require.NotNil(t, lib)

		next := filepath.Join(dir, "libcef.so.new")
		writeLib(t, next, "v156")
		require.NoError(t, os.Rename(next, path))
		assert.True(t, lib.replaced())
	})

	t.Run("file removed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "libcef.so")
		writeLib(t, path, "v154")
		lib := newRuntimeLibrary(path)
		require.NotNil(t, lib)

		require.NoError(t, os.Remove(path))
		assert.True(t, lib.replaced())
	})

	t.Run("unknown path disables the check", func(t *testing.T) {
		assert.Nil(t, newRuntimeLibrary(""))
		assert.Nil(t, newRuntimeLibrary(filepath.Join(t.TempDir(), "missing.so")))
		var lib *runtimeLibrary
		assert.False(t, lib.replaced())
	})
}

func TestTerminationReason(t *testing.T) {
	tests := []struct {
		status purecef.TerminationStatus
		reason port.WebProcessTerminationReason
		label  string
	}{
		{purecef.TerminationStatusTsAbnormalTermination, port.WebProcessTerminationCrashed, "abnormal termination"},
		{purecef.TerminationStatusTsProcessWasKilled, port.WebProcessTerminationByAPI, "killed"},
		{purecef.TerminationStatusTsProcessCrashed, port.WebProcessTerminationCrashed, "crashed"},
		{purecef.TerminationStatusTsProcessOom, port.WebProcessTerminationExceededMemory, "out of memory"},
		{purecef.TerminationStatus(99), port.WebProcessTerminationUnknown, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			reason, label := terminationReason(tt.status)
			assert.Equal(t, tt.reason, reason)
			assert.Equal(t, tt.label, label)
		})
	}
}

// A replaced runtime kills every new renderer, so it must keep reporting past
// the crash cap that otherwise silences repeated renderer deaths.
func TestOnRenderProcessTerminated_RuntimeReplacedBypassesCrashCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libcef.so")
	require.NoError(t, os.WriteFile(path, []byte("v154"), 0o600))
	lib := newRuntimeLibrary(path)
	require.NotNil(t, lib)

	var reasons []port.WebProcessTerminationReason
	wv := &WebView{
		ctx:         context.Background(),
		engine:      &Engine{runtimeLib: lib},
		gtkDispatch: func(fn func()) { fn() },
		callbacks: &port.WebViewCallbacks{
			OnWebProcessTerminated: func(reason port.WebProcessTerminationReason, _ string, _ string) {
				reasons = append(reasons, reason)
			},
		},
	}
	h := &handlerSet{wv: wv}

	for range maxConsecutiveCrashes + 1 {
		h.OnRenderProcessTerminated(nil, purecef.TerminationStatusTsProcessWasKilled, 0, "")
	}
	require.Len(t, reasons, maxConsecutiveCrashes, "crash cap applies while the runtime is unchanged")

	next := filepath.Join(dir, "libcef.so.new")
	require.NoError(t, os.WriteFile(next, []byte("v156"), 0o600))
	require.NoError(t, os.Rename(next, path))

	h.OnRenderProcessTerminated(nil, purecef.TerminationStatusTsProcessWasKilled, 0, "")
	require.Len(t, reasons, maxConsecutiveCrashes+1)
	assert.Equal(t, port.WebProcessTerminationRuntimeReplaced, reasons[len(reasons)-1])
}
