package cef

import (
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/require"
)

func TestPrepareCEFInitTraceFile_DisabledByDefault(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "")

	path, err := prepareCEFInitTraceFile(config.CEFEngineConfig{})
	require.NoError(t, err)
	require.Empty(t, path)
}

func TestPrepareCEFInitTraceFile_EnabledViaEnv(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "1")
	cfg := config.CEFEngineConfig{
		LogFile: filepath.Join(t.TempDir(), "cef_runtime.log"),
	}

	path, err := prepareCEFInitTraceFile(cfg)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(cfg.LogFile), "cef_runtime.bootstrap.log"), path)
	require.FileExists(t, path)
}
