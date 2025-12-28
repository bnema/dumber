package styles

import (
	"fmt"
	"path/filepath"
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
func (r *ConfigRenderer) RenderConfigInfo(path string, changeCount int) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	pathStyle := r.theme.Subtle

	var status string
	if changeCount > 0 {
		countStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
		status = fmt.Sprintf("\n  %s %s changes detected",
			iconStyle.Render(IconInfo),
			countStyle.Render(fmt.Sprintf("%d", changeCount)),
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

// RenderChanges renders the list of detected changes (added, renamed, removed).
func (r *ConfigRenderer) RenderChanges(changes []port.KeyChange) string {
	if len(changes) == 0 {
		return ""
	}

	addStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	renameStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
	removeStyle := lipgloss.NewStyle().Foreground(r.theme.Error)
	keyStyle := r.theme.Highlight
	valueStyle := r.theme.Subtle

	// Group changes by type
	var added, renamed, removed []port.KeyChange
	for _, c := range changes {
		switch c.Type {
		case port.KeyChangeAdded:
			added = append(added, c)
		case port.KeyChangeRenamed:
			renamed = append(renamed, c)
		case port.KeyChangeRemoved:
			removed = append(removed, c)
		}
	}

	var sb strings.Builder

	// Show renamed keys first (most important for user to understand)
	if len(renamed) > 0 {
		sb.WriteString(fmt.Sprintf("\n  Renamed settings (%d):\n", len(renamed)))
		for _, c := range renamed {
			sb.WriteString(fmt.Sprintf(
				"    %s %s %s %s\n      Value: %s\n",
				renameStyle.Render("~"),
				valueStyle.Render(c.OldKey),
				renameStyle.Render("→"),
				keyStyle.Render(c.NewKey),
				valueStyle.Render(c.OldValue),
			))
		}
	}

	// Show new settings
	if len(added) > 0 {
		sb.WriteString(fmt.Sprintf("\n  New settings (%d):\n", len(added)))
		for _, c := range added {
			sb.WriteString(fmt.Sprintf(
				"    %s %s\n      Default: %s\n",
				addStyle.Render("+"),
				keyStyle.Render(c.NewKey),
				valueStyle.Render(c.NewValue),
			))
		}
	}

	// Show deprecated settings (will be removed)
	if len(removed) > 0 {
		sb.WriteString(fmt.Sprintf("\n  Deprecated settings (%d):\n", len(removed)))
		for _, c := range removed {
			sb.WriteString(fmt.Sprintf(
				"    %s %s %s\n",
				removeStyle.Render("-"),
				valueStyle.Render(c.OldKey),
				removeStyle.Render("(will be ignored)"),
			))
		}
	}

	return sb.String()
}

// RenderChangesSummary renders a summary of changes for the config info header.
func (r *ConfigRenderer) RenderChangesSummary(changes []port.KeyChange) string {
	var added, renamed, removed int
	for _, c := range changes {
		switch c.Type {
		case port.KeyChangeAdded:
			added++
		case port.KeyChangeRenamed:
			renamed++
		case port.KeyChangeRemoved:
			removed++
		}
	}

	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	addStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	renameStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
	removeStyle := lipgloss.NewStyle().Foreground(r.theme.Error)

	var parts []string
	if renamed > 0 {
		parts = append(parts, renameStyle.Render(fmt.Sprintf("%d renamed", renamed)))
	}
	if added > 0 {
		parts = append(parts, addStyle.Render(fmt.Sprintf("%d new", added)))
	}
	if removed > 0 {
		parts = append(parts, removeStyle.Render(fmt.Sprintf("%d deprecated", removed)))
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("\n  %s %s", iconStyle.Render(IconInfo), strings.Join(parts, ", "))
}

// RenderMigrationSuccess renders the success message after migration.
func (r *ConfigRenderer) RenderMigrationSuccess(count int, path string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	countStyle := r.theme.Highlight
	pathStyle := r.theme.Subtle

	// Get just the filename from the path
	filename := filepath.Base(path)

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
