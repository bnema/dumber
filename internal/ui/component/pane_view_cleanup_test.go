package component

import "testing"

func TestPaneViewCleanupClearsMouseMotionCallback(t *testing.T) {
	pv := &PaneView{onMouseMotion: func() { t.Fatal("stale callback invoked") }}

	pv.Cleanup()

	if pv.onMouseMotion != nil {
		t.Fatal("Cleanup must release the mouse-motion callback slot")
	}
}
