package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/application/port"
)

// ConfigRenderer renders config status messages with styled output.
type ConfigRenderer struct {
	theme *Theme
}

// NewConfigRenderer creates a new config renderer with the given theme.
func NewConfigRenderer(theme *Theme) *ConfigRenderer {
	return &ConfigRenderer{theme: theme}
}

// RenderConfigInfo renders the config file info with status.
func (r *ConfigRenderer) RenderConfigInfo(path string, missingCount int) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	pathStyle := r.theme.Subtle

	var status string
	if missingCount > 0 {
		countStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
		status = fmt.Sprintf("\n  %s %s new settings available",
			iconStyle.Render(IconInfo),
			countStyle.Render(fmt.Sprintf("%d", missingCount)),
		)
	}

	return fmt.Sprintf(
		"\n  %s Config %s%s\n",
		iconStyle.Render(IconConfig),
		pathStyle.Render(path),
		status,
	)
}

// RenderMissingKeys renders the list of missing keys with their types and default values.
func (r *ConfigRenderer) RenderMissingKeys(keys []port.KeyInfo) string {
	if len(keys) == 0 {
		return ""
	}

	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	keyStyle := r.theme.Highlight
	typeStyle := r.theme.Subtle
	valueStyle := lipgloss.NewStyle().Foreground(r.theme.Text)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  Missing settings (%d):\n", len(keys)))

	for _, key := range keys {
		sb.WriteString(fmt.Sprintf(
			"    %s %s\n      Type: %s | Default: %s\n",
			iconStyle.Render(IconCursor),
			keyStyle.Render(key.Key),
			typeStyle.Render(key.Type),
			valueStyle.Render(key.DefaultValue),
		))
	}

	return sb.String()
}

// RenderMigrationSuccess renders the success message after migration.
func (r *ConfigRenderer) RenderMigrationSuccess(count int, path string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	countStyle := r.theme.Highlight
	pathStyle := r.theme.Subtle

	// Get just the filename from the path
	parts := strings.Split(path, "/")
	filename := parts[len(parts)-1]

	return fmt.Sprintf(
		"\n  %s Added %s new settings to %s\n",
		iconStyle.Render(IconCheck),
		countStyle.Render(fmt.Sprintf("%d", count)),
		pathStyle.Render(filename),
	)
}

// RenderUpToDate renders the "config is up to date" message.
func (r *ConfigRenderer) RenderUpToDate(path string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	pathStyle := r.theme.Subtle

	return fmt.Sprintf(
		"\n  %s Config %s\n  %s Config is up to date\n",
		iconStyle.Render(IconConfig),
		pathStyle.Render(path),
		iconStyle.Render(IconCheck),
	)
}

// RenderError renders an error message.
func (r *ConfigRenderer) RenderError(err error) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Error)

	return fmt.Sprintf(
		"\n  %s Config error: %v\n",
		iconStyle.Render(IconX),
		err,
	)
}

// RenderChecking renders the "checking config..." message with spinner.
func (*ConfigRenderer) RenderChecking(spinner string) string {
	return fmt.Sprintf("\n  %s Checking config...\n", spinner)
}

// RenderMigrateHint renders a hint to run the migrate command.
func (r *ConfigRenderer) RenderMigrateHint() string {
	hintStyle := r.theme.Subtle

	return fmt.Sprintf(
		"\n  %s\n",
		hintStyle.Render("Run 'dumber config migrate' to add missing defaults."),
	)
}

// RenderNoConfigFile renders message when config file doesn't exist yet.
func (r *ConfigRenderer) RenderNoConfigFile(path string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	pathStyle := r.theme.Subtle
	hintStyle := r.theme.Subtle

	return fmt.Sprintf(
		"\n  %s Config %s\n  %s\n",
		iconStyle.Render(IconConfig),
		pathStyle.Render(path),
		hintStyle.Render("Config file will be created on first run with all defaults."),
	)
}
