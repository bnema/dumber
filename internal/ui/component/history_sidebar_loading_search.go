package component

import (
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

// =============================================================================
// Data loading — background goroutine with paging
// =============================================================================

func (hs *HistorySidebar) startLoadHistory() {
	hs.mu.Lock()
	hs.loadGen++
	gen := hs.loadGen
	hs.loadStarted = true
	hs.isLoading = true
	hs.mu.Unlock()

	// Fetch first page in a background goroutine
	go hs.fetchPage(0, gen)
}

// fetchPage fetches a page of history entries in a background goroutine
// and schedules the UI update on the GTK main thread.
func (hs *HistorySidebar) fetchPage(offset int, gen uint64) {
	hs.mu.RLock()
	uc := hs.historyUC
	ctx := hs.ctx
	hs.mu.RUnlock()

	if uc == nil || ctx == nil {
		// No provider; show empty state
		cb := glib.SourceFunc(func(uintptr) bool {
			hs.mu.Lock()
			if hs.destroyed {
				hs.mu.Unlock()
				return false
			}
			hs.loadStarted = false
			hs.isLoading = false
			hs.loadDone = true
			hs.hasMore = false
			hs.mu.Unlock()
			hs.scheduleRebuild()
			return false
		})
		hs.scheduleIdle(cb)
		return
	}

	entries, err := uc.GetRecent(ctx, sidebarPageSize, offset)
	if err != nil {
		hs.logger.Error().Err(err).Int("offset", offset).Msg("failed to load history page")
	}

	if entries == nil {
		entries = []*entity.HistoryEntry{}
	}

	hasMore := len(entries) >= sidebarPageSize

	hs.mu.Lock()

	// If a newer load was started since this fetch began, drop stale results.
	// Must NOT mutate isLoading/loadStarted — they belong to the current
	// generation set by startLoadHistory or LoadMore.
	if gen != hs.loadGen {
		hs.mu.Unlock()
		return
	}

	// If search is active, don't update browse state with stale page data
	// and don't overwrite search results.
	if hs.currentQuery != "" {
		hs.isLoading = false
		hs.loadStarted = false
		hs.mu.Unlock()
		return
	}

	hs.totalLoaded = offset + len(entries)
	hs.hasMore = hasMore
	hs.isLoading = false
	hs.loadStarted = false
	hs.loadDone = true

	if offset == 0 {
		// First page: replace all entries
		hs.allEntries = entries
	} else {
		// Subsequent page: append
		hs.allEntries = append(hs.allEntries, entries...)
	}

	// Group for display
	hs.setDisplayGroupsLocked(groupHistoryByDay(hs.allEntries))
	hs.mu.Unlock()

	// Schedule UI rebuild on GTK main thread
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.rebuildList()
		return false
	})
	hs.scheduleIdle(cb)
}

// LoadMore fetches the next page and appends it to the existing entries.
func (hs *HistorySidebar) LoadMore() {
	hs.mu.Lock()
	if hs.isLoading || !hs.hasMore || hs.destroyed || hs.currentQuery != "" {
		hs.mu.Unlock()
		return
	}
	hs.isLoading = true
	offset := hs.totalLoaded
	gen := hs.loadGen
	hs.mu.Unlock()

	hs.logger.Debug().Int("offset", offset).Msg("loading more history entries")
	go hs.fetchPage(offset, gen)
}

// =============================================================================
// Scroll-aware load-more: detects when the user reaches the bottom
// =============================================================================

func (hs *HistorySidebar) setupScrollLoadMore() {
	if hs.scrolledWin == nil {
		return
	}

	vadj := hs.scrolledWin.GetVadjustment()
	if vadj == nil {
		return
	}

	changedCb := func(_ gtk.Adjustment) {
		hs.mu.RLock()
		if hs.destroyed || !hs.hasMore || hs.isLoading {
			hs.mu.RUnlock()
			return
		}
		value := vadj.GetValue()
		upper := vadj.GetUpper()
		pageSize := vadj.GetPageSize()
		hs.mu.RUnlock()

		// Trigger load-more when within 200px of the bottom
		if pageSize > 0 && value+pageSize >= upper-200.0 {
			hs.LoadMore()
		}
	}
	hs.retainedCallbacks = append(hs.retainedCallbacks, changedCb)
	vadj.ConnectValueChanged(&changedCb)
}

