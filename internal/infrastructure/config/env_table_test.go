package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newEnvTestManager(t *testing.T, toml string) *Manager {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(toml), 0o600))
	v, envTables := newConfigViper()
	m := &Manager{viper: v, envTables: envTables}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.configureAutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())
	return m
}

func loadEnvTestConfig(t *testing.T, toml string) *Config {
	t.Helper()
	cfg, err := newEnvTestManager(t, toml).unmarshalConfig()
	require.NoError(t, err)
	return cfg
}

func TestDumberEngineEnvDoesNotReplaceEngineTable(t *testing.T) {
	t.Setenv("DUMBER_ENGINE", "webkit")
	cfg := loadEnvTestConfig(t, "[engine]\ntype = \"cef\"\n")
	require.NoError(t, validateConfig(cfg), "engine table must survive DUMBER_ENGINE")
	require.Equal(t, EngineTypeCEF, cfg.Engine.Type, "config value kept")
	require.Equal(t, DefaultConfig().Engine.CEF.Input, cfg.Engine.CEF.Input, "nested defaults kept")
	require.Equal(t, EngineTypeWebKit, cfg.Engine.ResolveEngineType(), "DUMBER_ENGINE still overrides the engine choice")
}

func TestScalarEnvNeverReplacesTables(t *testing.T) {
	t.Setenv("DUMBER_ENGINE_CEF", "x")
	t.Setenv("DUMBER_ENGINE_CEF_INPUT", "x")
	t.Setenv("DUMBER_LOGGING", "x")
	cfg := loadEnvTestConfig(t, "")
	require.NoError(t, validateConfig(cfg))
	require.Equal(t, DefaultConfig().Engine.CEF.Input, cfg.Engine.CEF.Input)
	require.Equal(t, DefaultConfig().Logging.Level, cfg.Logging.Level)
}

func TestLeafEnvOverridesStillApply(t *testing.T) {
	t.Setenv("DUMBER_ENGINE", "cef")
	t.Setenv("DUMBER_ENGINE_TYPE", "webkit")
	t.Setenv("DUMBER_ENGINE_CEF_RENDER_STACK", "egl")
	cfg := loadEnvTestConfig(t, "")
	require.Equal(t, EngineTypeWebKit, cfg.Engine.Type)
	require.Equal(t, "egl", string(cfg.Engine.CEF.RenderStack))
}

func TestTableSafeEnvReplacer(t *testing.T) {
	r := newTableSafeEnvReplacer()
	r.refresh("DUMBER", []string{"engine.type", "engine.cef.input.scroll", "a_b", "a.b.c"})
	cases := map[string]string{
		"DUMBER_ENGINE":                  "",
		"DUMBER_ENGINE_CEF":              "",
		"DUMBER_ENGINE_CEF_INPUT":        "",
		"DUMBER_ENGINE_TYPE":             "DUMBER_ENGINE_TYPE",
		"DUMBER_ENGINE.CEF.INPUT.SCROLL": "DUMBER_ENGINE_CEF_INPUT_SCROLL",
		"DUMBER_A_B":                     "DUMBER_A_B", // also a leaf's env name
		"DUMBER_LOG_LEVEL":               "DUMBER_LOG_LEVEL",
	}
	for in, want := range cases {
		require.Equal(t, want, r.Replace(in), in)
	}
}
