package ui

import (
	"slices"

	"github.com/bnema/dumber/internal/domain/entity"
)

// floatingSessionRegistry owns every floating pane session, keyed by tab and
// profile session. All lookups over sessions go through it so the ownership
// rules (one session per key, nil entries ignored, pane-to-tab mapping via
// the key, never by parsing PaneID) live in one place.
type floatingSessionRegistry map[floatingSessionKey]*floatingWorkspaceSession

// forTab returns the non-nil sessions owned by tabID.
func (r floatingSessionRegistry) forTab(tabID entity.TabID) map[floatingSessionKey]*floatingWorkspaceSession {
	sessions := make(map[floatingSessionKey]*floatingWorkspaceSession)
	for key, session := range r {
		if key.tabID == tabID && session != nil {
			sessions[key] = session
		}
	}
	return sessions
}

// byPaneID returns the session hosting paneID and its key.
func (r floatingSessionRegistry) byPaneID(paneID entity.PaneID) (floatingSessionKey, *floatingWorkspaceSession, bool) {
	if paneID == "" {
		return floatingSessionKey{}, nil, false
	}
	for key, session := range r {
		if session != nil && session.paneID == paneID {
			return key, session, true
		}
	}
	return floatingSessionKey{}, nil, false
}

// visibleForTab returns the visible session of tabID, if any.
func (r floatingSessionRegistry) visibleForTab(tabID entity.TabID) (floatingSessionKey, *floatingWorkspaceSession, bool) {
	for key, session := range r {
		if key.tabID != tabID || session == nil || session.pane == nil {
			continue
		}
		if session.pane.IsVisible() {
			return key, session, true
		}
	}
	return floatingSessionKey{tabID: tabID}, nil, false
}

// tabIDs returns the distinct tabs owning at least one session, sorted.
func (r floatingSessionRegistry) tabIDs() []entity.TabID {
	ids := make([]entity.TabID, 0, len(r))
	for key := range r {
		if !slices.Contains(ids, key.tabID) {
			ids = append(ids, key.tabID)
		}
	}
	slices.Sort(ids)
	return ids
}
