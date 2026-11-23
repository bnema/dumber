package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

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

const crashHelperDirEnv = "DUMBER_CRASH_HELPER_DIR"

// TestCrashOutputHelper runs only as a child of TestCrashOutputSubprocess.
// It enables crash output in its working directory (the parent's temp dir)
// and dies from an unrecovered goroutine panic.
func TestCrashOutputHelper(t *testing.T) {
	if os.Getenv(crashHelperDirEnv) == "" {
		return
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if _, err = EnableCrashOutput(dir); err != nil {
		t.Fatalf("enable crash output: %v", err)
	}
	go func() { panic("crash output helper panic") }()
	select {}
}

func TestCrashOutputSubprocess(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashOutputHelper$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), crashHelperDirEnv+"="+dir, "GOTRACEBACK=single")

	err := cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "child must exit non-zero")

	data, err := os.ReadFile(FatalOutputPath(dir, cmd.Process.Pid))
	require.NoError(t, err)
	assert.Contains(t, string(data), "panic:")
	assert.Contains(t, string(data), "goroutine ")
}

func TestEnableCrashOutput_RotatesStaleFileForReusedPID(t *testing.T) {
	defer resetCrashOutput()
	logDir := t.TempDir()
	path := FatalOutputPath(logDir, os.Getpid())
	require.NoError(t, os.MkdirAll(filepath.Dir(path), dirPerm))
	require.NoError(t, os.WriteFile(path, []byte("panic: old\n"), filePerm))
	mtime := time.Unix(1700000000, 0)
	require.NoError(t, os.Chtimes(path, mtime, mtime))

	cleanup, err := EnableCrashOutput(logDir)
	require.NoError(t, err)
	defer cleanup()

	data, err := os.ReadFile(rotatedFatalOutputPath(logDir, os.Getpid(), mtime.Unix()))
	require.NoError(t, err)
	assert.Equal(t, "panic: old\n", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Size())
}