// =============================================================================
// Scroll/selection preservation
// =============================================================================

// preserveScrollAndSelection saves the current scroll position and selected row
// URL before a rebuild. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) preserveScrollAndSelection() {
	hs.prevScrollValue = 0
	hs.prevSelectedURL = ""

	if hs.scrolledWin != nil {
		if vadj := hs.scrolledWin.GetVadjustment(); vadj != nil {
			hs.prevScrollValue = vadj.GetValue()
		}
	}
	if hs.listBox != nil {
		if selected := hs.listBox.GetSelectedRow(); selected != nil {
			if url := hs.entryURLAtIndex(selected.GetIndex()); url != "" {
				hs.prevSelectedURL = url
			}
		}
	}
}

// restoreScrollAndSelection restores the previously saved scroll position and
// selection after a rebuild. Called on the GTK main thread.
func (hs *HistorySidebar) restoreScrollAndSelectionInListBox(listBox *gtk.ListBox) {
	// Restore selection first (changes scroll position)
	if hs.prevSelectedURL != "" {
		hs.selectRowByURLInListBox(listBox, hs.prevSelectedURL)
	}

	// Then restore scroll position if we have one
	if hs.prevScrollValue > 0 && hs.scrolledWin != nil {
		if vadj := hs.scrolledWin.GetVadjustment(); vadj != nil {
			maxVal := vadj.GetUpper() - vadj.GetPageSize()
			if hs.prevScrollValue > maxVal {
				hs.prevScrollValue = maxVal
			}
			vadj.SetValue(hs.prevScrollValue)
		}
	}

	hs.prevScrollValue = 0
	hs.prevSelectedURL = ""
}

// getRowURL extracts the URL stored in a list box row.
func (hs *HistorySidebar) getRowURL(row *gtk.ListBoxRow) string {
	if row == nil || !row.GetSelectable() {
		return ""
	}
	child := row.GetChild()
	if child == nil {
		return ""
	}

	// The child is the vertical box. Walk children to find our stored URL.
	// We store the URL directly on the row as data.
	// Actually, let's use a simpler approach: walk the list box to find the entry.
	idx := row.GetIndex()
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return hs.entryURLAtIndex(idx)
}

// entryURLAtIndex returns the URL of the history entry at the given
// linear list index (including group headers which return "").
func (hs *HistorySidebar) entryURLAtIndex(index int) string {
	return newKeyboardNavModelFromRows(hs.displayRows).entryURLAt(index)
}

// selectRowByURL finds and selects a row whose URL matches.
func (hs *HistorySidebar) selectRowByURLInListBox(listBox *gtk.ListBox, url string) {
	if url == "" || listBox == nil {
		return
	}
	for i := 0; ; i++ {
		row := listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		if !row.GetSelectable() {
			continue
		}
		if hs.getRowURL(row) == url {
			listBox.SelectRow(row)
			return
		}
	}
}

// =============================================================================
// Search / filtering
// =============================================================================

func (hs *HistorySidebar) setupSearchHandler() {
	if hs.searchEntry == nil {
		return
	}

	changedCb := func(_ gtk.SearchEntry) {
		hs.onSearchChanged()
	}
	hs.retainedCallbacks = append(hs.retainedCallbacks, changedCb)
	hs.searchEntry.ConnectSearchChanged(&changedCb)
}

func (hs *HistorySidebar) onSearchChanged() {
	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		return
	}
	hs.currentQuery = hs.searchEntry.GetText()
	hs.preserveScrollAndSelection()
	oldTimer := hs.debounceTimer
	hs.debounceTimer = 0
	hs.mu.Unlock()

	if oldTimer != 0 {
		hs.removeSource(oldTimer)
	}

	filterCb := glib.SourceFunc(func(uintptr) bool {
		hs.applyFilter()
		return false
	})
	timerID := hs.addTimeout(sidebarSearchDebounceMs, filterCb)

	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		if timerID != 0 {
			hs.removeSource(timerID)
		}
		return
	}
	hs.debounceTimer = timerID
	hs.mu.Unlock()
}

