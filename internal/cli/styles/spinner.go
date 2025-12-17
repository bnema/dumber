package styles

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerType defines available spinner styles.
type SpinnerType int

const (
	SpinnerDots SpinnerType = iota
	SpinnerLine
	SpinnerMiniDot
	SpinnerJump
	SpinnerPulse
	SpinnerPoints
	SpinnerGlobe
	SpinnerMoon
	SpinnerMonkey
)

// NewStyledSpinner creates a themed spinner.
func NewStyledSpinner(theme *Theme, spinnerType SpinnerType) spinner.Model {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)

	switch spinnerType {
	case SpinnerDots:
		s.Spinner = spinner.Dot
	case SpinnerLine:
		s.Spinner = spinner.Line
	case SpinnerMiniDot:
		s.Spinner = spinner.MiniDot
	case SpinnerJump:
		s.Spinner = spinner.Jump
	case SpinnerPulse:
		s.Spinner = spinner.Pulse
	case SpinnerPoints:
		s.Spinner = spinner.Points
	case SpinnerGlobe:
		s.Spinner = spinner.Globe
	case SpinnerMoon:
		s.Spinner = spinner.Moon
	case SpinnerMonkey:
		s.Spinner = spinner.Monkey
	default:
		s.Spinner = spinner.Dot
	}

	return s
}

// NewDefaultSpinner creates the default themed spinner.
func NewDefaultSpinner(theme *Theme) spinner.Model {
	return NewStyledSpinner(theme, SpinnerDots)
}

// LoadingModel wraps a spinner with a message.
type LoadingModel struct {
	Spinner spinner.Model
	Message string
	theme   *Theme
}

// NewLoading creates a loading indicator with message.
func NewLoading(theme *Theme, message string) LoadingModel {
	return LoadingModel{
		Spinner: NewDefaultSpinner(theme),
		Message: message,
		theme:   theme,
	}
}

// View renders the loading indicator.
func (m LoadingModel) View() string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.Spinner.View(),
		" ",
		m.theme.Subtle.Render(m.Message),
	)
}
