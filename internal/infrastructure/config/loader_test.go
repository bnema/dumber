package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestSetPrivacyDefaults(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	assert.Equal(t, "no_third_party", mgr.viper.GetString("privacy.cookie_policy"))
	assert.True(t, mgr.viper.GetBool("privacy.itp_enabled"))
}

func TestNormalizeConfig_PrivacyCookiePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Privacy.CookiePolicy = CookiePolicy("INVALID")

	normalizeConfig(cfg)

	assert.Equal(t, CookiePolicyNoThirdParty, cfg.Privacy.CookiePolicy)
}
