package styles

import (
	"fmt"
)

// CrashesCLIRenderer renders non-interactive CLI output for crashes subcommands
// that otherwise print raw crash markdown to stdout.
type CrashesCLIRenderer struct {
	theme *Theme
}

func NewCrashesCLIRenderer(theme *Theme) *CrashesCLIRenderer {
	return &CrashesCLIRenderer{theme: theme}
}

func (r *CrashesCLIRenderer) RenderError(err error) string {
	return fmt.Sprintf("%s %v", r.theme.ErrorStyle.Render(IconX), err)
}

func (r *CrashesCLIRenderer) RenderHintList() string {
	return r.theme.Subtle.Render("Hint: run `dumber crashes` to list available reports.")
}
