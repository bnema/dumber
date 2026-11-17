package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/a-h/templ"
)

type kvPair struct {
	Label string
	Value string
}

// renderedPage holds the data needed by renderAppFrame to produce the shell
// fragment. Route views populate this; the app frame wraps it.
type renderedPage struct {
	route          Route
	title          string
	subtitle       string
	subtitleDetail string
	body           string
}

// renderAppFrame produces a self-contained HTML fragment (no <html>, <head>,
// or <body>) that wraps route content in the shared shell. page.body is
// inserted with templ.Raw, so callers must only pass trusted pre-rendered HTML.
func renderAppFrame(page renderedPage, theme shellTheme) string {
	return mustRenderComponent(appFrameComponent(page, theme, templ.Raw(page.body)))
}

func pageDocumentTitle(page renderedPage) string {
	if title := strings.TrimSpace(page.title); title != "" {
		return title
	}
	if subtitle := strings.TrimSpace(page.subtitle); subtitle != "" {
		return subtitle
	}
	return routeSubtitle(page.route)
}

// shellStyleClass selects the chrome variant, matching the native overlays'
// omnibox.style. Only the default "command" style exists for system views.
const shellStyleClass = "sv-style-command"

func appRootClass(theme shellTheme) string {
	themeClass := theme.RootClass
	if themeClass == "" {
		themeClass = "sv-dark"
	}
	return "sv-app " + shellStyleClass + " " + themeClass
}

func alertClass(kind string) string {
	classes := "sv-alert"
	switch kind {
	case "error":
		return classes + " sv-alert-error"
	case "success":
		return classes + " sv-alert-success"
	default:
		return classes
	}
}

func alertRole(kind string) string {
	if kind == "error" {
		return "alert"
	}
	return "status"
}

func linkHTML(href, label string) string {
	return fmt.Sprintf(`<a class=%q href=%q>%s</a>`, "sv-link", html.EscapeString(sanitizeHref(href)), html.EscapeString(label))
}

func kvHTML(rows []kvPair) string {
	var items strings.Builder
	for _, row := range rows {
		if strings.TrimSpace(row.Label) == "" && strings.TrimSpace(row.Value) == "" {
			continue
		}
		items.WriteString(kvRowHTML(row.Label, row.Value))
	}
	if items.Len() == 0 {
		return emptyStateHTML("No configuration details")
	}

	return fmt.Sprintf(`<dl class="sv-kv">%s</dl>`, items.String())
}

func kvRowHTML(label, value string) string {
	return fmt.Sprintf(`<div class=%q><dt>%s</dt><dd>%s</dd></div>`, "sv-kv-row", html.EscapeString(label), html.EscapeString(value))
}

// emptyStateHTML returns a standard empty-state placeholder.
func emptyStateHTML(message string) string {
	return fmt.Sprintf(`<div class="sv-empty"><p>%s</p></div>`, html.EscapeString(message))
}

func errorStateHTML(message string) string {
	return fmt.Sprintf(
		`<div class="sv-error" role="alert"><p>Could not load this system view.</p><p class="sv-meta">%s</p></div>`,
		html.EscapeString(message),
	)
}
