package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestIdleRuntimeTimeoutDefaultsToDisabled(t *testing.T) {
	require.Equal(t, 0, DefaultConfig().Engine.CEF.IdleRuntimeTimeoutMs)
}

func TestIdleRuntimeTimeoutFileOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nidle_runtime_timeout_ms = 60000\n"), 0o600))

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.configureAutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	require.Equal(t, 60000, m.viper.GetInt("engine.cef.idle_runtime_timeout_ms"))
}

func TestIdleRuntimeTimeoutEnvMapping(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\n"), 0o600))
	t.Setenv("DUMBER_ENGINE_CEF_IDLE_RUNTIME_TIMEOUT_MS", "45000")

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.configureAutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	require.Equal(t, 45000, m.viper.GetInt("engine.cef.idle_runtime_timeout_ms"))
}

func TestIdleRuntimeTimeoutStableAcrossUnrelatedReload(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nidle_runtime_timeout_ms = 60000\n"), 0o600))

	load := func() int {
		m := &Manager{viper: viper.New()}
		m.viper.SetConfigFile(configPath)
		m.viper.SetConfigType("toml")
		m.configureAutomaticEnv()
		m.setDefaults()
		require.NoError(t, m.viper.ReadInConfig())
		return m.viper.GetInt("engine.cef.idle_runtime_timeout_ms")
	}
	require.Equal(t, 60000, load())

	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nidle_runtime_timeout_ms = 60000\nlog_severity = 3\n"), 0o600))
	require.Equal(t, 60000, load(), "unrelated reload must not change the effective timeout")
}
