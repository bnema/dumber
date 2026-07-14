package ui

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
)

func TestStopFloatingResizeWatcherReleasesRawCallbackSlot(t *testing.T) {
	callback := new(gtk.TickCallback)
	session := &floatingWorkspaceSession{resizeTickCallback: callback, resizeTickID: 17, resizeWatcherActive: true}
	oldUnref := unrefTickCallback
	defer func() { unrefTickCallback = oldUnref }()
	var unrefs int
	unrefTickCallback = func(got any) error {
		if got != callback {
			t.Fatalf("unref callback = %p, want %p", got, callback)
		}
		unrefs++
		return nil
	}

	(&App{}).stopFloatingResizeWatcher(session)
	(&App{}).stopFloatingResizeWatcher(session)

	if session.resizeTickCallback != nil || session.resizeTickID != 0 || session.resizeWatcherActive {
		t.Fatal("stop must release the floating resize callback slot exactly once")
	}
	if unrefs != 1 {
		t.Fatalf("callback unrefs = %d, want 1", unrefs)
	}
}
