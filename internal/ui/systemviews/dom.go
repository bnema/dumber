package systemviews

import (
	"fmt"
	"html"
)

type DOM interface {
	Mount(markup string) error
}

// DOMAction is a browser event delegated from the mounted systemview DOM.
type DOMAction struct {
	Action string
	Data   map[string]string
}

// DOMActionHandler handles delegated browser actions.
type DOMActionHandler func(DOMAction)

// DOMActionBinder is implemented by DOM adapters that can delegate events back
// to the App. Tests can keep using the smaller DOM interface.
type DOMActionBinder interface {
	BindActions(handler DOMActionHandler) error
}

func placeholderHTML(route Route) string {
	name := html.EscapeString(string(route))
	if name == "" || route == RouteUnknown {
		name = "unknown"
	}

	return fmt.Sprintf(`<p>systemviews %s</p>`, name)
}
