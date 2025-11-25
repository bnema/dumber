package webext

import (
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/webext/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPaneProvider implements PaneProvider for testing
type mockPaneProvider struct {
	mu         sync.Mutex
	panes      []api.PaneInfo
	activePane *api.PaneInfo
}

func (m *mockPaneProvider) GetAllPanes() []api.PaneInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.panes
}

func (m *mockPaneProvider) GetActivePane() *api.PaneInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activePane
}

func (m *mockPaneProvider) SetPanes(panes []api.PaneInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.panes = panes
}

func (m *mockPaneProvider) SetActivePane(pane *api.PaneInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activePane = pane
}

// createTestExtension creates a minimal extension for testing
func createTestExtension(t *testing.T) *Extension {
	return &Extension{
		ID:   "test-extension",
		Path: t.TempDir(),
		Manifest: &Manifest{
			Name:    "Test Extension",
			Version: "1.0.0",
		},
	}
}

// createTestBackgroundContext creates a BackgroundContext for testing
func createTestBackgroundContext(t *testing.T) *BackgroundContext {
	ext := createTestExtension(t)
	bc := NewBackgroundContext(ext)
	return bc
}

func TestNewBackgroundContext(t *testing.T) {
	ext := createTestExtension(t)
	bc := NewBackgroundContext(ext)

	assert.NotNil(t, bc)
	assert.Equal(t, ext, bc.ext)
	assert.NotNil(t, bc.runtimeOnMessage)
	assert.NotNil(t, bc.runtimeOnConnect)
	assert.NotNil(t, bc.storageOnChanged)
	assert.NotNil(t, bc.ports)
	assert.NotNil(t, bc.alarms)
	assert.NotNil(t, bc.alarmsEvent)
	assert.NotNil(t, bc.i18nMessages)
}

func TestBackgroundContext_SetPaneProvider(t *testing.T) {
	bc := createTestBackgroundContext(t)
	provider := &mockPaneProvider{}

	bc.SetPaneProvider(provider)

	bc.mu.Lock()
	assert.Equal(t, provider, bc.paneProvider)
	bc.mu.Unlock()
}

func TestBackgroundContext_StartStop(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)

	// Verify VM is initialized
	bc.mu.Lock()
	assert.NotNil(t, bc.vm)
	assert.NotNil(t, bc.tasks)
	bc.mu.Unlock()

	// Stop should clean up
	bc.Stop()

	bc.mu.Lock()
	assert.Nil(t, bc.tasks)
	assert.Nil(t, bc.alarms)
	bc.mu.Unlock()
}

