package cef

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
)

func TestStopBeginFrameLoopReleasesCallbackSlotWithoutWidget(t *testing.T) {
	callback := new(gtk.TickCallback)
	wv := &WebView{beginFrameTick: callback, beginFrameTickID: 17}

	wv.stopBeginFrameLoop()

	if wv.beginFrameTick != nil || wv.beginFrameTickID != 0 {
		t.Fatal("stop must release the BeginFrame callback slot even after the GTK widget is gone")
	}
}
