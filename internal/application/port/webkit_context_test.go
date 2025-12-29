package port_test

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestMemoryPressureConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		config   *port.MemoryPressureConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "empty config with default kill threshold",
			config: &port.MemoryPressureConfig{
				KillThreshold: -1, // explicit unset
			},
			expected: false,
		},
		{
			name: "only MemoryLimitMB set",
			config: &port.MemoryPressureConfig{
				MemoryLimitMB: 1024,
			},
			expected: true,
		},
		{
			name: "only PollIntervalSec set",
			config: &port.MemoryPressureConfig{
				PollIntervalSec: 30.0,
			},
			expected: true,
		},
		{
			name: "only ConservativeThreshold set",
			config: &port.MemoryPressureConfig{
				ConservativeThreshold: 0.33,
			},
			expected: true,
		},
		{
			name: "only StrictThreshold set",
			config: &port.MemoryPressureConfig{
				StrictThreshold: 0.5,
			},
			expected: true,
		},
		{
			name: "KillThreshold set to 0 (never kill)",
			config: &port.MemoryPressureConfig{
				KillThreshold: 0,
			},
			expected: true,
		},
		{
			name: "KillThreshold set to positive value",
			config: &port.MemoryPressureConfig{
				KillThreshold: 1.5,
			},
			expected: true,
		},
		{
			name: "KillThreshold unset (-1)",
			config: &port.MemoryPressureConfig{
				KillThreshold: -1,
			},
			expected: false,
		},
		{
			name: "all defaults (unset)",
			config: &port.MemoryPressureConfig{
				MemoryLimitMB:         0,
				PollIntervalSec:       0,
				ConservativeThreshold: 0,
				StrictThreshold:       0,
				KillThreshold:         -1,
			},
			expected: false,
		},
		{
			name: "fully configured",
			config: &port.MemoryPressureConfig{
				MemoryLimitMB:         2048,
				PollIntervalSec:       15.0,
				ConservativeThreshold: 0.25,
				StrictThreshold:       0.6,
				KillThreshold:         0.9,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsConfigured()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebKitContextOptions_IsWebProcessMemoryConfigured(t *testing.T) {
	tests := []struct {
		name     string
		opts     port.WebKitContextOptions
		expected bool
	}{
		{
			name:     "nil WebProcessMemory",
			opts:     port.WebKitContextOptions{},
			expected: false,
		},
		{
			name: "empty WebProcessMemory with default kill threshold",
			opts: port.WebKitContextOptions{
				WebProcessMemory: &port.MemoryPressureConfig{
					KillThreshold: -1, // explicit unset
				},
			},
			expected: false,
		},
		{
			name: "configured WebProcessMemory",
			opts: port.WebKitContextOptions{
				WebProcessMemory: &port.MemoryPressureConfig{
					MemoryLimitMB: 1024,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.IsWebProcessMemoryConfigured()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWebKitContextOptions_IsNetworkProcessMemoryConfigured(t *testing.T) {
	tests := []struct {
		name     string
		opts     port.WebKitContextOptions
		expected bool
	}{
		{
			name:     "nil NetworkProcessMemory",
			opts:     port.WebKitContextOptions{},
			expected: false,
		},
		{
			name: "empty NetworkProcessMemory with default kill threshold",
			opts: port.WebKitContextOptions{
				NetworkProcessMemory: &port.MemoryPressureConfig{
					KillThreshold: -1, // explicit unset
				},
			},
			expected: false,
		},
		{
			name: "configured NetworkProcessMemory",
			opts: port.WebKitContextOptions{
				NetworkProcessMemory: &port.MemoryPressureConfig{
					PollIntervalSec: 20.0,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.IsNetworkProcessMemoryConfigured()
			assert.Equal(t, tt.expected, result)
		})
	}
}
