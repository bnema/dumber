// Package gtkgdk renders CEF accelerated-paint DMABUF frames through GDK textures.
package gtkgdk

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/bnema/purego"
	"github.com/bnema/purego-cef/cef"
	"github.com/bnema/purego-cef2gtk/internal/cefadapter"
	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
	"github.com/bnema/purego-cef2gtk/internal/gtkgl"
	internalprofile "github.com/bnema/purego-cef2gtk/internal/profile"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	retiredTextureLimit   = 16
	stalePendingFrameWait = 250 * time.Millisecond
)

var (
	ErrNilRenderer                   = errors.New("nil gdk dmabuf renderer")
	ErrMissingPicture                = errors.New("missing gtk picture")
	ErrNoDisplay                     = errors.New("gdk display unavailable")
	ErrBuilderUnavailable            = errors.New("gdk dmabuf texture builder unavailable")
	ErrDmabufFormatsUnavailable      = errors.New("gdk dmabuf formats unavailable")
	ErrUnsupportedFormat             = errors.New("unsupported gdk dmabuf format")
	ErrTextureBuildFailed            = errors.New("gdk dmabuf texture build failed")
	ErrCloseDestroyNotifyUnavailable = errors.New("native close destroy notify unavailable")
)

type dmabufFormatSet interface {
	Contains(uint32, uint64) bool
}

// Diagnostics is a point-in-time snapshot of GDK DMABUF renderer counters.
type Diagnostics struct {
	TexturesBuilt           uint64
	TextureBuildFailures    uint64
	FDDupFailures           uint64
	UnsupportedFormats      uint64
	PaintableSwaps          uint64
	PendingFrame            bool
	PendingScheduled        bool
	PendingAge              time.Duration
	PendingSourceID         uint
	PendingReschedules      uint64
	PendingScheduleFailures uint64
	PendingIdleCallbacks    uint64
}

type dmabufTextureBuilder interface {
	SetDisplay(*gdk.Display)
	SetWidth(uint)
	SetHeight(uint)
	SetFourcc(uint32)
	SetModifier(uint64)
	SetPremultiplied(bool)
	SetNPlanes(uint)
	SetFd(uint, int)
	SetStride(uint, uint)
	SetOffset(uint, uint)
	BuildWithDestroyNotifyPointer(uintptr, uintptr) (*gdk.Texture, error)
}

type idleOnceScheduler func(*glib.SourceOnceFunc, uintptr) uint

// ownedTexture pairs a GdkTexture whose plane FD lifetime is managed by GDK's
// native close(2) GDestroyNotify.
type ownedTexture struct {
	texture *gdk.Texture
}

type ownedPlane struct {
	FD     int
	Stride uint32
	Offset uint64
	Size   uint64
}

type ownedFrame struct {
	CodedSize   dmabuf.Size
	VisibleRect dmabuf.Rect
	ContentRect dmabuf.Rect
	SourceSize  dmabuf.Size
	Format      dmabuf.FourCC
	Modifier    uint64
	Plane       ownedPlane
	onError     func(error)
}

// Renderer owns a GtkPicture presenter and imports callback-scoped CEF DMABUFs
// as GdkDmabufTexture instances. The initial implementation supports only
// single-plane RGB frames. Duplicated FDs are handed to GTK with a native
// close(2) destroy notify so they stay open until the GdkTexture is finalized.
type Renderer struct {
	widget  *gtk.Widget
	picture *gtk.Picture
	offload *gtk.GraphicsOffload

	display             *gdk.Display
	formats             dmabufFormatSet
	builder             dmabufTextureBuilder
	current             *ownedTexture
	retired             [retiredTextureLimit]*ownedTexture
	retiredStart        int
	retiredCount        int
	pictureSetPaintable func(*gdk.Texture)
	firstTextureSwapMu  sync.Mutex
	firstTextureSwap    func()
	firstTextureSwapped bool

	pendingMu          sync.Mutex
	pendingFrame       *ownedFrame
	pendingScheduled   bool
	pendingScheduledAt time.Time
	pendingSourceID    uint
	pendingGeneration  uint64

	dupFD       func(int) (int, error)
	closeFD     func(int) error
	idleAddOnce idleOnceScheduler

	texturesBuilt           atomic.Uint64
	textureBuildFailures    atomic.Uint64
	fdDupFailures           atomic.Uint64
	unsupportedFormats      atomic.Uint64
	paintableSwaps          atomic.Uint64
	pendingReschedules      atomic.Uint64
	pendingScheduleFailures atomic.Uint64
	pendingIdleCallbacks    atomic.Uint64
	frameTraces             atomic.Uint64
	textureTraces           atomic.Uint64

	profiler atomic.Pointer[internalprofile.Recorder]
}

