package cef

import (
	"net/url"
	"strings"

	purecef "bnema.dev/purego-cef/cef"
)

// isInternalTargetURL reports whether raw addresses a system view: the
// conceptual dumb: scheme or the internal https origin. The host is compared
// exactly (case-insensitive, port and userinfo ignored), never by prefix.
func isInternalTargetURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "dumb":
		return true
	case actualInternalScheme:
		return strings.EqualFold(u.Hostname(), actualInternalHost)
	default:
		return false
	}
}

// blockInternalNavigation decides whether a navigation to a system view must
// be canceled. Nothing legitimately frames system views, so subframes never
// load them. A main frame may load one when it already shows a system view
// (links between them, dumb: to https redirects) or when the browser itself
// started the navigation: Dumber's LoadURL (omnibox, keyboard, CLI, new
// pane, session restore) carries the direct-load flag, back/forward the
// forward-back flag. Page-initiated navigations (links, location=, forms,
// redirects from web pages) from any other page are blocked, user gesture or
// not: a click inside an embed is a gesture too.
func blockInternalNavigation(isMain bool, currentURL, targetURL string, transition purecef.TransitionType, isRedirect bool) bool {
	if !isInternalTargetURL(targetURL) {
		return false
	}
	if !isMain {
		return true
	}
	if isInternalTargetURL(currentURL) {
		return false
	}
	if isRedirect {
		return true
	}
	if transition&purecef.TransitionTypeTtDirectLoadFlag != 0 || transition&purecef.TransitionTypeTtForwardBackFlag != 0 {
		return false
	}
	return transition&purecef.TransitionTypeTtSourceMask != purecef.TransitionTypeTtReload
}

// blockInternalPopup reports whether a popup to targetURL must be refused:
// only a system view page (a main frame on the internal origin) may open
// another system view.
func blockInternalPopup(sourceIsMain bool, sourceURL, targetURL string) bool {
	if !isInternalTargetURL(targetURL) {
		return false
	}
	return !sourceIsMain || !isInternalTargetURL(sourceURL)
}
