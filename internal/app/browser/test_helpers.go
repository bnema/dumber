package browser

// Mock WindowShortcutHandler for testing
type mockWindowShortcutHandler struct{}

func (m *mockWindowShortcutHandler) Cleanup() {
	// No-op for testing
}
