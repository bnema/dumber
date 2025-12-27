package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// UpdateRenderer renders update status messages with styled output.
type UpdateRenderer struct {
	theme *Theme
}

// NewUpdateRenderer creates a new update renderer with the given theme.
func NewUpdateRenderer(theme *Theme) *UpdateRenderer {
	return &UpdateRenderer{theme: theme}
}

// RenderChecking renders the "checking for updates" message.
func (*UpdateRenderer) RenderChecking(spinner string) string {
	return fmt.Sprintf("\n  %s Checking for updates...\n", spinner)
}

// RenderUpToDate renders the "already up to date" message.
func (r *UpdateRenderer) RenderUpToDate(version string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	versionStyle := r.theme.Highlight

	return fmt.Sprintf(
		"\n  %s Already up to date (%s)\n",
		iconStyle.Render(IconCheck),
		versionStyle.Render(version),
	)
}

// RenderAvailable renders the "update available" message.
func (r *UpdateRenderer) RenderAvailable(current, latest, releaseURL string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	versionStyle := r.theme.Highlight
	urlStyle := r.theme.Subtle

	return fmt.Sprintf(
		"\n  %s Update available: %s %s %s\n     %s\n",
		iconStyle.Render(IconRocket),
		versionStyle.Render(current),
		iconStyle.Render(IconArrow),
		versionStyle.Render(latest),
		urlStyle.Render(releaseURL),
	)
}

// RenderDownloading renders the "downloading" message with spinner.
func (r *UpdateRenderer) RenderDownloading(spinner, version string) string {
	versionStyle := r.theme.Highlight

	return fmt.Sprintf(
		"\n  %s Downloading %s...\n",
		spinner,
		versionStyle.Render(version),
	)
}

// RenderStaged renders the "update staged" success message.
func (r *UpdateRenderer) RenderStaged(version string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Success)
	versionStyle := r.theme.Highlight

	return fmt.Sprintf(
		"\n  %s Update %s downloaded and staged\n  %s Will be applied when this command exits\n",
		iconStyle.Render(IconCheck),
		versionStyle.Render(version),
		iconStyle.Render(IconCheck),
	)
}

// RenderError renders an error message.
func (r *UpdateRenderer) RenderError(err error) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Error)

	return fmt.Sprintf(
		"\n  %s Update failed: %v\n",
		iconStyle.Render(IconX),
		err,
	)
}

// RenderCannotAutoUpdate renders the "cannot auto-update" message.
func (r *UpdateRenderer) RenderCannotAutoUpdate(current, latest, releaseURL string) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
	versionStyle := r.theme.Highlight
	urlStyle := r.theme.Subtle

	return fmt.Sprintf(
		"\n  %s Update available: %s %s %s\n"+
			"  %s Cannot auto-update: binary is not writable\n"+
			"     Download manually: %s\n",
		iconStyle.Render(IconRocket),
		versionStyle.Render(current),
		iconStyle.Render(IconArrow),
		versionStyle.Render(latest),
		iconStyle.Render(IconWarning),
		urlStyle.Render(releaseURL),
	)
}

// RenderDevBuild renders the "dev build" skip message.
func (r *UpdateRenderer) RenderDevBuild() string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Warning)
	return fmt.Sprintf(
		"\n  %s Development build - update check skipped\n",
		iconStyle.Render(IconInfo),
	)
}
