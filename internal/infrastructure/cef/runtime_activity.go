package cef

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
)

// downloadOwner identifies one download by its owning browser and the CEF
// download ID. Filename and destination are never identity: they can collide
// or change mid-download.
type downloadOwner struct {
	browserID  int32
	downloadID uint32
}

// RuntimeActivityTracker implements port.RuntimeActivity for the CEF engine.
// It counts accepted creations through OnAfterCreated/failure, registered
// live views, outstanding GTK cleanup, and unterminated downloads. All
// methods are safe for concurrent use from CEF callbacks. Delivery is
// serialized under the state lock, so the initial snapshot and every later
// transition arrive in order; subscribers must not reenter the tracker.
// A nil tracker is a safe no-op sink.
type RuntimeActivityTracker struct {
	mu          sync.Mutex
	pending     int
	active      int
	cleanup     int
	downloads   map[downloadOwner]struct{}
	subscribers map[uint64]func(port.RuntimeActivitySnapshot)
	nextSub     uint64
}

// NewRuntimeActivityTracker returns an idle tracker.
func NewRuntimeActivityTracker() *RuntimeActivityTracker {
	return &RuntimeActivityTracker{
		downloads:   make(map[downloadOwner]struct{}),
		subscribers: make(map[uint64]func(port.RuntimeActivitySnapshot)),
	}
}

// Snapshot returns the current outstanding-work counts.
func (t *RuntimeActivityTracker) Snapshot() port.RuntimeActivitySnapshot {
	if t == nil {
		return port.RuntimeActivitySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

// Subscribe delivers the current snapshot synchronously, then every later
// transition in subscription order. Delivery holds the state lock, so
// callbacks must not call back into the tracker. The returned function
// unsubscribes.
func (t *RuntimeActivityTracker) Subscribe(fn func(port.RuntimeActivitySnapshot)) (unsubscribe func()) {
	if t == nil || fn == nil {
		return func() {}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	id := t.nextSub
	t.nextSub++
	t.subscribers[id] = fn
	fn(t.snapshotLocked())
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		delete(t.subscribers, id)
	}
}

// NoteCreationAccepted records one accepted browser creation.
func (t *RuntimeActivityTracker) NoteCreationAccepted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending++
	t.notifyLocked()
}

// NoteCreationResolved records one creation resolved through OnAfterCreated
// (registered) or a definitive failure. Over-resolution is clamped at zero.
func (t *RuntimeActivityTracker) NoteCreationResolved() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending > 0 {
		t.pending--
	}
	t.notifyLocked()
}

// NoteViewRegistered records one live view registration.
func (t *RuntimeActivityTracker) NoteViewRegistered() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active++
	t.notifyLocked()
}

// NoteViewCloseStarted records one native close: the view leaves the active
// set and enters GTK-teardown outstanding.
func (t *RuntimeActivityTracker) NoteViewCloseStarted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active > 0 {
		t.active--
	}
	t.cleanup++
	t.notifyLocked()
}

// NoteCleanupCompleted records one finished GTK bridge/popup cleanup.
func (t *RuntimeActivityTracker) NoteCleanupCompleted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cleanup > 0 {
		t.cleanup--
	}
	t.notifyLocked()
}

// NoteDownloadStarted records one download start keyed by owning browser and
// CEF download ID.
func (t *RuntimeActivityTracker) NoteDownloadStarted(browserID int32, downloadID uint32) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.downloads[downloadOwner{browserID: browserID, downloadID: downloadID}] = struct{}{}
	t.notifyLocked()
}

// NoteDownloadProgress is observed without effect: only start and terminal
// events change activity. Progress without a start never creates work.
func (t *RuntimeActivityTracker) NoteDownloadProgress(_ int32, _ uint32) {}

// NoteDownloadTerminal records one terminal download event (complete,
// cancel, or failure). Duplicate terminals are idempotent; terminals for
// unknown IDs resolve no owner and change nothing.
func (t *RuntimeActivityTracker) NoteDownloadTerminal(downloadID uint32) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for owner := range t.downloads {
		if owner.downloadID == downloadID {
			delete(t.downloads, owner)
		}
	}
	t.notifyLocked()
}

// DropBrowser reconciles owner destruction through the native lifecycle:
// entries owned by a destroyed browser can never receive terminal events.
// It is never driven by window removal alone.
func (t *RuntimeActivityTracker) DropBrowser(browserID int32) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for owner := range t.downloads {
		if owner.browserID == browserID {
			delete(t.downloads, owner)
			changed = true
		}
	}
	if changed {
		t.notifyLocked()
	}
}

func (t *RuntimeActivityTracker) snapshotLocked() port.RuntimeActivitySnapshot {
	return port.RuntimeActivitySnapshot{
		PendingCreations: t.pending,
		ActiveViews:      t.active,
		PendingCleanup:   t.cleanup,
		ActiveDownloads:  len(t.downloads),
	}
}

// notifyLocked delivers the current snapshot to subscribers in subscription
// order. The caller must hold t.mu and must not reenter the tracker from a
// callback.
func (t *RuntimeActivityTracker) notifyLocked() {
	snapshot := t.snapshotLocked()
	for id := uint64(0); id < t.nextSub; id++ {
		if fn, ok := t.subscribers[id]; ok {
			fn(snapshot)
		}
	}
}
