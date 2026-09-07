package port

// FilterAction is the content-filter decision for one request.
type FilterAction int

const (
	// FilterAllow lets the request continue.
	FilterAllow FilterAction = iota
	// FilterBlock cancels the request.
	FilterBlock
)

// FilterRequest describes one request offered to a ContentFilter. URL is
// the full request URL; IsMainFrame reports a main-frame document load
// (as opposed to a subresource); IsNavigation reports a navigation versus
// an in-page resource load.
type FilterRequest struct {
	URL          string
	IsMainFrame  bool
	IsNavigation bool
}

// ContentFilter decides synchronously whether one request may proceed.
// This is a test-only feasibility seam for the CEF filtering gate: no
// production matcher is wired, and none may be added without the separately
// approved implementation design. Matchers must be pure decision functions
// over the request; they never fetch lists, mutate state, or touch the
// network.
type ContentFilter interface {
	Decide(req FilterRequest) FilterAction
}
