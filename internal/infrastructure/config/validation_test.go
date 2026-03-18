package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_EngineCookiePolicy(t *testing.T) {
	tests := []struct {
		name         string
		cookiePolicy CookiePolicy
		wantErr      bool
	}{
		{name: "always", cookiePolicy: CookiePolicyAlways, wantErr: false},
		{name: "no_third_party", cookiePolicy: CookiePolicyNoThirdParty, wantErr: false},
		{name: "never", cookiePolicy: CookiePolicyNever, wantErr: false},
		{name: "invalid", cookiePolicy: CookiePolicy("bad_policy"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Engine.CookiePolicy = tt.cookiePolicy

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "engine.cookie_policy")
				return
			}
			require.NoError(t, err)
		})
	}
}
