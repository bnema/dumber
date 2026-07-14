package gtkgl

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/bnema/purego-cef/cef"
	"github.com/bnema/purego-cef2gtk/internal/cefadapter"
	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
	"github.com/bnema/purego-cef2gtk/internal/egl"
	"github.com/bnema/purego-cef2gtk/internal/gl"
	internalprofile "github.com/bnema/purego-cef2gtk/internal/profile"
	"github.com/bnema/puregotk/v4/gtk"
)

var (
	ErrRendererNotInitialized = errors.New("accelerated renderer not initialized")
	ErrNilAcceleratedRenderer = errors.New("nil accelerated renderer")
)

type eglImporter interface {
	ImportDMABUF(dmabuf.BorrowedFrame) (egl.Image, error)
	Destroy(egl.Image) error
}

type glImporter interface {
	gl.Driver
	ImportEGLImageToTexture(uintptr) (gl.Texture, error)
}

type glCloser interface {
	Close() error
}

type eglCloser interface {
	Close() error
}

type textureCopier interface {
	CopyImportedToOwned(gl.Texture, dmabuf.Size, gl.Texture) (gl.Texture, error)
	DrawTextureToCurrentFramebuffer(gl.Texture, dmabuf.Size) error
	Close()
}

// QueuedFrame is the internally queued owned texture produced by ImportCopyAndQueue.
type QueuedFrame struct {
	Texture gl.Texture
	Size    dmabuf.Size
}

// AcceleratedRenderer owns the internal DMABUF import/copy lifecycle. All
// methods must be called from the GTK main thread because it owns GtkGLArea and
// current-context GL/EGL state.
type AcceleratedRenderer struct {
	area        *gtk.GLArea
	egl         eglImporter
	gl          glImporter
	copier      textureCopier
	queued      QueuedFrame
	contextPtr  uintptr
	initFunc    func(*gtk.GLArea) (eglImporter, glImporter, textureCopier, error)
	profiler    atomic.Pointer[internalprofile.Recorder]
	copyTimer   *gl.TimerQueryRecorder
	drawTimer   *gl.TimerQueryRecorder
	frameTraces atomic.Uint64
}

func NewAcceleratedRenderer(area *gtk.GLArea) *AcceleratedRenderer {
	return &AcceleratedRenderer{area: area}
}

// SetFirstDMABUFTextureSwapHook reports that GLArea has no GtkPicture DMABUF
// texture-swap milestone. Its copied GL frame path must never claim one.
func (r *AcceleratedRenderer) SetFirstDMABUFTextureSwapHook(func()) bool {
	return false
}

// InitializeOnGTKThread initializes EGL/GL import and copy primitives with the
// GtkGLArea context current. It must be called on the GTK thread.
func (r *AcceleratedRenderer) InitializeOnGTKThread() error {
	if r == nil {
		return ErrNilAcceleratedRenderer
	}
	contextPtr := r.currentContextPointer()
	if r.readyForContext(contextPtr) {
		return nil
	}
	if r.contextPtr != 0 && contextPtr != 0 && r.contextPtr != contextPtr {
		// Gtk may recreate a GtkGLArea context when a widget is reparented or
		// unrealized/realized. Resource names from the old context are invalid in
		// the new context, so drop references without issuing GL deletes against the
		// wrong current context.
		r.discardContextResources()
	} else {
		r.Close()
	}
	init := r.initFunc
	if init == nil {
		init = defaultAcceleratedRendererInit
	}
	eglImporter, glBackend, copier, err := init(r.area)
	if err != nil {
		return err
	}
	r.egl = eglImporter
	r.gl = glBackend
	r.copier = copier
	r.contextPtr = contextPtr
	if tq, ok := glBackend.(gl.TimerQueryDriver); ok && tq.TimerQuerySupported() {
		r.copyTimer = gl.NewTimerQueryRecorder(tq)
		r.drawTimer = gl.NewTimerQueryRecorder(tq)
	} else {
		r.copyTimer = nil
		r.drawTimer = nil
	}
	return nil
}

