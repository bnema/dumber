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

func TestValidateConfig_CEFConfig(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Config)
		wantErr  bool
		wantText string
	}{
		{
			name: "valid defaults",
			mutate: func(_ *Config) {
			},
			wantErr: false,
		},
		{
			name: "invalid log severity",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.LogSeverity = 7
			},
			wantErr:  true,
			wantText: "engine.cef.log_severity",
		},
		{
			name: "zero frame rate uses default",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.WindowlessFrameRate = 0
			},
			wantErr: false,
		},
		{
			name: "negative frame rate",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.WindowlessFrameRate = -1
			},
			wantErr:  true,
			wantText: "engine.cef.windowless_frame_rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantText)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateConfig_WebKitDefaultProfileIgnoresZeroGPUThreads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Engine.Profile = ProfileDefault
	cfg.Engine.WebKit.SkiaGPUPaintingThreads = 0

	err := validateConfig(cfg)
	require.NoError(t, err)
}
