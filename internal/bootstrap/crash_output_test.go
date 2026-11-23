package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetCrashOutput() {
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
}

func TestEnableCrashOutput_RemovesEmptyFileOnCleanup(t *testing.T) {
	defer resetCrashOutput()
	logDir := t.TempDir()

	cleanup, err := EnableCrashOutput(logDir)
	require.NoError(t, err)

	path := FatalOutputPath(logDir, os.Getpid())
	assert.Equal(t, filepath.Join(logDir, "crashes", fmt.Sprintf("pid_%d.fatal.txt", os.Getpid())), path)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Size())

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestEnableCrashOutput_KeepsNonEmptyFileOnCleanup(t *testing.T) {
	defer resetCrashOutput()
	logDir := t.TempDir()

	cleanup, err := EnableCrashOutput(logDir)
	require.NoError(t, err)

	path := FatalOutputPath(logDir, os.Getpid())
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, filePerm)
	require.NoError(t, err)
	_, err = f.WriteString("fatal error: test\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cleanup()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fatal error: test\n", string(data))
}

func TestEnableCrashOutput_EmptyLogDirIsNoop(t *testing.T) {
	defer resetCrashOutput()

	cleanup, err := EnableCrashOutput("")
	require.NoError(t, err)
	assert.NotPanics(t, cleanup)
}
