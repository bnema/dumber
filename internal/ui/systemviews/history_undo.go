package systemviews

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// Deferred deletion of single history entries: the row leaves the view at
// once and the bridge delete is sent only when the undo toast is dismissed.
const (
	historyActionCommitDelete = "history.commitDelete"
	historyActionUndoDelete   = "history.undoDelete"
	// actionFlushDeletes commits every pending delete (history, favorites
	// and tags); the DOM sends it when the page is hidden for good (pagehide).
	actionFlushDeletes = "deletes.flush"

	// undoDataKey marks a delete the DOM can offer to undo (the visual layer
	// is present and the confirm dialog was skipped).
	undoDataKey = "undoable"

	historyUndoTimeout = 5 * time.Second
)

func historyUndoAction(action DOMAction) bool {
	switch action.Action {
	case historyActionCommitDelete, historyActionUndoDelete:
		return true
	case historyActionDeleteEntry:
		return action.Data[undoDataKey] == "true"
	default:
		return false
	}
}

func (a *App) handleHistoryUndoAction(ctx context.Context, event DOMAction) error {
	switch event.Action {
	case historyActionDeleteEntry:
		return a.deferHistoryDelete(ctx, event)
	case historyActionUndoDelete:
		return a.undoHistoryDelete(event)
	case historyActionCommitDelete:
		id, err := parsePositiveInt64(event.Data["id"], "history entry id")
		if err != nil {
			return err
		}
		return a.commitHistoryDeletes(ctx, []int64{id})
	default:
		return nil
	}
}

// deferHistoryDelete hides the entry and offers an undo toast. Without a
// toast UI it deletes immediately, like the confirmed path.
func (a *App) deferHistoryDelete(ctx context.Context, event DOMAction) error {
	id, err := parsePositiveInt64(event.Data["id"], "history entry id")
	if err != nil {
		return err
	}
	toaster, canToast := a.deps.DOM.(DOMToaster)
	if !canToast {
		return a.deleteHistoryEntryNow(ctx, event)
	}

	a.lockState()
	if a.closed {
		a.unlockState()
		return nil
	}
	if _, pending := a.pendingHistoryDeletes[id]; pending {
		a.unlockState()
		return nil
	}
	entry := a.takeHistoryEntryLocked(id)
	if a.pendingHistoryDeletes == nil {
		a.pendingHistoryDeletes = map[int64]*entity.HistoryEntry{}
	}
	if entry == nil {
		entry = &entity.HistoryEntry{ID: id}
	}
	a.pendingHistoryDeletes[id] = entry
	a.historyNotice = ""
	a.historyError = ""
	err = a.refreshHistoryRegionsLocked()
	a.unlockState()
	if err != nil {
		return err
	}

	idValue := strconv.FormatInt(id, 10)
	shown := toaster.ShowToast(ToastRequest{
		Message:     historyDeletedMessage(entry),
		ActionLabel: "Undo",
		Timeout:     historyUndoTimeout,
		OnAction:    DOMAction{Action: historyActionUndoDelete, Data: map[string]string{"id": idValue}},
		OnDismiss:   DOMAction{Action: historyActionCommitDelete, Data: map[string]string{"id": idValue}},
	})
	if !shown {
		return a.commitHistoryDeletes(ctx, []int64{id})
	}
	return nil
}

func (a *App) deleteHistoryEntryNow(ctx context.Context, event DOMAction) error {
	plain := DOMAction{Action: historyActionDeleteEntry, Data: map[string]string{"id": event.Data["id"]}}
	a.lockState()
	defer a.unlockState()
	return a.applyHistoryActionLocked(ctx, plain)
}

// undoHistoryDelete puts a pending entry back in its time-ordered slot.
func (a *App) undoHistoryDelete(event DOMAction) error {
	id, err := parsePositiveInt64(event.Data["id"], "history entry id")
	if err != nil {
		return err
	}
	a.lockState()
	defer a.unlockState()
	entry, ok := a.pendingHistoryDeletes[id]
	if !ok || a.closed {
		return nil
	}
	delete(a.pendingHistoryDeletes, id)
	if a.currentRoute != RouteHistory || entry.URL == "" || !a.historyEntryMatchesViewLocked(entry) {
		return a.refreshHistoryRegionsLocked()
	}
	a.historyEntries = insertHistoryEntryByRecency(a.historyEntries, entry)
	return a.refreshHistoryRegionsLocked()
}