var (
	closeDestroyNotifyOnce sync.Once
	closeDestroyNotifyPtr  uintptr
	closeDestroyNotifyErr  error
)

// resolveDestroyNotify is overridable in tests to simulate native close failure.
var resolveDestroyNotify = nativeCloseDestroyNotify

type presenterPicture interface {
	SetCanShrink(bool)
	SetContentFit(gtk.ContentFit)
	SetHexpand(bool)
	SetVexpand(bool)
	SetSizeRequest(int, int)
}

func configurePresenterPicture(picture presenterPicture) {
	if picture == nil {
		return
	}
	picture.SetCanShrink(true)
	picture.SetContentFit(gtk.ContentFitFillValue)
	picture.SetHexpand(true)
	picture.SetVexpand(true)
	picture.SetSizeRequest(1, 1)
}

// NewRenderer creates a GtkPicture-backed GDK DMABUF renderer. When useOffload
// is true and GtkGraphicsOffload can be constructed, Widget returns the offload
// wrapper; otherwise it returns the picture widget directly.
func NewRenderer(useOffload bool) (*Renderer, error) {
	picture := gtk.NewPicture()
	if picture == nil {
		return nil, ErrMissingPicture
	}
	configurePresenterPicture(picture)

	widget := &picture.Widget
	var offload *gtk.GraphicsOffload
	if useOffload {
		offload = gtk.NewGraphicsOffload(widget)
		if offload != nil {
			offload.SetEnabled(gtk.GraphicsOffloadEnabledValue)
			offload.SetHexpand(true)
			offload.SetVexpand(true)
			offload.SetSizeRequest(1, 1)
			widget = &offload.Widget
		}
	}

	builder, err := newTextureBuilder()
	if err != nil {
		return nil, err
	}

	// Validate native close(2) GDestroyNotify availability at construction time
	// so BackendAuto callers can detect the failure and fall back to the GLArea
	// renderer without waiting for the first frame to fail.
	if _, err := resolveDestroyNotify(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCloseDestroyNotifyUnavailable, err)
	}

	return &Renderer{
		widget:      widget,
		picture:     picture,
		offload:     offload,
		builder:     builder,
		dupFD:       dupFDClOExec,
		closeFD:     unix.Close,
		idleAddOnce: glib.IdleAddOnce,
	}, nil
}

func newTextureBuilder() (builder dmabufTextureBuilder, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			builder = nil
			retErr = fmt.Errorf("%w: %v", ErrBuilderUnavailable, r)
		}
	}()
	builder = gdk.NewDmabufTextureBuilder()
	if builder == nil {
		return nil, ErrBuilderUnavailable
	}
	return builder, nil
}

func nativeCloseDestroyNotify() (uintptr, error) {
	closeDestroyNotifyOnce.Do(func() {
		if !nativeCloseDestroyNotifyABICompatible() {
			closeDestroyNotifyErr = ErrCloseDestroyNotifyUnavailable
			return
		}
		closeDestroyNotifyPtr, closeDestroyNotifyErr = purego.Dlsym(purego.RTLD_DEFAULT, "close")
		if closeDestroyNotifyErr != nil {
			closeDestroyNotifyErr = fmt.Errorf("%w: %w", ErrCloseDestroyNotifyUnavailable, closeDestroyNotifyErr)
			return
		}
		if closeDestroyNotifyPtr == 0 {
			closeDestroyNotifyErr = ErrCloseDestroyNotifyUnavailable
		}
	})
	return closeDestroyNotifyPtr, closeDestroyNotifyErr
}

func nativeCloseDestroyNotifyABICompatible() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	switch runtime.GOARCH {
	case "amd64", "arm64", "386", "arm":
		return true
	default:
		return false
	}
}

// Widget returns the widget that should be packed into GTK containers.
func (r *Renderer) Widget() *gtk.Widget {
	if r == nil {
		return nil
	}
	return r.widget
}

