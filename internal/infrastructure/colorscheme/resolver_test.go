package colorscheme

import (
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfigProvider implements ConfigProvider for testing.
type mockConfigProvider struct {
	scheme string
}

func (m *mockConfigProvider) GetColorScheme() string {
	return m.scheme
}

// mockDetector implements port.ColorSchemeDetector for testing.
type mockDetector struct {
	name        string
	priority    int
	available   bool
	prefersDark bool
	detectOk    bool
}

func (m *mockDetector) Name() string         { return m.name }
func (m *mockDetector) Priority() int        { return m.priority }
func (m *mockDetector) Available() bool      { return m.available }
func (m *mockDetector) Detect() (bool, bool) { return m.prefersDark, m.detectOk }

func TestResolver_ConfigOverride(t *testing.T) {
	tests := []struct {
		name        string
		configValue string
		wantDark    bool
		wantSource  string
	}{
		{
			name:        "prefer-dark from config",
			configValue: "prefer-dark",
			wantDark:    true,
			wantSource:  "config",
		},
		{
			name:        "dark from config",
			configValue: "dark",
			wantDark:    true,
			wantSource:  "config",
		},
		{
			name:        "prefer-light from config",
			configValue: "prefer-light",
			wantDark:    false,
			wantSource:  "config",
		},
		{
			name:        "light from config",
			configValue: "light",
			wantDark:    false,
			wantSource:  "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mockConfigProvider{scheme: tt.configValue}
			resolver := NewResolver(config)

			pref := resolver.Resolve()

			assert.Equal(t, tt.wantDark, pref.PrefersDark)
			assert.Equal(t, tt.wantSource, pref.Source)
		})
	}
}

func TestResolver_DefaultFallsThrough(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// Add a detector that returns light
	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	pref := resolver.Resolve()

	assert.False(t, pref.PrefersDark)
	assert.Equal(t, "test", pref.Source)
}

func TestResolver_DetectorPriority(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// Low priority detector returns dark
	lowPriority := &mockDetector{
		name:        "low",
		priority:    10,
		available:   true,
		prefersDark: true,
		detectOk:    true,
	}
	// High priority detector returns light
	highPriority := &mockDetector{
		name:        "high",
		priority:    100,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}

	// Register low first, high second (order shouldn't matter)
	resolver.RegisterDetector(lowPriority)
	resolver.RegisterDetector(highPriority)

	pref := resolver.Resolve()

	// Should use high priority detector
	assert.False(t, pref.PrefersDark)
	assert.Equal(t, "high", pref.Source)
}

func TestResolver_SkipsUnavailableDetector(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// High priority but unavailable
	unavailable := &mockDetector{
		name:        "unavailable",
		priority:    100,
		available:   false,
		prefersDark: false,
		detectOk:    true,
	}
	// Low priority but available
	available := &mockDetector{
		name:        "available",
		priority:    10,
		available:   true,
		prefersDark: true,
		detectOk:    true,
	}

	resolver.RegisterDetector(unavailable)
	resolver.RegisterDetector(available)

	pref := resolver.Resolve()

	// Should use available detector
	assert.True(t, pref.PrefersDark)
	assert.Equal(t, "available", pref.Source)
}

func TestResolver_SkipsFailedDetection(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// High priority but detection fails
	failing := &mockDetector{
		name:        "failing",
		priority:    100,
		available:   true,
		prefersDark: false,
		detectOk:    false, // Detection fails
	}
	// Low priority but succeeds
	succeeding := &mockDetector{
		name:        "succeeding",
		priority:    10,
		available:   true,
		prefersDark: true,
		detectOk:    true,
	}

	resolver.RegisterDetector(failing)
	resolver.RegisterDetector(succeeding)

	pref := resolver.Resolve()

	// Should use succeeding detector
	assert.True(t, pref.PrefersDark)
	assert.Equal(t, "succeeding", pref.Source)
}

