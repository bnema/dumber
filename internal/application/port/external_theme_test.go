package port

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

// compileCheckExternalThemeSource ensures the ExternalThemeSource interface
// is defined and can be implemented by test mocks.
func TestCompileCheckExternalThemeSource(_ *testing.T) {
	var _ ExternalThemeSource = (*mockExternalThemeSource)(nil)
	var _ ConfigurableExternalThemeSource = (*mockExternalThemeSource)(nil)
	var _ ExternalThemeWatcher = (*mockExternalThemeWatcher)(nil)
}

// mockExternalThemeSource is a test-only implementation of ExternalThemeSource.
type mockExternalThemeSource struct {
	theme    *entity.ExternalTheme
	err      error
	enabled  bool
	identity string
}

func (m *mockExternalThemeSource) Get(_ context.Context) (*entity.ExternalTheme, error) {
	return m.theme, m.err
}

func (m *mockExternalThemeSource) IsEnabled() bool {
	return m.enabled
}

func (m *mockExternalThemeSource) Configure(cfg entity.ExternalThemeConfig) {
	m.enabled = cfg.Enabled
	m.identity = cfg.Provider + "|" + cfg.Format + "|" + cfg.Path
}

func (m *mockExternalThemeSource) ExternalThemeIdentity() string {
	return m.identity
}

type mockExternalThemeWatcher struct{}

func (*mockExternalThemeWatcher) Start(context.Context, entity.ExternalThemeConfig, func()) error {
	return nil
}

func (*mockExternalThemeWatcher) Stop() error {
	return nil
}