// SetFirstDMABUFTextureSwapHook registers the one-shot callback invoked after
// GtkPicture.SetPaintable succeeds for the first imported DMABUF texture.
func (r *Renderer) SetFirstDMABUFTextureSwapHook(fn func()) bool {
	if r == nil {
		return false
	}
	r.firstTextureSwapMu.Lock()
	r.firstTextureSwap = fn
	r.firstTextureSwapMu.Unlock()
	return true
}

func (r *Renderer) recordFirstDMABUFTextureSwap() {
	if r == nil {
		return
	}
	r.firstTextureSwapMu.Lock()
	if r.firstTextureSwapped {
		r.firstTextureSwapMu.Unlock()
		return
	}
	r.firstTextureSwapped = true
	fn := r.firstTextureSwap
	r.firstTextureSwapMu.Unlock()
	if fn != nil {
		fn()
	}
}

// Picture returns the GtkPicture used as the paintable presenter.
func (r *Renderer) Picture() *gtk.Picture {
	if r == nil {
		return nil
	}
	return r.picture
}

// InitializeOnGTKThread resolves the GDK display and builder on the GTK thread.
func (r *Renderer) InitializeOnGTKThread() error {
	if r == nil {
		return ErrNilRenderer
	}
	if r.builder == nil {
		builder, err := newTextureBuilder()
		if err != nil {
			return err
		}
		r.builder = builder
	}
	if r.display != nil {
		return nil
	}
	if r.widget != nil {
		r.display = r.widget.GetDisplay()
	}
	if r.display == nil {
		r.display = gdk.DisplayGetDefault()
	}
	if r.display == nil {
		return ErrNoDisplay
	}
	if r.formats == nil {
		r.formats = r.display.GetDmabufFormats()
	}
	if r.formats == nil {
		return ErrDmabufFormatsUnavailable
	}
	return nil
}

// ImportAndQueueAsync duplicates callback-scoped CEF DMABUF FDs before the CEF
// callback returns, then posts the GDK texture import/swap to the GTK main
// thread. This mirrors Dumber's old OnPaint staging model: CEF never waits for
// GTK, and GTK always consumes an owned, callback-independent frame.
func (r *Renderer) ImportAndQueueAsync(info *cef.AcceleratedPaintInfo, onError func(error)) error {
	if r == nil {
		return ErrNilRenderer
	}
	frame, err := cefadapter.SinglePlaneFrameFromAcceleratedPaint(info)
	if err != nil {
		return err
	}
	owned, err := r.duplicateSinglePlaneFrame(frame)
	if err != nil {
		return err
	}
	owned.onError = onError
	r.enqueueOwnedFrame(owned)
	return nil
}

// ImportAndQueueOnGTKThread is kept for the GLArea backend contract and tests.
// Prefer ImportAndQueueAsync for GDK DMABUF so CEF paint callbacks do not block
// on the GTK main loop.
func (r *Renderer) ImportAndQueueOnGTKThread(info *cef.AcceleratedPaintInfo) (queued gtkgl.QueuedFrame, retErr error) {
	start := time.Now()
	frame, err := cefadapter.SinglePlaneFrameFromAcceleratedPaint(info)
	if err != nil {
		return gtkgl.QueuedFrame{}, err
	}
	owned, err := r.duplicateSinglePlaneFrame(frame)
	if err != nil {
		return gtkgl.QueuedFrame{}, err
	}
	if err := gtkgl.RunOnGTKThreadSync(func() {
		if r == nil {
			retErr = ErrNilRenderer
			return
		}
		r.recordGTKWait(time.Since(start))
		defer func(begin time.Time) { r.recordImportCopyCPU(time.Since(begin)) }(time.Now())
		retErr = r.importAndSwapOwnedFrame(owned)
	}); err != nil {
		retErr = err
	}
	if retErr != nil {
		r.releaseOwnedFrame(owned)
	}
	return gtkgl.QueuedFrame{}, retErr
}

