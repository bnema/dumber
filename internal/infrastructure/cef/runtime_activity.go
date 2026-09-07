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
// methods are safe for concurrent use from CEF callbacks; subscriber
// delivery is sequenced under the tracker lock snapshot.
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
// transition in subscription order. The returned function unsubscribes.
func (t *RuntimeActivityTracker) Subscribe(fn func(port.RuntimeActivitySnapshot)) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	if t == nil {
		return func() {}
	}
	t.mu.Lock()
	id := t.nextSub
	t.nextSub++
	t.subscribers[id] = fn
	snapshot := t.snapshotLocked()
	t.mu.Unlock()
	fn(snapshot)
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
	t.pending++
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// NoteCreationResolved records one creation resolved through OnAfterCreated
// (registered) or a definitive failure. Over-resolution is clamped at zero.
func (t *RuntimeActivityTracker) NoteCreationResolved() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.pending > 0 {
		t.pending--
	}
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// NoteViewRegistered records one live view registration.
func (t *RuntimeActivityTracker) NoteViewRegistered() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.active++
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// NoteViewCloseStarted records one native close: the view leaves the active
// set and enters GTK-teardown outstanding.
func (t *RuntimeActivityTracker) NoteViewCloseStarted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.active > 0 {
		t.active--
	}
	t.cleanup++
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// NoteCleanupCompleted records one finished GTK bridge/popup cleanup.
func (t *RuntimeActivityTracker) NoteCleanupCompleted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.cleanup > 0 {
		t.cleanup--
	}
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// NoteDownloadStarted records one download start keyed by owning browser and
// CEF download ID.
func (t *RuntimeActivityTracker) NoteDownloadStarted(browserID int32, downloadID uint32) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.downloads[downloadOwner{browserID: browserID, downloadID: downloadID}] = struct{}{}
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
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
	for owner := range t.downloads {
		if owner.downloadID == downloadID {
			delete(t.downloads, owner)
		}
	}
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

// DropBrowser reconciles owner destruction through the native lifecycle:
// entries owned by a destroyed browser can never receive terminal events.
// It is never driven by window removal alone.
func (t *RuntimeActivityTracker) DropBrowser(browserID int32) {
	if t == nil {
		return
	}
	t.mu.Lock()
	changed := false
	for owner := range t.downloads {
		if owner.browserID == browserID {
			delete(t.downloads, owner)
			changed = true
		}
	}
	if !changed {
		t.mu.Unlock()
		return
	}
	snapshot := t.snapshotLocked()
	subs := t.subscribersLocked()
	t.mu.Unlock()
	notifyRuntimeActivitySubscribers(subs, snapshot)
}

func (t *RuntimeActivityTracker) snapshotLocked() port.RuntimeActivitySnapshot {
	return port.RuntimeActivitySnapshot{
		PendingCreations: t.pending,
		ActiveViews:      t.active,
		PendingCleanup:   t.cleanup,
		ActiveDownloads:  len(t.downloads),
	}
}

func (t *RuntimeActivityTracker) subscribersLocked() []func(port.RuntimeActivitySnapshot) {
	subs := make([]func(port.RuntimeActivitySnapshot), 0, len(t.subscribers))
	for id := uint64(0); id < t.nextSub; id++ {
		if fn, ok := t.subscribers[id]; ok {
			subs = append(subs, fn)
		}
	}
	return subs
}

func notifyRuntimeActivitySubscribers(subs []func(port.RuntimeActivitySnapshot), snapshot port.RuntimeActivitySnapshot) {
	for _, fn := range subs {
		fn(snapshot)
	}
}
