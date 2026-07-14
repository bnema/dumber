package ui

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
)

func TestStopFloatingResizeWatcherReleasesRawCallbackSlot(t *testing.T) {
	callback := new(gtk.TickCallback)
	session := &floatingWorkspaceSession{resizeTickCallback: callback, resizeTickID: 17, resizeWatcherActive: true}

	(&App{}).stopFloatingResizeWatcher(session)

	if session.resizeTickCallback != nil || session.resizeTickID != 0 || session.resizeWatcherActive {
		t.Fatal("stop must release the floating resize callback slot exactly once")
	}
}
