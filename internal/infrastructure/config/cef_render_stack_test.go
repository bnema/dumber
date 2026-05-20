package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCEFRenderStackDefaultIsVulkan(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, CEFRenderStackVulkan, cfg.Engine.CEF.CEFRenderStack())
}

func TestValidateConfig_CEFRenderStack(t *testing.T) {
	tests := []struct {
		name    string
		stack   CEFRenderStack
		wantErr bool
	}{
		{name: "vulkan", stack: CEFRenderStackVulkan},
		{name: "egl", stack: CEFRenderStackEGL},
		{name: "empty uses default", stack: ""},
		{name: "invalid", stack: CEFRenderStack("auto"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Engine.CEF.RenderStack = tt.stack

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "engine.cef.render_stack")
				return
			}
			require.NoError(t, err)
		})
	}
}