func defaultAcceleratedRendererInit(area *gtk.GLArea) (eglImporter, glImporter, textureCopier, error) {
	probe, err := ProbeCurrentGLAreaContext(area)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := probe.Validate(); err != nil {
		return nil, nil, nil, err
	}
	eglImporter, err := egl.NewImporterFromCurrentDisplay()
	if err != nil {
		return nil, nil, nil, err
	}
	glBackend, err := gl.NewBackendFromCurrentContext()
	if err != nil {
		if closer, ok := any(eglImporter).(eglCloser); ok {
			_ = closer.Close()
		}
		return nil, nil, nil, err
	}
	copier, err := gl.NewTexturedQuadCopierForAPI(glBackend, probe.GLAPI)
	if err != nil {
		if closer, ok := any(eglImporter).(eglCloser); ok {
			_ = closer.Close()
		}
		_ = glBackend.Close()
		return nil, nil, nil, err
	}
	return eglImporter, glBackend, copier, nil
}

// ImportAndQueueOnGTKThread imports and copies callback-scoped CEF DMABUF
// metadata on the GTK main context before returning. OnAcceleratedPaint callers
// must not return to CEF until this method completes because CEF owns the DMABUF
// resources only for the duration of the callback.
func (r *AcceleratedRenderer) ImportAndQueueOnGTKThread(info *cef.AcceleratedPaintInfo) (queued QueuedFrame, retErr error) {
	start := time.Now()
	if err := RunOnGTKThreadSync(func() {
		if r == nil {
			retErr = ErrNilAcceleratedRenderer
			return
		}
		r.recordGTKWait(time.Since(start))
		defer func(begin time.Time) { r.recordImportCopyCPU(time.Since(begin)) }(time.Now())
		if r.area != nil {
			if !r.area.GetRealized() {
				retErr = ErrGLAreaNotRealized
				return
			}
			r.area.MakeCurrent()
			if gerr := r.area.GetError(); gerr != nil {
				retErr = fmt.Errorf("gtk gl area error: %s", glibErrorMessage(gerr))
				return
			}
		}
		if err := r.InitializeOnGTKThread(); err != nil {
			retErr = err
			return
		}
		queued, retErr = r.ImportCopyAndQueue(info)
	}); err != nil {
		retErr = err
		return
	}
	return queued, retErr
}

// ImportCopyAndQueue imports a CEF accelerated-paint DMABUF, binds it to a
// temporary GL texture, copies it to an owned texture, queues the owned texture,
// and destroys callback-scoped resources before returning. Call on the GTK main
// thread with the GtkGLArea context current.
func (r *AcceleratedRenderer) ImportCopyAndQueue(info *cef.AcceleratedPaintInfo) (queued QueuedFrame, retErr error) {
	if r == nil {
		return QueuedFrame{}, ErrNilAcceleratedRenderer
	}
	if r.egl == nil || r.gl == nil || r.copier == nil {
		return QueuedFrame{}, ErrRendererNotInitialized
	}
	frame, err := cefadapter.BorrowedFrameFromAcceleratedPaint(info)
	if err != nil {
		return QueuedFrame{}, err
	}
	r.traceFrame(frame)

	importStart := time.Now()
	image, err := r.egl.ImportDMABUF(frame)
	r.recordImportCPU(time.Since(importStart))
	if err != nil {
		return QueuedFrame{}, err
	}
	destroyed := false
	defer func() {
		if !destroyed {
			if err := r.egl.Destroy(image); err != nil && retErr == nil {
				retErr = fmt.Errorf("cleanup imported frame: %w", err)
			}
		}
	}()

	importStart = time.Now()
	imported, err := r.gl.ImportEGLImageToTexture(uintptr(image))
	r.recordImportCPU(time.Since(importStart))
	if err != nil {
		return QueuedFrame{}, err
	}
	defer func() {
		id := uint32(imported)
		r.gl.DeleteTextures(1, &id)
	}()

	copyStart := time.Now()
	copyQuery, copyQueryOK := r.beginTimer(r.copyTimer)
	owned, err := r.copier.CopyImportedToOwned(imported, frame.CodedSize, 0)
	r.endTimer(r.copyTimer, copyQuery, copyQueryOK)
	r.recordCopyCPU(time.Since(copyStart))
	r.collectCopyGPU()
	if err != nil {
		return QueuedFrame{}, err
	}
	if err := r.egl.Destroy(image); err != nil {
		destroyed = true
		ownedID := uint32(owned)
		r.gl.DeleteTextures(1, &ownedID)
		return QueuedFrame{}, fmt.Errorf("cleanup imported frame: %w", err)
	}
	destroyed = true
	if r.queued.Texture != 0 {
		old := uint32(r.queued.Texture)
		r.gl.DeleteTextures(1, &old)
	}
	r.queued = QueuedFrame{Texture: owned, Size: frame.CodedSize}
	return r.queued, nil
}

