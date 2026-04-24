package systemviews

import (
	"fmt"
	"html"
	"strings"
)

type kvPair struct {
	Label string
	Value string
}

// renderedPage holds the data needed by renderAppFrame to produce the shell
// fragment. Route views populate this; the app frame wraps it.
type renderedPage struct {
	route    Route
	subtitle string
	body     string
}

// renderAppFrame produces a self-contained HTML fragment (no <html>, <head>,
// or <body>) that wraps route content in the shared shell.
func renderAppFrame(page renderedPage, theme shellTheme) string {
	rootClass := "sv-app"
	themeClass := theme.RootClass
	if themeClass == "" {
		themeClass = "sv-dark"
	}
	rootClass += " " + themeClass

	var styleAttr string
	if theme.InlineVars != "" {
		styleAttr = fmt.Sprintf(` style=%q`, html.EscapeString(theme.InlineVars))
	}

	routeAttr := html.EscapeString(string(page.route))

	var subtitle string
	if page.subtitle != "" {
		subtitle = fmt.Sprintf(`<h1 class="sv-title">%s</h1>`, html.EscapeString(page.subtitle))
	}

	return fmt.Sprintf(`<div class=%q data-route=%q%s><div class=%q>%s<div class=%q>%s</div></div></div>`,
		html.EscapeString(rootClass),
		routeAttr,
		styleAttr,
		"sv-shell",
		subtitle,
		"sv-content",
		page.body,
	)
}

func listHTML(rows, emptyMessage string) string {
	if strings.TrimSpace(rows) == "" {
		return emptyStateHTML(emptyMessage)
	}

	return fmt.Sprintf(`<ul class="sv-list">%s</ul>`, rows)
}

func listRowHTML(inner string) string {
	return fmt.Sprintf(`<li class="sv-list-row">%s</li>`, inner)
}

func linkHTML(href, label string) string {
	return fmt.Sprintf(`<a class=%q href=%q>%s</a>`, "sv-link", html.EscapeString(sanitizeHref(href)), html.EscapeString(label))
}

func metaHTML(text string) string {
	return fmt.Sprintf(`<p class="sv-meta">%s</p>`, html.EscapeString(text))
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

// sectionHTML wraps content in a styled section with a heading.
func sectionHTML(class, title, inner string) string {
	classes := "sv-section"
	if strings.TrimSpace(class) != "" {
		classes += " " + strings.TrimSpace(class)
	}

	return fmt.Sprintf(`<section class=%q><h2>%s</h2>%s</section>`,
		html.EscapeString(classes),
		html.EscapeString(title),
		inner,
	)
}

// emptyStateHTML returns a standard empty-state placeholder.
func emptyStateHTML(message string) string {
	return fmt.Sprintf(`<div class="sv-empty"><p>%s</p></div>`, html.EscapeString(message))
}

func errorStateHTML(message string) string {
	return fmt.Sprintf(`<div class="sv-error" role="alert"><p>Could not load this system view.</p><p class="sv-meta">%s</p></div>`, html.EscapeString(message))
}