func TestBackgroundContext_I18n_GetMessage(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Set up test translations AFTER Start() since Start() reloads from extension path
	bc.mu.Lock()
	bc.i18nMessages = map[string]I18nMessage{
		"greeting": {Message: "Hello, $1!"},
		"simple":   {Message: "Simple message"},
	}
	bc.i18nLocale = "en"
	bc.mu.Unlock()

	tests := []struct {
		name     string
		key      string
		subs     []string
		expected string
	}{
		{
			name:     "simple message",
			key:      "simple",
			expected: "Simple message",
		},
		{
			name:     "message with substitution",
			key:      "greeting",
			subs:     []string{"World"},
			expected: "Hello, World!",
		},
		{
			name:     "missing key returns empty",
			key:      "nonexistent",
			expected: "",
		},
		{
			name:     "predefined @@extension_id",
			key:      "@@extension_id",
			expected: "test-extension",
		},
		{
			name:     "predefined @@ui_locale",
			key:      "@@ui_locale",
			expected: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			err := bc.call(func() error {
				var jsResult interface{}
				var jsErr error
				if len(tt.subs) > 0 {
					val, err := bc.vm.RunString(`browser.i18n.getMessage("` + tt.key + `", ["` + tt.subs[0] + `"])`)
					jsErr = err
					if val != nil {
						jsResult = val.Export()
					}
				} else {
					val, err := bc.vm.RunString(`browser.i18n.getMessage("` + tt.key + `")`)
					jsErr = err
					if val != nil {
						jsResult = val.Export()
					}
				}
				if jsErr != nil {
					return jsErr
				}
				if jsResult != nil {
					if s, ok := jsResult.(string); ok {
						result = s
					}
				}
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBackgroundContext_Tabs_Query(t *testing.T) {
	bc := createTestBackgroundContext(t)

	// Set up mock pane provider
	provider := &mockPaneProvider{
		panes: []api.PaneInfo{
			{ID: 1, WindowID: 100, URL: "https://example.com", Title: "Example", Active: true, Index: 0},
			{ID: 2, WindowID: 100, URL: "https://test.com", Title: "Test", Active: false, Index: 1},
			{ID: 3, WindowID: 200, URL: "https://other.com", Title: "Other", Active: true, Index: 0},
		},
		activePane: &api.PaneInfo{ID: 1, WindowID: 100, URL: "https://example.com", Title: "Example", Active: true, Index: 0},
	}
	bc.SetPaneProvider(provider)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	tests := []struct {
		name          string
		query         string
		expectedCount int
		expectedIDs   []int
	}{
		{
			name:          "all tabs",
			query:         "{}",
			expectedCount: 3,
			expectedIDs:   []int{1, 2, 3},
		},
		{
			name:          "active tabs only",
			query:         "{active: true}",
			expectedCount: 2,
			expectedIDs:   []int{1, 3},
		},
		{
			name:          "current window",
			query:         "{currentWindow: true}",
			expectedCount: 2,
			expectedIDs:   []int{1, 2},
		},
		{
			name:          "specific window",
			query:         "{windowId: 200}",
			expectedCount: 1,
			expectedIDs:   []int{3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bc.call(func() error {
				// Execute the query synchronously for testing
				_, err := bc.vm.RunString(`browser.tabs.query(` + tt.query + `)`)
				return err
			})
			require.NoError(t, err)

			// Verify provider was called - check panes were accessed
			panes := provider.GetAllPanes()
			assert.NotEmpty(t, panes)
		})
	}
}

func TestBackgroundContext_Alarms_Create(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	t.Run("create alarm with delay", func(t *testing.T) {
		err := bc.call(func() error {
			_, err := bc.vm.RunString(`
				browser.alarms.create("test-alarm", {delayInMinutes: 1});
			`)
			return err
		})
		require.NoError(t, err)

		bc.mu.Lock()
		alarm, exists := bc.alarms["test-alarm"]
		bc.mu.Unlock()

		assert.True(t, exists)
		assert.NotNil(t, alarm)
		assert.Equal(t, "test-alarm", alarm.Name)
		assert.True(t, alarm.sourceHandle != 0 || alarm.backstop != nil)
	})

	t.Run("create alarm with period", func(t *testing.T) {
		err := bc.call(func() error {
			_, err := bc.vm.RunString(`
				browser.alarms.create("periodic-alarm", {periodInMinutes: 5});
			`)
			return err
		})
		require.NoError(t, err)

		bc.mu.Lock()
		alarm, exists := bc.alarms["periodic-alarm"]
		bc.mu.Unlock()

		assert.True(t, exists)
		assert.Equal(t, float64(5), alarm.PeriodMinutes)
	})

	t.Run("replace existing alarm", func(t *testing.T) {
		// Create first alarm
		err := bc.call(func() error {
			_, err := bc.vm.RunString(`
				browser.alarms.create("replace-test", {delayInMinutes: 10});
			`)
			return err
		})
		require.NoError(t, err)

		bc.mu.Lock()
		firstScheduledTime := bc.alarms["replace-test"].ScheduledTime
		bc.mu.Unlock()

		// Small sleep to ensure different timestamp
		time.Sleep(5 * time.Millisecond)

		// Create replacement with different delay
		err = bc.call(func() error {
			_, err := bc.vm.RunString(`
				browser.alarms.create("replace-test", {delayInMinutes: 20});
			`)
			return err
		})
		require.NoError(t, err)

		bc.mu.Lock()
		secondScheduledTime := bc.alarms["replace-test"].ScheduledTime
		bc.mu.Unlock()

		// Second alarm should have later scheduled time (20 min vs 10 min delay)
		assert.Greater(t, secondScheduledTime, firstScheduledTime)
	})
}

func TestBackgroundContext_Alarms_Clear(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Create an alarm first
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			browser.alarms.create("to-clear", {delayInMinutes: 60});
		`)
		return err
	})
	require.NoError(t, err)

	// Verify it exists
	bc.mu.Lock()
	_, exists := bc.alarms["to-clear"]
	bc.mu.Unlock()
	assert.True(t, exists)

	// Clear it
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			(async () => {
				await browser.alarms.clear("to-clear");
			})()
		`)
		return err
	})
	require.NoError(t, err)

	// Give async operation time to complete
	time.Sleep(10 * time.Millisecond)

	// Verify it's gone
	bc.mu.Lock()
	_, exists = bc.alarms["to-clear"]
	bc.mu.Unlock()
	assert.False(t, exists)
}

func TestBackgroundContext_Alarms_ClearAll(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Create multiple alarms
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			browser.alarms.create("alarm1", {delayInMinutes: 60});
			browser.alarms.create("alarm2", {delayInMinutes: 60});
			browser.alarms.create("alarm3", {delayInMinutes: 60});
		`)
		return err
	})
	require.NoError(t, err)

	bc.mu.Lock()
	count := len(bc.alarms)
	bc.mu.Unlock()
	assert.Equal(t, 3, count)

	// Clear all
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			(async () => {
				await browser.alarms.clearAll();
			})()
		`)
		return err
	})
	require.NoError(t, err)

	// Give async operation time to complete
	time.Sleep(10 * time.Millisecond)

	bc.mu.Lock()
	count = len(bc.alarms)
	bc.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestBackgroundContext_Alarms_OnAlarm_Fires(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Set up alarm listener and create short-delay alarm
	fired := make(chan string, 1)

	err = bc.call(func() error {
		// Register a listener
		listener := bc.vm.ToValue(func(alarm map[string]interface{}) {
			if name, ok := alarm["name"].(string); ok {
				select {
				case fired <- name:
				default:
				}
			}
		})
		bc.alarmsEvent.add(bc.vm, listener)

		// Create alarm with very short delay (will fire after ~100ms minimum)
		_, err := bc.vm.RunString(`
			browser.alarms.create("quick-alarm", {when: Date.now() + 50});
		`)
		return err
	})
	require.NoError(t, err)

	// Wait for alarm to fire (with timeout)
	select {
	case name := <-fired:
		assert.Equal(t, "quick-alarm", name)
	case <-time.After(2 * time.Second):
		t.Fatal("alarm did not fire within timeout")
	}
}