func (r *AcceleratedRenderer) QueuedFrame() (QueuedFrame, bool) {
	if r == nil || r.queued.Texture == 0 {
		return QueuedFrame{}, false
	}
	return r.queued, true
}

func (r *AcceleratedRenderer) traceFrame(frame dmabuf.BorrowedFrame) {
	if os.Getenv("PUREGO_CEF2GTK_GL_TRACE") == "" || r == nil || r.frameTraces.Add(1) > 8 {
		return
	}
	fmt.Fprintf(os.Stderr,
		"cef2gtk-gl frame coded=%dx%d visible=%dx%d+%d+%d content=%dx%d+%d+%d source=%dx%d format=%s modifier=0x%x\n",
		frame.CodedSize.Width, frame.CodedSize.Height,
		frame.VisibleRect.Width, frame.VisibleRect.Height, frame.VisibleRect.X, frame.VisibleRect.Y,
		frame.ContentRect.Width, frame.ContentRect.Height, frame.ContentRect.X, frame.ContentRect.Y,
		frame.SourceSize.Width, frame.SourceSize.Height,
		frame.Format, frame.Modifier)
}

// QueueRender schedules the GtkGLArea render signal on the GTK main context.
func (r *AcceleratedRenderer) QueueRender() {
	if r == nil || r.area == nil {
		return
	}
	QueueRenderOnGTKThread(r.area)
}

// RenderQueuedOnGTKThread draws the current owned texture into the GtkGLArea framebuffer.
func (r *AcceleratedRenderer) RenderQueuedOnGTKThread() error {
	if r == nil {
		return ErrNilAcceleratedRenderer
	}
	if r.area != nil && !r.area.GetRealized() {
		r.discardContextResources()
		return nil
	}
	contextPtr := r.currentContextPointer()
	if r.contextPtr != 0 && contextPtr != 0 && r.contextPtr != contextPtr {
		r.discardContextResources()
		return nil
	}
	queued, ok := r.QueuedFrame()
	if !ok {
		return nil
	}
	if r.copier == nil {
		return ErrRendererNotInitialized
	}
	drawStart := time.Now()
	drawQuery, drawQueryOK := r.beginTimer(r.drawTimer)
	err := r.copier.DrawTextureToCurrentFramebuffer(queued.Texture, queued.Size)
	r.endTimer(r.drawTimer, drawQuery, drawQueryOK)
	r.recordRenderCPU(time.Since(drawStart))
	r.collectDrawGPU()
	return err
}

func (r *AcceleratedRenderer) currentContextPointer() uintptr {
	if r == nil || r.area == nil || !r.area.GetRealized() {
		return 0
	}
	ctx := r.area.GetContext()
	if ctx == nil {
		return 0
	}
	return ctx.GoPointer()
}

func (r *AcceleratedRenderer) readyForContext(contextPtr uintptr) bool {
	return r != nil && r.egl != nil && r.gl != nil && r.copier != nil && r.contextPtr != 0 && r.contextPtr == contextPtr
}

// InvalidateOnGTKThread drops renderer state after GtkGLArea unrealize/context
// loss. Call on the GTK thread. No GL deletes are issued because the old context
// may already be gone; the next import will recreate resources for the current
// context.
func (r *AcceleratedRenderer) InvalidateOnGTKThread() {
	r.discardContextResources()
}