// commitHistoryDeletes sends the bridge delete for ids (all pending when ids
// is nil). Entries are dropped from the pending set before the bridge call so
// a late undo can no longer resurrect them.
func (a *App) commitHistoryDeletes(ctx context.Context, ids []int64) error {
	a.lockState()
	if ids == nil {
		for id := range a.pendingHistoryDeletes {
			ids = append(ids, id)
		}
	}
	commit := make([]*entity.HistoryEntry, 0, len(ids))
	for _, id := range ids {
		if entry, ok := a.pendingHistoryDeletes[id]; ok {
			delete(a.pendingHistoryDeletes, id)
			commit = append(commit, entry)
		}
	}
	history := a.deps.History
	a.unlockState()

	if history == nil || len(commit) == 0 {
		return nil
	}
	var (
		failed        []string
		failedEntries []*entity.HistoryEntry
	)
	for _, entry := range commit {
		if err := history.DeleteEntry(ctx, entry.ID); err != nil {
			failed = append(failed, err.Error())
			failedEntries = append(failedEntries, entry)
		}
	}
	if len(failed) == 0 {
		return nil
	}

	a.lockState()
	defer a.unlockState()
	if a.closed {
		return nil
	}
	// The failed entries still exist: show them again where the view allows.
	if a.currentRoute == RouteHistory {
		for _, entry := range failedEntries {
			if entry.URL != "" && a.historyEntryMatchesViewLocked(entry) {
				a.historyEntries = insertHistoryEntryByRecency(a.historyEntries, entry)
			}
		}
	}
	a.historyError = "Could not delete history entry: " + strings.Join(failed, "; ")
	return a.refreshHistoryRegionsLocked()
}

// takeHistoryEntryLocked removes the entry with id from the loaded list.
func (a *App) takeHistoryEntryLocked(id int64) *entity.HistoryEntry {
	for i, entry := range a.historyEntries {
		if entry != nil && entry.ID == id {
			a.historyEntries = append(a.historyEntries[:i:i], a.historyEntries[i+1:]...)
			return entry
		}
	}
	return nil
}

// withoutPendingDeletesLocked hides entries waiting for their undo window.
func (a *App) withoutPendingDeletesLocked(entries []*entity.HistoryEntry) []*entity.HistoryEntry {
	return withoutHiddenEntries(entries, a.pendingHistoryDeleteIDsLocked())
}

func (a *App) pendingHistoryDeleteIDsLocked() map[int64]struct{} {
	if len(a.pendingHistoryDeletes) == 0 {
		return nil
	}
	ids := make(map[int64]struct{}, len(a.pendingHistoryDeletes))
	for id := range a.pendingHistoryDeletes {
		ids[id] = struct{}{}
	}
	return ids
}

func withoutHiddenEntries(entries []*entity.HistoryEntry, hidden map[int64]struct{}) []*entity.HistoryEntry {
	if len(hidden) == 0 {
		return entries
	}
	kept := make([]*entity.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if _, skip := hidden[entry.ID]; !skip {
			kept = append(kept, entry)
		}
	}
	return kept
}

// historyEntryMatchesViewLocked keeps an undone entry out of a view that
// would not have shown it (another domain filter or search).
func (a *App) historyEntryMatchesViewLocked(entry *entity.HistoryEntry) bool {
	if strings.TrimSpace(a.historyQuery) != "" {
		return true
	}
	domain := strings.TrimSpace(a.historyDomainFilter)
	return domain == "" || strings.EqualFold(historyDomainActionKey(&entity.DomainStat{Domain: entry.URL}), domain)
}

func (a *App) refreshHistoryRegionsLocked() error {
	if a.currentRoute != RouteHistory {
		return nil
	}
	data := a.historyRenderDataLocked()
	a.renderedHTML = renderAppFrame(renderedPage{
		route:          RouteHistory,
		title:          historyDocumentTitle(data),
		subtitle:       "History",
		subtitleDetail: historyTitleDetail(data),
		body:           historyHTML(data),
	}, a.shellTheme)
	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok {
		return updater.UpdateRegions(RegionUpdate{Regions: historyLiveRegions(data)})
	}
	return a.mountRenderedHTML()
}

func insertHistoryEntryByRecency(entries []*entity.HistoryEntry, entry *entity.HistoryEntry) []*entity.HistoryEntry {
	at := sort.Search(len(entries), func(i int) bool {
		other := entries[i]
		return other == nil || !other.LastVisited.After(entry.LastVisited)
	})
	entries = append(entries, nil)
	copy(entries[at+1:], entries[at:])
	entries[at] = entry
	return entries
}

func historyDeletedMessage(entry *entity.HistoryEntry) string {
	label := historyItemLabel(entry)
	if label == "" {
		return "Deleted history entry"
	}
	return "Deleted " + quotedLabel(label)
}