func (hs *HistorySidebar) applyFilter() {
	hs.mu.Lock()
	hs.debounceTimer = 0
	query := hs.currentQuery

	if query == "" {
		// Empty query: use in-memory browse entries (paged getRecent).
		// Clear search state and invalidate any in-flight search so a late
		// search result doesn't overwrite browse state.
		hs.searchResults = nil
		hs.searchDone = false
		hs.searchGen++
		hs.setDisplayGroupsLocked(nil)
		if !hs.loadDone {
			// Browse was never fully loaded (e.g., a search superseded the
			// initial page fetch). Clear the list, show a loading indicator,
			// and restart loading history in the background.
			hs.mu.Unlock()
			hs.scheduleRebuild() // Shows "Loading history…" while fetch runs
			hs.startLoadHistory()
			return
		}
		hs.setDisplayGroupsLocked(groupHistoryByDay(hs.allEntries))
		hs.mu.Unlock()
		hs.scheduleRebuild()
		return
	}

	// Non-empty query: use real FTS search via the provider.
	// Cancel any stale in-flight search via generation counter.
	hs.searchGen++
	gen := hs.searchGen
	hs.searchDone = false
	hs.searchResults = nil
	hs.setDisplayGroupsLocked(nil)
	hs.mu.Unlock()

	// Clear the list immediately to avoid showing stale browse results
	// while the search is in flight.
	hs.scheduleClearList()

	hs.doFTSearch(query, gen)
}

// doFTSearch runs a history FTS search in a background goroutine and
// updates the display when results arrive. Stale results (from a superseded
// search generation) are silently dropped.
func (hs *HistorySidebar) doFTSearch(query string, gen uint64) {
	hs.mu.RLock()
	uc := hs.historyUC
	ctx := hs.ctx
	hs.mu.RUnlock()

	if uc == nil || ctx == nil {
		return
	}

	go func() {
		out, err := uc.Search(ctx, dto.HistorySearchInput{Query: query, Limit: sidebarSearchLimit})
		var entries []*entity.HistoryEntry
		if out != nil {
			entries = make([]*entity.HistoryEntry, len(out.Matches))
			for i, m := range out.Matches {
				entries[i] = m.Entry
			}
		}
		if err != nil {
			hs.logger.Error().Err(err).Str("query", query).Msg("history FTS search failed")
		}
		if entries == nil {
			entries = []*entity.HistoryEntry{}
		}

		// Apply results on the GTK main thread with stale-result protection
		cb := glib.SourceFunc(func(uintptr) bool {
			if hs.applySearchResults(entries, gen, err) {
				hs.scheduleRebuild()
			}
			return false
		})
		hs.scheduleIdle(cb)
	}()
}

// applySearchResults applies search results under the generation guard.
// Returns true if results were applied (non-stale), false if the generation
// had moved on and the results were dropped.
func (hs *HistorySidebar) applySearchResults(entries []*entity.HistoryEntry, gen uint64, err error) bool {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.destroyed || gen != hs.searchGen {
		return false
	}
	hs.searchResults = entries
	hs.searchDone = true
	hs.searchErr = err
	hs.setDisplayGroupsLocked(groupHistoryByDay(entries))
	return true
}

func (hs *HistorySidebar) scheduleIdle(cb glib.SourceFunc) {
	if hs != nil && hs.idleScheduler != nil {
		hs.idleScheduler(cb)
		return
	}
	glib.IdleAdd(&cb, 0)
}

// scheduleClearList clears the list box on the GTK main thread.
func (hs *HistorySidebar) scheduleClearList() {
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		listBox := hs.listBox
		hs.mu.RUnlock()
		if destroyed || listBox == nil {
			return false
		}
		hs.mu.Lock()
		hs.clearRelativeTimeBindingsLocked()
		hs.mu.Unlock()
		listBox.RemoveAll()
		return false
	})
	hs.scheduleIdle(cb)
}

// scheduleRebuild schedules a list rebuild on the GTK main thread.
func (hs *HistorySidebar) scheduleRebuild() {
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.rebuildList()
		return false
	})
	hs.scheduleIdle(cb)
}

// setDisplayGroupsLocked updates grouped history and the explicit display-row
// model. Caller must hold hs.mu for writing.
func (hs *HistorySidebar) setDisplayGroupsLocked(groups []historyGroup) {
	hs.groups = groups
	hs.displayRows = buildHistoryDisplayRows(groups)
}