func (r *AcceleratedRenderer) discardContextResources() {
	if r == nil {
		return
	}
	r.queued = QueuedFrame{}
	// Do not delete timer queries here for the same reason copier.Close is avoided:
	// they are GL object names owned by the context that may already be gone.
	r.copyTimer = nil
	r.drawTimer = nil
	// Do not call copier.Close here: it deletes VBO/VAO/program names that were
	// created in the old GL context. On context loss those names are invalid in
	// the new/current context and are reclaimed by the driver with the old one.
	r.copier = nil
	if r.gl != nil {
		if closer, ok := r.gl.(glCloser); ok {
			_ = closer.Close()
		}
		r.gl = nil
	}
	if r.egl != nil {
		if closer, ok := r.egl.(eglCloser); ok {
			_ = closer.Close()
		}
		r.egl = nil
	}
	r.contextPtr = 0
}

func (r *AcceleratedRenderer) SetProfiler(profiler *internalprofile.Recorder) {
	if r == nil {
		return
	}
	r.profiler.Store(profiler)
}

func (r *AcceleratedRenderer) beginTimer(timer *gl.TimerQueryRecorder) (gl.TimerQuery, bool) {
	if timer == nil {
		return 0, false
	}
	return timer.Begin()
}

func (r *AcceleratedRenderer) endTimer(timer *gl.TimerQueryRecorder, query gl.TimerQuery, ok bool) {
	if timer == nil || !ok {
		return
	}
	timer.End(query)
}

func (r *AcceleratedRenderer) profileRecorder() *internalprofile.Recorder {
	if r == nil {
		return nil
	}
	return r.profiler.Load()
}

func (r *AcceleratedRenderer) collectCopyGPU() {
	profiler := r.profileRecorder()
	if profiler == nil || r.copyTimer == nil {
		return
	}
	for _, d := range r.copyTimer.Collect() {
		profiler.RecordCopyGPU(d)
	}
}

func (r *AcceleratedRenderer) collectDrawGPU() {
	profiler := r.profileRecorder()
	if profiler == nil || r.drawTimer == nil {
		return
	}
	for _, d := range r.drawTimer.Collect() {
		profiler.RecordDrawGPU(d)
	}
}

func (r *AcceleratedRenderer) recordGTKWait(d time.Duration) {
	if profiler := r.profileRecorder(); profiler != nil {
		profiler.RecordGTKWait(d)
	}
}

func (r *AcceleratedRenderer) recordImportCopyCPU(d time.Duration) {
	if profiler := r.profileRecorder(); profiler != nil {
		profiler.RecordImportCopyCPU(d)
	}
}

func (r *AcceleratedRenderer) recordImportCPU(d time.Duration) {
	if profiler := r.profileRecorder(); profiler != nil {
		profiler.RecordImportCPU(d)
	}
}

func (r *AcceleratedRenderer) recordCopyCPU(d time.Duration) {
	if profiler := r.profileRecorder(); profiler != nil {
		profiler.RecordCopyCPU(d)
	}
}

func (r *AcceleratedRenderer) recordRenderCPU(d time.Duration) {
	if profiler := r.profileRecorder(); profiler != nil {
		profiler.RecordRenderCPU(d)
	}
}

func (r *AcceleratedRenderer) Close() {
	if r == nil {
		return
	}
	if r.copyTimer != nil {
		r.copyTimer.Close()
		r.copyTimer = nil
	}
	if r.drawTimer != nil {
		r.drawTimer.Close()
		r.drawTimer = nil
	}
	if r.queued.Texture != 0 && r.gl != nil {
		id := uint32(r.queued.Texture)
		r.gl.DeleteTextures(1, &id)
		r.queued = QueuedFrame{}
	}
	if r.copier != nil {
		r.copier.Close()
		r.copier = nil
	}
	if r.gl != nil {
		if closer, ok := r.gl.(glCloser); ok {
			_ = closer.Close()
		}
		r.gl = nil
	}
	if r.egl != nil {
		if closer, ok := r.egl.(eglCloser); ok {
			_ = closer.Close()
		}
		r.egl = nil
	}
	r.contextPtr = 0
}
