package config

import (
	"math"
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
		{
			name: "zero wheel multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollWheelMultiplier = 0
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_wheel_multiplier",
		},
		{
			name: "negative precise multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollPreciseMultiplier = -0.1
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_precise_multiplier",
		},
		{
			name: "zero horizontal multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollHorizontalMultiplier = 0
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_horizontal_multiplier",
		},
		{
			name: "negative vertical multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollVerticalMultiplier = -0.1
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_vertical_multiplier",
		},
		{
			name: "nan horizontal multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollHorizontalMultiplier = math.NaN()
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_horizontal_multiplier",
		},
		{
			name: "infinite vertical multiplier",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollVerticalMultiplier = math.Inf(1)
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_vertical_multiplier",
		},
		{
			name: "zero max delta disables clamp",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollMaxDelta = 0
			},
			wantErr: false,
		},
		{
			name: "negative max delta",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.ScrollMaxDelta = -1
			},
			wantErr:  true,
			wantText: "engine.cef.input.scroll_max_delta",
		},
		{
			name: "zero touchpad navigation min delta",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.TouchpadNavigationMinDelta = 0
			},
			wantErr:  true,
			wantText: "engine.cef.input.touchpad_navigation_min_delta",
		},
		{
			name: "nan touchpad navigation max vertical ratio",
			mutate: func(cfg *Config) {
				cfg.Engine.CEF.Input.TouchpadNavigationMaxVerticalRatio = math.NaN()
			},
			wantErr:  true,
			wantText: "engine.cef.input.touchpad_navigation_max_vertical_ratio",
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
