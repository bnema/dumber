package ui

import (
	"testing"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

// watchFinalized reports, through the returned flag, when widget is finalized.
func watchFinalized(t *testing.T, widget *gtk.Widget) *bool {
	t.Helper()
	finalized := new(bool)
	notify := gobject.WeakNotify(func(_, _ uintptr) { *finalized = true })
	t.Cleanup(func() { _ = notify })
	widget.WeakRef(&notify, 0)
	return finalized
}

func TestGTKModeToasterViewReleasesWidgets(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()
	// Windows may only be created once the GApplication has started up;
	// registering emits the startup signal.
	registered, err := gtkApp.Register(nil)
	require.NoError(t, err)
	require.True(t, registered, "the test application registers")

	var view *gtkModeToasterView
	done := make(chan struct{})
	run := glib.SourceFunc(func(uintptr) bool {
		defer close(done)
		window := gtk.NewApplicationWindow(gtkApp)
		require.NotNil(t, window)
		overlay := gtk.NewOverlay()
		window.SetChild(&overlay.Widget)
		overlayFinalized := watchFinalized(t, &overlay.Widget)
		view = newGTKModeToasterView(overlay, window, modeToasterEvents{})
		require.NotNil(t, view)

		groups := []legendGroup{{Name: "SCROLL", Rows: []legendRow{{Action: "down", Keys: []string{"j"}, Desc: "Down"}}}}
		view.setLegend(groups)
		row := watchFinalized(t, view.rows["down"][0])
		keycap := watchFinalized(t, view.keycaps["down"][0])
		group := watchFinalized(t, &view.groupBoxes[0].Widget)

		view.setLegend(nil)
		require.True(t, *row, "a replaced legend row is released")
		require.True(t, *keycap, "a replaced legend keycap is released")
		require.True(t, *group, "a replaced legend group is released")

		view.setLegend(groups)
		view.layoutColumns(1)
		column := watchFinalized(t, &view.columnBoxes[0].Widget)
		view.layoutColumns(2)
		require.True(t, *column, "a column dropped by a relayout is released")

		pill := watchFinalized(t, &view.pill.Widget)
		body := watchFinalized(t, &view.root.Widget)
		label := watchFinalized(t, &view.pillLabel.Widget)
		lastRow := watchFinalized(t, view.rows["down"][0])
		view.destroy()
		require.True(t, *pill, "destroy releases the pill")
		require.True(t, *body, "destroy releases the legend body")
		require.True(t, *label, "destroy releases the pill label")
		require.True(t, *lastRow, "destroy releases the legend rows")

		frame := newModeFrame(overlay)
		require.NotNil(t, frame)
		border := watchFinalized(t, &frame.border.Widget)
		frame.destroy()
		require.True(t, *border, "destroy releases the mode frame border")

		// The window is never presented: destroying an unrealized window
		// crashes on Wayland, so drop the references instead (the same as
		// the global shortcut handler test). The overlay's last reference
		// goes with the window's child; the window itself stays on GTK's
		// toplevel list until destroyed, so its finalization is not asserted.
		window.SetChild(nil)
		overlay.Unref()
		require.True(t, *overlayFinalized, "the test releases its overlay")
		gtkApp.RemoveWindow(&window.Window)
		window.Unref()
		return false
	})
	glib.MainContextDefault().Invoke(&run, 0)
	<-done
}
