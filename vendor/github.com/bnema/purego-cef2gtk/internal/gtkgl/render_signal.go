package gtkgl

import (
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

// QueueRenderOnGTKThread schedules GtkGLArea.QueueRender on the default GTK main context.
func QueueRenderOnGTKThread(area *gtk.GLArea) {
	if area == nil {
		return
	}
	fn := glib.SourceOnceFunc(func(uintptr) {
		area.QueueRender()
	})
	glib.IdleAddOnce(&fn, 0)
}
