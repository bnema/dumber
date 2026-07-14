package cef

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumeNextStartSafetyMarkerDisablesVAAPIWithoutChangingVulkanStack(t *testing.T) {
	root := t.TempDir()
	t.Setenv(cefEnableVAAPIEnvVar, "1")
	t.Setenv(cefChromiumFlagsEnvVar, "--ignore-gpu-blocklist --disable-gpu-driver-bug-workaround")
	t.Setenv(dumberRenderStackEnvVar, renderStackVulkanDMABUF)

	require.NoError(t, writeNextStartSafetyMarker(root))
	require.True(t, consumeNextStartSafetyMarker(root))
	applyNextStartSafetyEnvironment()

	require.Equal(t, "0", os.Getenv(cefEnableVAAPIEnvVar))
	require.Empty(t, os.Getenv(cefChromiumFlagsEnvVar))
	require.Equal(t, renderStackVulkanDMABUF, os.Getenv(dumberRenderStackEnvVar))
	_, err := os.Stat(filepath.Join(root, nextStartSafetyMarkerFile))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestGPUProcessRelaunchMarksNextStartSafeAfterRepeatedRelaunch(t *testing.T) {
	root := t.TempDir()
	engine := &Engine{stateRoot: root}
	handler := &dumberBPH{engine: engine}
	commandLine := relaunchCommandLineStub{commandLineString: "dumber --type=gpu-process"}

	require.Zero(t, handler.OnAlreadyRunningAppRelaunch(commandLine, ""))
	require.NoFileExists(t, filepath.Join(root, nextStartSafetyMarkerFile))
	require.Zero(t, handler.OnAlreadyRunningAppRelaunch(commandLine, ""))
	require.FileExists(t, filepath.Join(root, nextStartSafetyMarkerFile))
}
