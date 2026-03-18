package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCheckLegacyFormat_OldSectionsNoEngine_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.mode", "gpu")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_OldSectionsWithEngine_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.mode", "gpu")
	m.viper.Set("engine.type", "webkit")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_FreshConfig_ReturnsNoError(t *testing.T) {
	m := &Manager{viper: viper.New()}

	err := m.checkLegacyFormat()

	require.NoError(t, err)
}

func TestCheckLegacyFormat_DisableDmabuf_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.disable_dmabuf_renderer", true)

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_PerformanceProfile_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("performance.profile", "lite")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_PrivacyCookiePolicy_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("privacy.cookie_policy", "always")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_RuntimePrefix_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("runtime.prefix", "/usr")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}
