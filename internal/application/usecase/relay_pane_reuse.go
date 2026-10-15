package usecase

import (
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
)

type relayPaneRecord struct {
	paneID   entity.PaneID
	openedAt time.Time
}

// RelayPaneReuse remembers panes opened for externally relayed URLs so a
// repeated identical request (desktop apps retry OAuth URLs) reuses the pane
// instead of creating duplicates. It is not safe for concurrent use; callers
// confine it to the UI main thread. The zero value is ready to use.
type RelayPaneReuse struct {
	entries map[string]relayPaneRecord
	// Now overrides the clock in tests; nil means time.Now.
	Now func() time.Time
}

func (r *RelayPaneReuse) clock() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Lookup returns the pane to reuse for url. Entries that expired or whose pane
// no longer exists (per exists) are forgotten first.
func (r *RelayPaneReuse) Lookup(url string, exists func(entity.PaneID) bool) (entity.PaneID, bool) {
	now := r.clock()
	for key, rec := range r.entries {
		if now.Sub(rec.openedAt) > domainurl.RelayReuseWindow || (exists != nil && !exists(rec.paneID)) {
			delete(r.entries, key)
		}
	}
	key := domainurl.RelayURLKey(url)
	rec, ok := r.entries[key]
	if !ok || !domainurl.ShouldReuseRelayPane(key, rec.openedAt, url, now, domainurl.RelayReuseWindow) {
		return "", false
	}
	return rec.paneID, true
}

// Record remembers that paneID was created for url.
func (r *RelayPaneReuse) Record(url string, paneID entity.PaneID) {
	key := domainurl.RelayURLKey(url)
	if key == "" || paneID == "" {
		return
	}
	if r.entries == nil {
		r.entries = make(map[string]relayPaneRecord)
	}
	r.entries[key] = relayPaneRecord{paneID: paneID, openedAt: r.clock()}
}

// Forget drops the record for url.
func (r *RelayPaneReuse) Forget(url string) {
	delete(r.entries, domainurl.RelayURLKey(url))
}

// Len reports how many relay panes are remembered.
func (r *RelayPaneReuse) Len() int {
	return len(r.entries)
}