func (r *Renderer) enqueueOwnedFrame(frame *ownedFrame) {
	if frame == nil {
		return
	}
	if r == nil {
		closeOwnedFrame(frame, nil)
		return
	}
	now := time.Now()
	r.pendingMu.Lock()
	if r.pendingFrame != nil {
		r.releaseOwnedFrame(r.pendingFrame)
	}
	r.pendingFrame = frame
	schedule := !r.pendingScheduled
	if !schedule && !r.pendingScheduledAt.IsZero() && now.Sub(r.pendingScheduledAt) > stalePendingFrameWait {
		schedule = true
		r.pendingReschedules.Add(1)
	}
	generation := r.pendingGeneration
	if schedule {
		r.pendingScheduled = true
		r.pendingScheduledAt = now
		r.pendingSourceID = 0
		r.pendingGeneration++
		generation = r.pendingGeneration
	}
	r.pendingMu.Unlock()
	if !schedule {
		return
	}
	r.schedulePendingImport(generation)
}

func (r *Renderer) schedulePendingImport(generation uint64) {
	if r == nil {
		return
	}
	cb := glib.SourceOnceFunc(func(uintptr) {
		r.pendingIdleCallbacks.Add(1)
		r.importPendingFrameOnGTKThread()
	})
	scheduler := r.idleAddOnce
	if scheduler == nil {
		scheduler = glib.IdleAddOnce
	}
	sourceID := scheduler(&cb, 0)
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	if !r.pendingScheduled || r.pendingGeneration != generation {
		return
	}
	if sourceID == 0 {
		r.pendingScheduleFailures.Add(1)
		r.pendingScheduled = false
		r.pendingScheduledAt = time.Time{}
		r.pendingSourceID = 0
		r.pendingGeneration++
		return
	}
	r.pendingSourceID = sourceID
}

func (r *Renderer) importPendingFrameOnGTKThread() {
	if r == nil {
		return
	}
	r.pendingMu.Lock()
	frame := r.pendingFrame
	r.pendingFrame = nil
	r.pendingScheduled = false
	r.pendingScheduledAt = time.Time{}
	r.pendingSourceID = 0
	r.pendingGeneration++
	r.pendingMu.Unlock()
	if frame == nil {
		return
	}
	start := time.Now()
	defer func(begin time.Time) { r.recordImportCopyCPU(time.Since(begin)) }(start)
	if err := r.importAndSwapOwnedFrame(frame); err != nil {
		r.releaseOwnedFrame(frame)
		if frame.onError != nil {
			frame.onError(err)
		}
	}
}

func (r *Renderer) importAndSwapOwnedFrame(frame *ownedFrame) error {
	if r == nil {
		return ErrNilRenderer
	}
	if err := r.InitializeOnGTKThread(); err != nil {
		return err
	}
	built, err := r.buildTextureFromOwnedFrame(frame)
	if err != nil {
		return err
	}
	if built == nil || built.texture == nil || built.texture.GoPointer() == 0 {
		if built != nil {
			r.releaseOwnedTexture(built)
		}
		return ErrTextureBuildFailed
	}

	if r.picture == nil {
		r.releaseOwnedTexture(r.current)
		r.current = nil
		r.releaseOwnedTexture(built)
		return ErrMissingPicture
	}
	old := r.current
	if r.pictureSetPaintable != nil {
		r.pictureSetPaintable(built.texture)
	} else {
		r.picture.SetPaintable(built.texture)
	}
	r.current = built
	r.recordPaintableSwap()
	r.recordFirstDMABUFTextureSwap()
	r.retireOwnedTexture(old)
	return nil
}

// duplicateFrame keeps the slice-backed compatibility path for callers outside
// the GDK import hot path.
func (r *Renderer) duplicateFrame(frame dmabuf.BorrowedFrame) (*ownedFrame, error) {
	if err := frame.Validate(); err != nil {
		return nil, err
	}
	return r.duplicateSinglePlaneFrame(dmabuf.SinglePlaneFrame{
		CodedSize:   frame.CodedSize,
		VisibleRect: frame.VisibleRect,
		ContentRect: frame.ContentRect,
		SourceSize:  frame.SourceSize,
		Format:      frame.Format,
		Modifier:    frame.Modifier,
		Plane:       frame.Planes[0],
	})
}

