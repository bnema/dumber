package systemviews

import (
	"fmt"
	"html"
)

type DOM interface {
	Mount(html string) error
}

func placeholderHTML(route Route) string {
	name := html.EscapeString(string(route))
	if name == "" || route == RouteUnknown {
		name = "unknown"
	}

	return fmt.Sprintf(`<p>systemviews %s</p>`, name)
}
