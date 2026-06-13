package component

// searchStateSnapshot captures the observable state of a history search
// at a point in time. This is test-only data for pure transition checks.
type searchStateSnapshot struct {
	Query         string
	HasSearchDone bool
	HasResults    bool
	ResultCount   int
}

// transitionSearchState models a search state transition for tests without GTK.
func transitionSearchState(newQuery string, resultCount int) searchStateSnapshot {
	return searchStateSnapshot{
		Query:         newQuery,
		HasSearchDone: newQuery != "",
		HasResults:    newQuery != "" && resultCount > 0,
		ResultCount:   resultCount,
	}
}

// reloadPreservationSnapshot captures the preserved state during a reload.
type reloadPreservationSnapshot struct {
	PreservedQuery string
	ResetBrowse    bool
	ClearSearch    bool
}

// applyReloadState computes the expected state after a reload for test-only
// transition checks.
func applyReloadState(currentQuery string) reloadPreservationSnapshot {
	s := reloadPreservationSnapshot{PreservedQuery: currentQuery}
	if currentQuery == "" {
		s.ResetBrowse = true
	} else {
		s.ClearSearch = true
	}
	return s
}