func (r *Renderer) duplicateSinglePlaneFrame(frame dmabuf.SinglePlaneFrame) (_ *ownedFrame, retErr error) {
	if r == nil {
		return nil, ErrNilRenderer
	}
	if err := frame.Validate(); err != nil {
		return nil, err
	}
	dup := r.dupFD
	if dup == nil {
		dup = dupFDClOExec
	}
	owned := &ownedFrame{
		CodedSize:   frame.CodedSize,
		VisibleRect: frame.VisibleRect,
		ContentRect: frame.ContentRect,
		SourceSize:  frame.SourceSize,
		Format:      frame.Format,
		Modifier:    frame.Modifier,
		Plane:       ownedPlane{FD: -1},
	}
	defer func() {
		if retErr != nil {
			r.releaseOwnedFrame(owned)
		}
	}()
	ownedFD, err := dup(frame.Plane.FD)
	if err != nil {
		r.recordFDDupFailure()
		return nil, err
	}
	owned.Plane = ownedPlane{FD: ownedFD, Stride: frame.Plane.Stride, Offset: frame.Plane.Offset, Size: frame.Plane.Size}
	return owned, nil
}

func (r *Renderer) buildTextureFromOwnedFrame(frame *ownedFrame) (built *ownedTexture, retErr error) {
	if r == nil {
		return nil, ErrNilRenderer
	}
	if r.builder == nil {
		return nil, ErrBuilderUnavailable
	}
	if frame == nil {
		return nil, dmabuf.ErrInvalidPlaneFD
	}
	if err := frame.validate(); err != nil {
		return nil, err
	}
	gdkFormat := gdkTextureFormat(frame.Format)
	if r.formats != nil && !r.formats.Contains(uint32(gdkFormat), frame.Modifier) {
		r.recordUnsupportedFormat()
		return nil, fmt.Errorf("%w: %s as %s modifier 0x%x", ErrUnsupportedFormat, frame.Format, gdkFormat, frame.Modifier)
	}
	r.traceFrame(frame, gdkFormat)
	closeFD := r.closeFD
	if closeFD == nil {
		closeFD = unix.Close
	}

	ownedFD := frame.Plane.FD
	textureOwnsFD := false
	defer func() {
		if !textureOwnsFD {
			_ = closeFD(ownedFD)
			frame.Plane.FD = -1
		}
	}()
	destroyNotify, destroyNotifyErr := resolveDestroyNotify()
	if destroyNotifyErr != nil {
		r.recordTextureBuildFailure()
		return nil, fmt.Errorf("%w: native close GDestroyNotify unavailable: %w", ErrTextureBuildFailed, destroyNotifyErr)
	}

	plane := frame.Plane
	r.builder.SetWidth(uint(frame.CodedSize.Width))
	r.builder.SetHeight(uint(frame.CodedSize.Height))
	r.builder.SetFourcc(uint32(gdkFormat))
	r.builder.SetModifier(frame.Modifier)
	r.builder.SetPremultiplied(false)
	r.builder.SetNPlanes(1)
	if r.display != nil {
		r.builder.SetDisplay(r.display)
	}
	r.builder.SetFd(0, ownedFD)
	r.builder.SetStride(0, uint(plane.Stride))
	r.builder.SetOffset(0, uint(plane.Offset))

	defer func() {
		if p := recover(); p != nil {
			r.recordTextureBuildFailure()
			built = nil
			retErr = fmt.Errorf("%w: %v", ErrTextureBuildFailed, p)
		}
	}()
	// GDK requires the caller to keep plane FDs open until the returned texture is
	// released. Pass libc close(2) as a native GDestroyNotify so GTK/GSK closes
	// the duplicated FD at the exact texture-finalization point, without calling
	// back into Go from renderer/finalizer code. This relies on the standard C ABI
	// convention GLib documents for destroy callbacks: the gpointer data value is
	// passed in the first integer/pointer argument register, and close(2) reads
	// the low int fd from that register. If the native symbol or ABI compatibility
	// check is unavailable, texture construction fails and the caller can fall
	// back to the GLArea rendering backend.
	texture, err := r.builder.BuildWithDestroyNotifyPointer(destroyNotify, uintptr(ownedFD))
	if err != nil {
		r.recordTextureBuildFailure()
		return nil, fmt.Errorf("%w: %v", ErrTextureBuildFailed, err)
	}
	if texture == nil || texture.GoPointer() == 0 {
		r.recordTextureBuildFailure()
		return nil, ErrTextureBuildFailed
	}
	textureOwnsFD = true
	frame.Plane.FD = -1
	r.recordTextureBuilt()
	r.traceTexturePresent(frame, gdkFormat, texture)
	return &ownedTexture{texture: texture}, nil
}

