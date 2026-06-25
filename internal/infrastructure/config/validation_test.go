package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_EngineType(t *testing.T) {
	t.Setenv("DUMBER_ENGINE", "")

	tests := []struct {
		name       string
		engineType string
		wantErr    bool
	}{
		{name: "cef", engineType: EngineTypeCEF, wantErr: false},
		{name: "webkit", engineType: EngineTypeWebKit, wantErr: false},
		{name: "empty defaults to cef", engineType: "", wantErr: false},
		{name: "invalid", engineType: "netscape", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Engine.Type = tt.engineType

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "engine.type")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateConfig_EngineTypeIgnoresEnvOverride(t *testing.T) {
	t.Run("invalid configured type is still rejected when env override is valid", func(t *testing.T) {
		t.Setenv("DUMBER_ENGINE", EngineTypeWebKit)
		cfg := DefaultConfig()
		cfg.Engine.Type = "netscape"

		err := validateConfig(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "engine.type")
	})

	t.Run("valid configured type is not rejected when env override is invalid", func(t *testing.T) {
		t.Setenv("DUMBER_ENGINE", "netscape")
		cfg := DefaultConfig()
		cfg.Engine.Type = EngineTypeCEF

		err := validateConfig(cfg)

		require.NoError(t, err)
	})
}

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

func TestValidateConfig_WebKitMediaSettings(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Config)
		wantErr   bool
		wantField string
	}{
		{
			name: "valid gl rendering modes",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GLRenderingMode = GLRenderingModeGLES2
			},
			wantErr: false,
		},
		{
			name: "invalid gl rendering mode",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GLRenderingMode = GLRenderingMode("metal")
			},
			wantErr:   true,
			wantField: "engine.webkit.gl_rendering_mode",
		},
		{
			name: "gstreamer debug level lower bound",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GStreamerDebugLevel = 0
			},
			wantErr: false,
		},
		{
			name: "gstreamer debug level upper bound",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GStreamerDebugLevel = 5
			},
			wantErr: false,
		},
		{
			name: "negative gstreamer debug level",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GStreamerDebugLevel = -1
			},
			wantErr:   true,
			wantField: "engine.webkit.gstreamer_debug_level",
		},
		{
			name: "too high gstreamer debug level",
			mutate: func(cfg *Config) {
				cfg.Engine.WebKit.GStreamerDebugLevel = 6
			},
			wantErr:   true,
			wantField: "engine.webkit.gstreamer_debug_level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantField)
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

func TestValidateConfig_WorkspaceNewPaneURLAllowsExistingLocalPathLikeValues(t *testing.T) {
	// This test mutates process CWD; do not add t.Parallel here.
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	childDir := filepath.Join(tmpDir, "child")
	require.NoError(t, os.Mkdir(childDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "page.html"), []byte("ok"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "page.html"), []byte("ok"), 0644))
	require.NoError(t, os.Chdir(childDir))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	for _, value := range []string{filepath.Join(tmpDir, "page.html"), "./page.html", "../page.html"} {
		t.Run(value, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Workspace.NewPaneURL = value

			err := validateConfig(cfg)
			require.NoError(t, err)
		})
	}
}

func TestValidateConfig_WorkspaceNewPaneURLRejectsMissingLocalPathLikeValues(t *testing.T) {
	for _, value := range []string{"/missing/dumber/page.html", "./missing.html", "../missing.html", "~/missing-dumber-page.html"} {
		t.Run(value, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Workspace.NewPaneURL = value

			err := validateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "workspace.new_pane_url local path must exist")
		})
	}
}

func TestValidateConfig_WorkspaceNewPaneURLAllowsExistingBareRelativeFile(t *testing.T) {
	// This test mutates process CWD; do not add t.Parallel here.
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.WriteFile("README", []byte("ok"), 0644))

	cfg := DefaultConfig()
	cfg.Workspace.NewPaneURL = "README"

	require.NoError(t, validateConfig(cfg))
}

func TestValidateConfig_WorkspaceNewPaneURLRejectsMissingBareRelativeValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.NewPaneURL = "missing-local-file"

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace.new_pane_url")
}

func TestValidateConfig_WebKitDefaultProfileIgnoresZeroGPUThreads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Engine.Profile = ProfileDefault
	cfg.Engine.WebKit.SkiaGPUPaintingThreads = 0

	err := validateConfig(cfg)
	require.NoError(t, err)
}
