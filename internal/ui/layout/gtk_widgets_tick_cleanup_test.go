package layout

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
)

func TestGtkPanedReleaseTickCallbackUnrefsRawSlotExactlyOnce(t *testing.T) {
	callback := new(gtk.TickCallback)
	paned := &gtkPaned{tickCallbacks: map[uint]*gtk.TickCallback{7: callback}}
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

	paned.releaseTickCallback(7)
	paned.releaseTickCallback(7)

	if unrefs != 1 {
		t.Fatalf("callback unrefs = %d, want 1", unrefs)
	}
}