// QueueRender is a no-op for GtkPicture; GTK schedules painting for paintable changes.
func (r *Renderer) QueueRender() {}

// RenderQueuedOnGTKThread is a no-op for GtkPicture; GTK/GSK renders the paintable.
func (r *Renderer) RenderQueuedOnGTKThread() error { return nil }

// InvalidateOnGTKThread drops the renderer's current texture reference without
// requiring a GL context. The GtkPicture may still hold its own paintable ref.
func (r *Renderer) InvalidateOnGTKThread() {
	if r == nil {
		return
	}
	r.pendingMu.Lock()
	pending := r.pendingFrame
	r.pendingFrame = nil
	r.pendingScheduled = false
	r.pendingScheduledAt = time.Time{}
	r.pendingSourceID = 0
	r.pendingGeneration++
	r.pendingMu.Unlock()
	r.releaseOwnedFrame(pending)
	r.releaseOwnedTexture(r.current)
	r.current = nil
	r.releaseRetiredTextures()
	r.display = nil
	r.formats = nil
}

func (r *Renderer) releaseOwnedFrame(frame *ownedFrame) {
	if frame == nil {
		return
	}
	var closeFD func(int) error
	if r != nil {
		closeFD = r.closeFD
	}
	closeOwnedFrame(frame, closeFD)
}

func (f *ownedFrame) validate() error {
	if f == nil {
		return dmabuf.ErrInvalidPlaneFD
	}
	return (dmabuf.SinglePlaneFrame{
		CodedSize: f.CodedSize,
		Format:    f.Format,
		Plane: dmabuf.Plane{
			FD:     f.Plane.FD,
			Stride: f.Plane.Stride,
			Offset: f.Plane.Offset,
			Size:   f.Plane.Size,
		},
	}).Validate()
}

func closeOwnedFrame(frame *ownedFrame, closeFD func(int) error) {
	if frame == nil {
		return
	}
	if closeFD == nil {
		closeFD = unix.Close
	}
	if frame.Plane.FD >= 0 {
		_ = closeFD(frame.Plane.FD)
		frame.Plane.FD = -1
	}
}

func (r *Renderer) retireOwnedTexture(owned *ownedTexture) {
	if r == nil || owned == nil {
		return
	}
	if r.retiredCount == retiredTextureLimit {
		oldest := r.retired[r.retiredStart]
		r.retired[r.retiredStart] = owned
		r.retiredStart = (r.retiredStart + 1) % retiredTextureLimit
		r.releaseOwnedTexture(oldest)
		return
	}
	index := (r.retiredStart + r.retiredCount) % retiredTextureLimit
	r.retired[index] = owned
	r.retiredCount++
}

func (r *Renderer) retiredAt(offset int) *ownedTexture {
	if r == nil || offset < 0 || offset >= r.retiredCount {
		return nil
	}
	return r.retired[(r.retiredStart+offset)%retiredTextureLimit]
}

func (r *Renderer) releaseRetiredTextures() {
	if r == nil {
		return
	}
	for r.retiredCount > 0 {
		index := r.retiredStart
		r.releaseOwnedTexture(r.retired[index])
		r.retired[index] = nil
		r.retiredStart = (r.retiredStart + 1) % retiredTextureLimit
		r.retiredCount--
	}
	r.retiredStart = 0
}

func (r *Renderer) releaseOwnedTexture(owned *ownedTexture) {
	if r == nil || owned == nil {
		return
	}
	if owned.texture != nil {
		owned.texture.Unref()
		runtime.KeepAlive(owned.texture)
	}
}

func gdkTextureFormat(format dmabuf.FourCC) dmabuf.FourCC {
	// CEF accelerated OSR frames are page content, not intentionally transparent
	// UI layers. Import them as opaque for GSK so a zero/undefined alpha channel
	// cannot make valid content render as a black/transparent rectangle.
	switch format {
	case dmabuf.FormatARGB8888:
		return dmabuf.FormatXRGB8888
	case dmabuf.FormatABGR8888:
		return dmabuf.FormatXBGR8888
	default:
		return format
	}
}