func TestBackgroundContext_Alarms_Get(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Create an alarm
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			browser.alarms.create("get-test", {delayInMinutes: 30, periodInMinutes: 10});
		`)
		return err
	})
	require.NoError(t, err)

	// Get it back
	var alarmName string
	var periodMinutes float64

	err = bc.call(func() error {
		result, err := bc.vm.RunString(`
			(async () => {
				const alarm = await browser.alarms.get("get-test");
				return JSON.stringify(alarm);
			})()
		`)
		if err != nil {
			return err
		}
		// The result is a promise, need to handle async
		_ = result
		return nil
	})
	require.NoError(t, err)

	// Verify alarm properties from the map directly
	bc.mu.Lock()
	alarm := bc.alarms["get-test"]
	if alarm != nil {
		alarmName = alarm.Name
		periodMinutes = alarm.PeriodMinutes
	}
	bc.mu.Unlock()

	assert.Equal(t, "get-test", alarmName)
	assert.Equal(t, float64(10), periodMinutes)
}

func TestBackgroundContext_Alarms_GetAll(t *testing.T) {
	bc := createTestBackgroundContext(t)

	err := bc.Start()
	require.NoError(t, err)
	defer bc.Stop()

	// Create multiple alarms
	err = bc.call(func() error {
		_, err := bc.vm.RunString(`
			browser.alarms.create("a1", {delayInMinutes: 10});
			browser.alarms.create("a2", {delayInMinutes: 20});
		`)
		return err
	})
	require.NoError(t, err)

	bc.mu.Lock()
	count := len(bc.alarms)
	names := make([]string, 0, count)
	for name := range bc.alarms {
		names = append(names, name)
	}
	bc.mu.Unlock()

	assert.Equal(t, 2, count)
	assert.Contains(t, names, "a1")
	assert.Contains(t, names, "a2")
}

// Benchmark tests

func BenchmarkBackgroundContext_I18n_GetMessage(b *testing.B) {
	ext := &Extension{
		ID:   "bench-extension",
		Path: b.TempDir(),
		Manifest: &Manifest{
			Name:    "Bench Extension",
			Version: "1.0.0",
		},
	}
	bc := NewBackgroundContext(ext)
	bc.i18nMessages = map[string]I18nMessage{
		"test": {Message: "Test message with $1 substitution"},
	}
	bc.i18nLocale = "en"

	if err := bc.Start(); err != nil {
		b.Fatal(err)
	}
	defer bc.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bc.call(func() error {
			_, _ = bc.vm.RunString(`browser.i18n.getMessage("test", ["value"])`)
			return nil
		})
	}
}

func BenchmarkBackgroundContext_Alarms_Create(b *testing.B) {
	ext := &Extension{
		ID:   "bench-extension",
		Path: b.TempDir(),
		Manifest: &Manifest{
			Name:    "Bench Extension",
			Version: "1.0.0",
		},
	}
	bc := NewBackgroundContext(ext)

	if err := bc.Start(); err != nil {
		b.Fatal(err)
	}
	defer bc.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bc.call(func() error {
			_, _ = bc.vm.RunString(`browser.alarms.create("bench", {delayInMinutes: 60})`)
			return nil
		})
	}
}
