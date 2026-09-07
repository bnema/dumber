package port

import "context"

// RuntimeActivitySnapshot is an engine-neutral view of outstanding native
// runtime work that must settle before the process may idle out. Counts only:
// generations and acknowledgement correlation live with the implementation.
type RuntimeActivitySnapshot struct {
	// PendingCreations counts accepted browser creations not yet resolved
	// through OnAfterCreated or a definitive failure.
	PendingCreations int
	// ActiveViews counts registered live views, including views whose
	// native close has started but whose GTK teardown is outstanding.
	ActiveViews int
	// PendingCleanup counts native-closed views whose queued GTK
	// bridge/popup cleanup has not completed.
	PendingCleanup int
	// ActiveDownloads counts downloads with a terminal event outstanding.
	ActiveDownloads int
}

// Quiescent reports whether no native work is outstanding.
func (s RuntimeActivitySnapshot) Quiescent() bool {
	return s.PendingCreations == 0 && s.ActiveViews == 0 && s.PendingCleanup == 0 && s.ActiveDownloads == 0
}

// RuntimeActivity is the application-port boundary for native runtime
// quiescence. The initial snapshot and subscription are race-safe and
// sequenced: Subscribe delivers the current snapshot synchronously, then all
// later transitions in order. Unsubscribe at shutdown.
type RuntimeActivity interface {
	Snapshot() RuntimeActivitySnapshot
	Subscribe(func(RuntimeActivitySnapshot)) (unsubscribe func())
}

// PersistenceDrain is the narrow activity/drain boundary for persistence
// owners whose saves cross the GTK/database boundary. Active covers pending
// debounce, in-flight capture/execution, and unsettled terminal failure.
// WaitSettled blocks until settled or ctx ends; it must never be called on
// the GTK thread when capture dispatches there.
type PersistenceDrain interface {
	Active() bool
	WaitSettled(ctx context.Context) error
}