func (r *Renderer) traceFrame(frame *ownedFrame, gdkFormat dmabuf.FourCC) {
	if os.Getenv("PUREGO_CEF2GTK_GDK_TRACE") == "" || r == nil || r.frameTraces.Add(1) > 8 {
		return
	}
	plane := frame.Plane
	fmt.Fprintf(os.Stderr,
		"cef2gtk-gdk-dmabuf frame coded=%dx%d visible=%dx%d+%d+%d content=%dx%d+%d+%d source=%dx%d cef_format=%s gdk_format=%s modifier=0x%x fd=%d stride=%d offset=%d size=%d\n",
		frame.CodedSize.Width, frame.CodedSize.Height,
		frame.VisibleRect.Width, frame.VisibleRect.Height, frame.VisibleRect.X, frame.VisibleRect.Y,
		frame.ContentRect.Width, frame.ContentRect.Height, frame.ContentRect.X, frame.ContentRect.Y,
		frame.SourceSize.Width, frame.SourceSize.Height,
		frame.Format, gdkFormat, frame.Modifier, plane.FD, plane.Stride, plane.Offset, plane.Size)
}

func (r *Renderer) traceTexturePresent(frame *ownedFrame, gdkFormat dmabuf.FourCC, texture *gdk.Texture) {
	if os.Getenv("PUREGO_CEF2GTK_GDK_TRACE") == "" || r == nil || r.textureTraces.Add(1) > 8 {
		return
	}
	textureWidth, textureHeight := 0, 0
	intrinsicWidth, intrinsicHeight := 0, 0
	if texture != nil {
		textureWidth, textureHeight = texture.GetWidth(), texture.GetHeight()
		intrinsicWidth, intrinsicHeight = texture.GetIntrinsicWidth(), texture.GetIntrinsicHeight()
	}
	widgetWidth, widgetHeight := 0, 0
	widgetAllocatedWidth, widgetAllocatedHeight := 0, 0
	widgetScaleFactor := 0
	surfaceScale := 1.0
	surfaceScaleFactor := 0
	surfaceWidth, surfaceHeight := 0, 0
	if r.widget != nil {
		widgetWidth, widgetHeight = r.widget.GetWidth(), r.widget.GetHeight()
		widgetAllocatedWidth, widgetAllocatedHeight = r.widget.GetAllocatedWidth(), r.widget.GetAllocatedHeight()
		widgetScaleFactor = r.widget.GetScaleFactor()
		if surface := rendererWidgetSurface(r.widget); surface != nil {
			surfaceScale = surface.GetScale()
			surfaceScaleFactor = surface.GetScaleFactor()
			surfaceWidth, surfaceHeight = surface.GetWidth(), surface.GetHeight()
		}
	}
	pictureWidth, pictureHeight := 0, 0
	pictureAllocatedWidth, pictureAllocatedHeight := 0, 0
	if r.picture != nil {
		pictureWidth, pictureHeight = r.picture.GetWidth(), r.picture.GetHeight()
		pictureAllocatedWidth, pictureAllocatedHeight = r.picture.GetAllocatedWidth(), r.picture.GetAllocatedHeight()
	}
	expectedSurfaceWidth := int(math.Ceil(float64(widgetAllocatedWidth) * surfaceScale))
	expectedSurfaceHeight := int(math.Ceil(float64(widgetAllocatedHeight) * surfaceScale))
	fmt.Fprintf(os.Stderr,
		"cef2gtk-gdk-dmabuf-present coded=%dx%d visible=%dx%d+%d+%d source=%dx%d texture=%dx%d intrinsic=%dx%d widget=%dx%d alloc=%dx%d picture=%dx%d picture_alloc=%dx%d surface=%dx%d surface_scale=%.3f surface_scale_factor=%d widget_scale_factor=%d expected_surface_pixels=%dx%d coded_to_expected=%.3fx%.3f texture_to_widget=%.3fx%.3f cef_format=%s gdk_format=%s content_fit=contain offload=%t\n",
		frame.CodedSize.Width, frame.CodedSize.Height,
		frame.VisibleRect.Width, frame.VisibleRect.Height, frame.VisibleRect.X, frame.VisibleRect.Y,
		frame.SourceSize.Width, frame.SourceSize.Height,
		textureWidth, textureHeight, intrinsicWidth, intrinsicHeight,
		widgetWidth, widgetHeight, widgetAllocatedWidth, widgetAllocatedHeight,
		pictureWidth, pictureHeight, pictureAllocatedWidth, pictureAllocatedHeight,
		surfaceWidth, surfaceHeight, surfaceScale, surfaceScaleFactor, widgetScaleFactor,
		expectedSurfaceWidth, expectedSurfaceHeight,
		ratioInt32ToInt(frame.CodedSize.Width, expectedSurfaceWidth), ratioInt32ToInt(frame.CodedSize.Height, expectedSurfaceHeight),
		ratioIntToInt(textureWidth, widgetAllocatedWidth), ratioIntToInt(textureHeight, widgetAllocatedHeight),
		frame.Format, gdkFormat, r.offload != nil)
}

