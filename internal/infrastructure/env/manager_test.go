package env_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_ApplyEnvironment_SkiaEnv(t *testing.T) {
	// Save original env vars and restore after test
	origCPU := os.Getenv("WEBKIT_SKIA_CPU_PAINTING_THREADS")
	origGPU := os.Getenv("WEBKIT_SKIA_GPU_PAINTING_THREADS")
	origCPURender := os.Getenv("WEBKIT_SKIA_ENABLE_CPU_RENDERING")
	t.Cleanup(func() {
		restoreEnv("WEBKIT_SKIA_CPU_PAINTING_THREADS", origCPU)
		restoreEnv("WEBKIT_SKIA_GPU_PAINTING_THREADS", origGPU)
		restoreEnv("WEBKIT_SKIA_ENABLE_CPU_RENDERING", origCPURender)
	})

	tests := []struct {
		name           string
		settings       port.RenderingEnvSettings
		presetEnv      map[string]string
		expectedVars   map[string]string
		unexpectedVars []string
	}{
		{
			name: "CPU threads set when > 0",
			settings: port.RenderingEnvSettings{
				SkiaCPUPaintingThreads: 4,
				SkiaGPUPaintingThreads: -1, // unset
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_CPU_PAINTING_THREADS": "4",
			},
			unexpectedVars: []string{"WEBKIT_SKIA_GPU_PAINTING_THREADS"},
		},
		{
			name: "GPU threads set when >= 0",
			settings: port.RenderingEnvSettings{
				SkiaCPUPaintingThreads: 0, // unset
				SkiaGPUPaintingThreads: 2,
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_GPU_PAINTING_THREADS": "2",
			},
			unexpectedVars: []string{"WEBKIT_SKIA_CPU_PAINTING_THREADS"},
		},
		{
			name: "GPU threads 0 disables GPU tile painting",
			settings: port.RenderingEnvSettings{
				SkiaGPUPaintingThreads: 0,
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_GPU_PAINTING_THREADS": "0",
			},
		},
		{
			name: "CPU rendering enabled",
			settings: port.RenderingEnvSettings{
				SkiaEnableCPURendering: true,
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_ENABLE_CPU_RENDERING": "1",
			},
		},
		{
			name: "CPU rendering disabled (default)",
			settings: port.RenderingEnvSettings{
				SkiaEnableCPURendering: false,
			},
			unexpectedVars: []string{"WEBKIT_SKIA_ENABLE_CPU_RENDERING"},
		},
		{
			name: "does not override existing CPU threads env var",
			settings: port.RenderingEnvSettings{
				SkiaCPUPaintingThreads: 8,
			},
			presetEnv: map[string]string{
				"WEBKIT_SKIA_CPU_PAINTING_THREADS": "2",
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_CPU_PAINTING_THREADS": "2", // keeps original
			},
		},
		{
			name: "does not override existing GPU threads env var",
			settings: port.RenderingEnvSettings{
				SkiaGPUPaintingThreads: 4,
			},
			presetEnv: map[string]string{
				"WEBKIT_SKIA_GPU_PAINTING_THREADS": "1",
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_GPU_PAINTING_THREADS": "1", // keeps original
			},
		},
		{
			name: "does not override existing CPU rendering env var",
			settings: port.RenderingEnvSettings{
				SkiaEnableCPURendering: true,
			},
			presetEnv: map[string]string{
				"WEBKIT_SKIA_ENABLE_CPU_RENDERING": "0",
			},
			expectedVars: map[string]string{
				"WEBKIT_SKIA_ENABLE_CPU_RENDERING": "0", // keeps original
			},
		},
		{
			name: "all unset by default",
			settings: port.RenderingEnvSettings{
				SkiaCPUPaintingThreads: 0,  // unset
				SkiaGPUPaintingThreads: -1, // unset
				SkiaEnableCPURendering: false,
			},
			unexpectedVars: []string{
				"WEBKIT_SKIA_CPU_PAINTING_THREADS",
				"WEBKIT_SKIA_GPU_PAINTING_THREADS",
				"WEBKIT_SKIA_ENABLE_CPU_RENDERING",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars before each test
			os.Unsetenv("WEBKIT_SKIA_CPU_PAINTING_THREADS")
			os.Unsetenv("WEBKIT_SKIA_GPU_PAINTING_THREADS")
			os.Unsetenv("WEBKIT_SKIA_ENABLE_CPU_RENDERING")

			// Set any preset env vars
			for k, v := range tt.presetEnv {
				os.Setenv(k, v)
			}

			manager := env.NewManager()
			ctx := context.Background()

			err := manager.ApplyEnvironment(ctx, tt.settings)
			require.NoError(t, err)

			// Check expected vars are set correctly
			for k, v := range tt.expectedVars {
				actual := os.Getenv(k)
				assert.Equal(t, v, actual, "env var %s", k)
			}

			// Check unexpected vars are not set (or were not set by us)
			appliedVars := manager.GetAppliedVars()
			for _, k := range tt.unexpectedVars {
				_, wasApplied := appliedVars[k]
				if tt.presetEnv[k] == "" {
					assert.False(t, wasApplied, "env var %s should not be applied", k)
				}
			}
		})
	}
}

func restoreEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}

func TestIsFlatpak(t *testing.T) {
	// IsFlatpak checks for /.flatpak-info file existence
	// In normal test environment, this should return false
	result := env.IsFlatpak()

	// We can't easily mock filesystem, so just verify it returns a boolean
	// and doesn't panic. In CI/dev environment it should be false.
	assert.False(t, result, "should not detect Flatpak in test environment")
}

func TestIsPacman(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		expected bool
	}{
		{
			name:     "system binary owned by pacman",
			binary:   "/usr/bin/bash",
			expected: true, // bash is always installed via pacman on Arch
		},
		{
			name:     "non-existent binary",
			binary:   "/nonexistent/path/to/binary",
			expected: false,
		},
	}

	// Only run these tests on Arch-based systems with pacman
	if _, err := os.Stat("/usr/bin/pacman"); os.IsNotExist(err) {
		t.Skip("pacman not found, skipping Arch-specific tests")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test IsPacman directly since it uses os.Executable()
			// but we can test the underlying logic with exec.Command
			if tt.binary == "/usr/bin/bash" {
				// Verify pacman -Qo works for a known system binary
				cmd := exec.Command("pacman", "-Qo", tt.binary)
				err := cmd.Run()
				if tt.expected {
					assert.NoError(t, err, "pacman should own %s", tt.binary)
				} else {
					assert.Error(t, err, "pacman should not own %s", tt.binary)
				}
			}
		})
	}
}

func TestIsPacman_NonArchSystem(t *testing.T) {
	// On non-Arch systems, IsPacman should return false (pacman not found)
	if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		t.Skip("pacman found, skipping non-Arch test")
	}

	result := env.IsPacman()
	assert.False(t, result, "should return false when pacman is not installed")
}
