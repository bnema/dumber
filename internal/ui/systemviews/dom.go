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

// DOMReadinessSignaler is implemented by DOM adapters that can mark the
// loading shell ready or terminally failed. The App calls it after the
// initial mount and action binding complete; history-data completion is a
// separate concern and never gates readiness. Adapters without it keep
// legacy behavior, leaving shell state to the page script.
type DOMReadinessSignaler interface {
	// SignalReady clears the shell loading state after mount and binding.
	SignalReady() error
	// SignalError clears the shell loading state with a fatal message
	// when mount or binding fails.
	SignalError(message string) error
}

// DOMHistoryTimelineAppender is implemented by DOM adapters that can append
// older history windows without reparsing and remounting the whole page.
type DOMHistoryTimelineAppender interface {
	AppendHistoryTimeline(markup string) error
}

func placeholderHTML(route Route) string {
	name := html.EscapeString(string(route))
	if name == "" || route == RouteUnknown {
		name = "unknown"
	}

	return fmt.Sprintf(`<p>systemviews %s</p>`, name)
}