func rendererWidgetSurface(widget *gtk.Widget) *gdk.Surface {
	if widget == nil {
		return nil
	}
	native := widget.GetNative()
	if native == nil {
		return nil
	}
	return native.GetSurface()
}

func ratioInt32ToInt(num int32, den int) float64 {
	return ratioFloat(float64(num), float64(den))
}

func ratioIntToInt(num, den int) float64 {
	return ratioFloat(float64(num), float64(den))
}

func ratioFloat(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den
}

// SetProfiler installs a development profile recorder.
func (r *Renderer) SetProfiler(p *internalprofile.Recorder) {
	if r == nil {
		return
	}
	r.profiler.Store(p)
}

// Close releases references owned by the renderer.
func (r *Renderer) Close() { r.InvalidateOnGTKThread() }

// Diagnostics returns GDK DMABUF renderer-specific counters.
func (r *Renderer) Diagnostics() Diagnostics {
	if r == nil {
		return Diagnostics{}
	}
	r.pendingMu.Lock()
	pendingFrame := r.pendingFrame != nil
	pendingScheduled := r.pendingScheduled
	pendingScheduledAt := r.pendingScheduledAt
	pendingSourceID := r.pendingSourceID
	r.pendingMu.Unlock()

	pendingAge := time.Duration(0)
	if pendingScheduled && !pendingScheduledAt.IsZero() {
		pendingAge = time.Since(pendingScheduledAt)
		if pendingAge < 0 {
			pendingAge = 0
		}
	}
	return Diagnostics{
		TexturesBuilt:           r.texturesBuilt.Load(),
		TextureBuildFailures:    r.textureBuildFailures.Load(),
		FDDupFailures:           r.fdDupFailures.Load(),
		UnsupportedFormats:      r.unsupportedFormats.Load(),
		PaintableSwaps:          r.paintableSwaps.Load(),
		PendingFrame:            pendingFrame,
		PendingScheduled:        pendingScheduled,
		PendingAge:              pendingAge,
		PendingSourceID:         pendingSourceID,
		PendingReschedules:      r.pendingReschedules.Load(),
		PendingScheduleFailures: r.pendingScheduleFailures.Load(),
		PendingIdleCallbacks:    r.pendingIdleCallbacks.Load(),
	}
}

func dupFDClOExec(fd int) (int, error) {
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return -1, err
	}
	return dup, nil
}

func (r *Renderer) profileRecorder() *internalprofile.Recorder {
	if r == nil {
		return nil
	}
	return r.profiler.Load()
}

func (r *Renderer) recordGTKWait(d time.Duration) {
	if p := r.profileRecorder(); p != nil {
		p.RecordGTKWait(d)
	}
}

func (r *Renderer) recordImportCopyCPU(d time.Duration) {
	if p := r.profileRecorder(); p != nil {
		p.RecordImportCopyCPU(d)
	}
}

func (r *Renderer) recordTextureBuilt() {
	r.texturesBuilt.Add(1)
	if p := r.profileRecorder(); p != nil {
		p.RecordTextureBuilt()
	}
}

func (r *Renderer) recordTextureBuildFailure() {
	r.textureBuildFailures.Add(1)
	if p := r.profileRecorder(); p != nil {
		p.RecordTextureBuildFailure()
	}
}

func (r *Renderer) recordFDDupFailure() {
	r.fdDupFailures.Add(1)
	if p := r.profileRecorder(); p != nil {
		p.RecordFDDupFailure()
	}
}

func (r *Renderer) recordUnsupportedFormat() {
	r.unsupportedFormats.Add(1)
	if p := r.profileRecorder(); p != nil {
		p.RecordUnsupportedFormat()
	}
}

func (r *Renderer) recordPaintableSwap() {
	r.paintableSwaps.Add(1)
	if p := r.profileRecorder(); p != nil {
		p.RecordPaintableSwap()
	}
}