func TestResolver_FallbackWhenNoDetectors(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// No detectors registered
	pref := resolver.Resolve()

	// Should fallback to dark
	assert.True(t, pref.PrefersDark)
	assert.Equal(t, "fallback", pref.Source)
}

func TestResolver_FallbackWhenAllFail(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// All detectors fail
	resolver.RegisterDetector(&mockDetector{
		name:      "fail1",
		priority:  100,
		available: true,
		detectOk:  false,
	})
	resolver.RegisterDetector(&mockDetector{
		name:      "fail2",
		priority:  50,
		available: false,
	})

	pref := resolver.Resolve()

	// Should fallback to dark
	assert.True(t, pref.PrefersDark)
	assert.Equal(t, "fallback", pref.Source)
}

func TestResolver_NilConfig(t *testing.T) {
	resolver := NewResolver(nil)

	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	pref := resolver.Resolve()

	// Should use detector since config is nil
	assert.False(t, pref.PrefersDark)
	assert.Equal(t, "test", pref.Source)
}

func TestResolver_Refresh(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// Start with light-preferring detector
	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	pref1 := resolver.Refresh()
	assert.False(t, pref1.PrefersDark)

	// Change detector preference
	detector.prefersDark = true

	pref2 := resolver.Refresh()
	assert.True(t, pref2.PrefersDark)
}

func TestResolver_OnChange(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	var callbackPref port.ColorSchemePreference
	var callbackCount int
	resolver.OnChange(func(pref port.ColorSchemePreference) {
		callbackPref = pref
		callbackCount++
	})

	// Initial refresh - callback should be called (default was dark, now light)
	resolver.Refresh()
	assert.Equal(t, 1, callbackCount)
	assert.False(t, callbackPref.PrefersDark)

	// Same preference - callback should NOT be called
	resolver.Refresh()
	assert.Equal(t, 1, callbackCount)

	// Change preference - callback should be called
	detector.prefersDark = true
	resolver.Refresh()
	assert.Equal(t, 2, callbackCount)
	assert.True(t, callbackPref.PrefersDark)
}

func TestResolver_OnChangeUnregister(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	var callbackCount int
	unregister := resolver.OnChange(func(_ port.ColorSchemePreference) {
		callbackCount++
	})

	// Initial refresh triggers callback
	resolver.Refresh()
	assert.Equal(t, 1, callbackCount)

	// Unregister
	unregister()

	// Change preference - callback should NOT be called
	detector.prefersDark = true
	resolver.Refresh()
	assert.Equal(t, 1, callbackCount) // Still 1, not 2
}

func TestResolver_ConcurrentAccess(_ *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	var wg sync.WaitGroup
	const goroutines = 10

	// Concurrent Resolve calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				resolver.Resolve()
			}
		}()
	}

	// Concurrent Refresh calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				resolver.Refresh()
			}
		}()
	}

	// Concurrent RegisterDetector calls
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resolver.RegisterDetector(&mockDetector{
				name:        "concurrent",
				priority:    id,
				available:   true,
				prefersDark: id%2 == 0,
				detectOk:    true,
			})
		}(i)
	}

	wg.Wait()
	// Test passes if no race conditions detected
}

func TestResolver_EmptyConfigScheme(t *testing.T) {
	config := &mockConfigProvider{scheme: ""}
	resolver := NewResolver(config)

	detector := &mockDetector{
		name:        "test",
		priority:    50,
		available:   true,
		prefersDark: false,
		detectOk:    true,
	}
	resolver.RegisterDetector(detector)

	pref := resolver.Resolve()

	// Empty config should fall through to detector
	assert.False(t, pref.PrefersDark)
	assert.Equal(t, "test", pref.Source)
}

func TestResolver_ImplementsInterface(t *testing.T) {
	config := &mockConfigProvider{scheme: "default"}
	resolver := NewResolver(config)

	// Verify Resolver implements port.ColorSchemeResolver
	var _ port.ColorSchemeResolver = resolver
	require.NotNil(t, resolver)
}
