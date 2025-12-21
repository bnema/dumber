package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/domain/build"
)

// AboutRenderer renders build info in fastfetch style.
type AboutRenderer struct {
	theme *Theme
}

// NewAboutRenderer creates a new about renderer with the given theme.
func NewAboutRenderer(theme *Theme) *AboutRenderer {
	return &AboutRenderer{theme: theme}
}

// Render renders build info with ASCII logo and styled info lines.
func (r *AboutRenderer) Render(info build.Info) string {
	logo := r.renderLogo()
	lines := r.renderInfoLines(info)

	// Combine horizontally: logo | info
	return lipgloss.JoinHorizontal(lipgloss.Top, logo, "   ", lines)
}

func (r *AboutRenderer) renderLogo() string {
	// ASCII art: Lightning bolt + D (matches pixel logo)
	// All in accent color (green)
	logoStyle := lipgloss.NewStyle().Foreground(r.theme.Accent).Bold(true)

	// Simple bold D
	logo := `██████▄
██   ██
██   ██
██   ██
██████▀`

	return logoStyle.MarginTop(1).MarginLeft(2).Render(logo)
}

func (r *AboutRenderer) renderInfoLines(info build.Info) string {
	keyStyle := r.theme.Subtle
	valStyle := r.theme.Highlight
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)

	lines := []string{
		fmt.Sprintf("%s %s %s", iconStyle.Render(IconVersion), keyStyle.Render("Version"), valStyle.Render(info.Version)),
		fmt.Sprintf("%s %s %s", iconStyle.Render(IconGitBranch), keyStyle.Render("Commit"), valStyle.Render(info.Commit)),
		fmt.Sprintf("%s %s %s", iconStyle.Render(IconCalendar), keyStyle.Render("Built"), valStyle.Render(info.BuildDate)),
		fmt.Sprintf("%s %s %s", iconStyle.Render(IconGo), keyStyle.Render("Go"), valStyle.Render(info.GoVersion)),
		"",
		fmt.Sprintf("%s %s", iconStyle.Render(IconGithub), keyStyle.Render(build.RepoURL())),
		fmt.Sprintf(
			"%s %s %s",
			iconStyle.Render(IconHeart),
			keyStyle.Render("Made with love by"),
			valStyle.Render(strings.Join(build.Contributors(), ", ")),
		),
	}

	return strings.Join(lines, "\n")
}
