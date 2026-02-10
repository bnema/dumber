package styles

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
)

// SessionsCLIRenderer renders non-interactive CLI output for sessions subcommands
// (e.g. `dumber sessions list`, `restore`, `delete`).
type SessionsCLIRenderer struct {
	theme *Theme
}

func NewSessionsCLIRenderer(theme *Theme) *SessionsCLIRenderer {
	return &SessionsCLIRenderer{theme: theme}
}

func (r *SessionsCLIRenderer) RenderEmptyList() string {
	return r.theme.Subtle.Render("No saved sessions found.")
}

func (r *SessionsCLIRenderer) RenderList(items []entity.SessionInfo, limit int) string {
	if len(items) == 0 {
		return r.RenderEmptyList()
	}

	var b strings.Builder
	title := fmt.Sprintf("%s %s", r.theme.Highlight.Render(IconSessionStack), r.theme.Title.Render("Sessions"))
	b.WriteString(title)
	if limit > 0 {
		b.WriteString(r.theme.Subtle.Render(fmt.Sprintf(" (showing up to %d)", limit)))
	}
	b.WriteString("\n\n")

	for _, s := range items {
		b.WriteString(r.renderOne(s))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(r.theme.Subtle.Render("Tip: use `dumber sessions` for the interactive browser."))
	return b.String()
}

func (r *SessionsCLIRenderer) renderOne(info entity.SessionInfo) string {
	status := " "
	statusStyle := r.theme.Subtle
	switch {
	case info.IsCurrent:
		status = "●"
		statusStyle = r.theme.Highlight
	case info.IsActive:
		status = "○"
		statusStyle = r.theme.Subtle
	}

	id := r.theme.Highlight.Render(string(info.Session.ID))
	tabs := r.theme.BadgeMuted.Render(fmt.Sprintf("%d tabs", info.TabCount))
	panes := r.theme.BadgeMuted.Render(fmt.Sprintf("%d panes", info.PaneCount))
	updated := r.theme.Subtle.Render(usecase.GetRelativeTime(info.UpdatedAt))

	return fmt.Sprintf("%s %s  %s %s  %s",
		statusStyle.Render(status),
		id,
		tabs,
		panes,
		updated,
	)
}

func (r *SessionsCLIRenderer) RenderRestoreStarted(sessionID entity.SessionID) string {
	return fmt.Sprintf("%s Restoring session %s...",
		r.theme.SuccessStyle.Render(IconRestore),
		r.theme.Highlight.Render(string(sessionID)),
	)
}

func (r *SessionsCLIRenderer) RenderDeleted(sessionID entity.SessionID) string {
	return fmt.Sprintf("%s Session %s deleted.",
		r.theme.SuccessStyle.Render(IconCheck),
		r.theme.Highlight.Render(string(sessionID)),
	)
}

func (r *SessionsCLIRenderer) RenderError(err error) string {
	return fmt.Sprintf("%s %v", r.theme.ErrorStyle.Render(IconX), err)
}
