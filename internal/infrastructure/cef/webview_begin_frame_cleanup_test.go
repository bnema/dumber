package cef

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
)

func TestStopBeginFrameLoopReleasesCallbackSlotWithoutWidget(t *testing.T) {
	callback := new(gtk.TickCallback)
	wv := &WebView{beginFrameTick: callback, beginFrameTickID: 17}
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

	wv.stopBeginFrameLoop()
	wv.stopBeginFrameLoop()

	if wv.beginFrameTick != nil || wv.beginFrameTickID != 0 {
		t.Fatal("stop must release the BeginFrame callback slot even after the GTK widget is gone")
	}
	if unrefs != 1 {
		t.Fatalf("callback unrefs = %d, want 1", unrefs)
	}
}
