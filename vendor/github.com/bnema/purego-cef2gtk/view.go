package cef2gtk

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/purego-cef/cef"
	"github.com/bnema/purego-cef2gtk/internal/gtkgdk"
	"github.com/bnema/purego-cef2gtk/internal/gtkgl"
	internalprofile "github.com/bnema/purego-cef2gtk/internal/profile"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
)

var ErrNilView = errors.New("nil View")

const (
	scaleTraceEnvVar = "PUREGO_CEF2GTK_TRACE_SCALE"
	osrTraceEnvVar   = "PUREGO_CEF2GTK_TRACE_OSR"
)

// Hooks contains optional callbacks invoked by the public CEF render adapter.
type Hooks struct {
	OnUnsupportedPaint     func()
	OnError                func(error)
	OnTextSelectionChanged func(selectedText string, selectedRange *cef.Range)
}

type renderer interface {
	InitializeOnGTKThread() error
	ImportAndQueueOnGTKThread(*cef.AcceleratedPaintInfo) (gtkgl.QueuedFrame, error)
	QueueRender()
	RenderQueuedOnGTKThread() error
	InvalidateOnGTKThread()
	SetProfiler(*internalprofile.Recorder)
	Close()
}

// View is a GTK-backed CEF OSR view.
type View struct {
	backend                     Backend
	widget                      *gtk.Widget
	area                        *gtk.GLArea
	renderer                    renderer
	signalObject                *gobject.Object
	input                       *gtkgl.InputBridge
	inputWidget                 *gtk.Widget
	diag                        *diagnosticsRecorder
	destroyed                   atomic.Bool
	renderLifecycleMu           sync.RWMutex
	hooks                       Hooks
	handler                     *renderHandler
	renderFunc                  func(gtk.GLArea, uintptr) bool
	resizeFunc                  func(gtk.GLArea, int, int)
	mapFunc                     func(gtk.Widget)
	showFunc                    func(gtk.Widget)
	realizeFunc                 func(gtk.Widget)
	unrealizeFunc               func(gtk.Widget)
	surfaceLayoutFunc           func(gdk.Surface, int, int)
	surfaceWidthNotify          func(gobject.Object, *gobject.ParamSpec)
	surfaceHeightNotify         func(gobject.Object, *gobject.ParamSpec)
	surfaceScaleNotify          func(gobject.Object, *gobject.ParamSpec)
	sizeTickFunc                *gtk.TickCallback
	renderHandlerID             uint
	resizeHandlerID             uint
	mapHandlerID                uint
	showHandlerID               uint
	realizeHandlerID            uint
	unrealizeHandlerID          uint
	surfaceLayoutHandlerID      uint
	surfaceWidthHandlerID       uint
	surfaceHeightHandlerID      uint
	surfaceScaleHandlerID       uint
	surfaceScaleFactorHandlerID uint
	sizeTickID                  uint
	scaleNotify                 func(gobject.Object, *gobject.ParamSpec)
	scaleHandlerID              uint
	sizeTickSettler             sizeTickSettler
	sizeObservationSampleFunc   func() sizeObservationSample
	sizeTickRegistrar           func(*gtk.TickCallback) uint
	surfaceRefFunc              func(*gdk.Surface)
	surfaceUnrefFunc            func(*gdk.Surface)
	surface                     *gdk.Surface
	cachedWidth                 atomic.Int32
	cachedHeight                atomic.Int32
	scaleBits                   atomic.Uint64
	scaleMultiplierBits         atomic.Uint64
	inputScaleOverride          atomic.Uint64
	scaleTraceCount             atomic.Uint64
	osrTraceCount               atomic.Uint64
	sizeHooksMu                 sync.Mutex
	sizeHooks                   map[uint64]func(width, height int32)
	nextSizeHookID              uint64
	profileMu                   sync.Mutex
	profileEnabled              atomic.Bool
	profilePtr                  atomic.Pointer[internalprofile.Recorder]
	profile                     *internalprofile.Recorder
	profileOptions              ProfileOptions
}

// NewView creates an accelerated CEF view using the default Vulkan/GDK DMABUF
// render stack or the PUREGO_CEF2GTK_BACKEND override when it is set.
func NewView() *View {
	return NewViewWithOptions(ViewOptions{})
}

// NewViewWithOptions creates an accelerated CEF view with explicit options.
// When PUREGO_CEF2GTK_BACKEND is set, it intentionally overrides opts.Backend
// for diagnostics and deployment-level backend selection.
func NewViewWithOptions(opts ViewOptions) *View {
	opts, err := resolveViewOptions(opts)
	if err != nil {
		return nil
	}

	backend := opts.Backend
	scaleMultiplier := normalizeDeviceScale(opts.ScaleMultiplier)
	if backend == BackendAuto {
		if v := newGDKDMABUFView(opts.Profile, scaleMultiplier); v != nil {
			return v
		}
		backend = BackendGLArea
	}
	switch backend {
	case BackendGLArea:
		return newGLAreaView(opts.Profile, scaleMultiplier)
	case BackendGDKDMABUF:
		return newGDKDMABUFView(opts.Profile, scaleMultiplier)
	default:
		return nil
	}
}

func newGLAreaView(profile ProfileOptions, scaleMultiplier float64) *View {
	area := gtk.NewGLArea()
	if area == nil {
		return nil
	}
	v := &View{backend: BackendGLArea, widget: &area.Widget, area: area, diag: newDiagnosticsRecorder()}
	v.setScaleMultiplier(scaleMultiplier)
	area.SetAutoRender(false)
	v.renderer = gtkgl.NewAcceleratedRenderer(area)
	v.connectRenderSignal()
	if profile.Enabled {
		if err := v.ConfigureProfiling(profile); err != nil {
			return nil
		}
	}
	return v
}

func newGDKDMABUFView(profile ProfileOptions, scaleMultiplier float64) *View {
	renderer, err := gtkgdk.NewRenderer(false)
	if err != nil || renderer == nil || renderer.Widget() == nil {
		return nil
	}
	v := &View{backend: BackendGDKDMABUF, widget: renderer.Widget(), renderer: renderer, diag: newDiagnosticsRecorder()}
	v.setScaleMultiplier(scaleMultiplier)
	v.connectRenderSignal()
	if profile.Enabled {
		if err := v.ConfigureProfiling(profile); err != nil {
			return nil
		}
	}
	return v
}

func (v *View) observedSize() (width, height int32, ok bool) {
	if v == nil {
		return 0, 0, false
	}
	width = v.cachedWidth.Load()
	height = v.cachedHeight.Load()
	return width, height, width > 0 && height > 0
}

func (v *View) connectRenderSignal() {
	if v == nil || v.widget == nil || v.renderer == nil {
		return
	}
	v.updateCachedSizeOnGTKThread()
	v.signalObject = &v.widget.Object
	strategy := sizeObservationStrategy(v.area != nil)
	v.scaleNotify = func(gobject.Object, *gobject.ParamSpec) { v.handleObservationSignal() }
	v.scaleHandlerID = v.signalObject.ConnectNotifyWithDetail(strategy.widgetNotifyDetails[0], &v.scaleNotify)
	v.mapFunc = func(gtk.Widget) { v.handleObservationSignal() }
	v.showFunc = func(gtk.Widget) { v.handleObservationSignal() }
	v.realizeFunc = func(gtk.Widget) { v.handleObservationSignal() }
	v.unrealizeFunc = func(gtk.Widget) {
		// Keep surface signal connections across transient unrealize/realize churn.
		// Reconnecting on every tab switch/map cycle allocates purego callback
		// trampolines until the process aborts under stress. Destroy() still
		// disconnects the signals when the view lifetime ends.
		if v.renderer != nil {
			v.renderer.InvalidateOnGTKThread()
		}
	}
	v.mapHandlerID = v.widget.ConnectMap(&v.mapFunc)
	v.showHandlerID = v.widget.ConnectShow(&v.showFunc)
	v.realizeHandlerID = v.widget.ConnectRealize(&v.realizeFunc)
	v.unrealizeHandlerID = v.widget.ConnectUnrealize(&v.unrealizeFunc)
	v.armSizeTickObservation()
	if !strategy.useGLAreaResize {
		return
	}
	v.resizeFunc = func(gtk.GLArea, int, int) { v.handleObservationSignal() }
	v.resizeHandlerID = v.area.ConnectResize(&v.resizeFunc)
	v.renderFunc = func(_ gtk.GLArea, _ uintptr) bool {
		v.updateCachedSizeOnGTKThread()
		return v.renderOnGTKThread()
	}
	v.renderHandlerID = v.area.ConnectRender(&v.renderFunc)
}

func (v *View) handleObservationSignal() {
	if v == nil {
		return
	}
	v.refreshSurfaceSignals()
	v.currentSizeObservationOnGTKThread()
	v.armSizeTickObservation()
}

func (v *View) refreshSurfaceSignals() {
	if v == nil || v.widget == nil {
		return
	}
	surface := widgetSurface(v.widget)
	if sameSurface(v.surface, surface) {
		return
	}
	v.disconnectSurfaceSignals()
	if surface == nil {
		return
	}
	strategy := sizeObservationStrategy(v.area != nil)
	if strategy.useSurfaceLayout {
		v.surfaceLayoutFunc = func(gdk.Surface, int, int) { v.handleObservationSignal() }
		v.surfaceLayoutHandlerID = surface.ConnectLayout(&v.surfaceLayoutFunc)
	}
	if len(strategy.surfaceSizeNotifyDetails) > 0 {
		v.surfaceWidthNotify = func(gobject.Object, *gobject.ParamSpec) { v.handleObservationSignal() }
		v.surfaceHeightNotify = func(gobject.Object, *gobject.ParamSpec) { v.handleObservationSignal() }
		v.surfaceWidthHandlerID = surface.ConnectNotifyWithDetail(strategy.surfaceSizeNotifyDetails[0], &v.surfaceWidthNotify)
		v.surfaceHeightHandlerID = surface.ConnectNotifyWithDetail(strategy.surfaceSizeNotifyDetails[1], &v.surfaceHeightNotify)
	}
	v.surfaceScaleNotify = func(gobject.Object, *gobject.ParamSpec) { v.handleObservationSignal() }
	v.surfaceScaleHandlerID = surface.ConnectNotifyWithDetail(strategy.surfaceScaleNotifyDetails[0], &v.surfaceScaleNotify)
	v.surfaceScaleFactorHandlerID = surface.ConnectNotifyWithDetail(strategy.surfaceScaleNotifyDetails[1], &v.surfaceScaleNotify)
	v.setObservedSurface(surface)
}

func (v *View) setObservedSurface(surface *gdk.Surface) {
	if v == nil || surface == nil {
		return
	}
	v.retainObservedSurface(surface)
	v.surface = surface
}

func (v *View) retainObservedSurface(surface *gdk.Surface) {
	if surface == nil {
		return
	}
	if v != nil && v.surfaceRefFunc != nil {
		v.surfaceRefFunc(surface)
		return
	}
	_ = surface.Object.Ref()
}

func (v *View) releaseObservedSurface(surface *gdk.Surface) {
	if surface == nil {
		return
	}
	if v != nil && v.surfaceUnrefFunc != nil {
		v.surfaceUnrefFunc(surface)
		return
	}
	surface.Object.Unref()
}

func (v *View) disconnectSurfaceSignals() {
	if v == nil || v.surface == nil {
		return
	}
	if v.surfaceLayoutHandlerID != 0 {
		gobject.SignalHandlerDisconnect(&v.surface.Object, v.surfaceLayoutHandlerID)
		v.surfaceLayoutHandlerID = 0
	}
	if v.surfaceWidthHandlerID != 0 {
		gobject.SignalHandlerDisconnect(&v.surface.Object, v.surfaceWidthHandlerID)
		v.surfaceWidthHandlerID = 0
	}
	if v.surfaceHeightHandlerID != 0 {
		gobject.SignalHandlerDisconnect(&v.surface.Object, v.surfaceHeightHandlerID)
		v.surfaceHeightHandlerID = 0
	}
	if v.surfaceScaleHandlerID != 0 {
		gobject.SignalHandlerDisconnect(&v.surface.Object, v.surfaceScaleHandlerID)
		v.surfaceScaleHandlerID = 0
	}
	if v.surfaceScaleFactorHandlerID != 0 {
		gobject.SignalHandlerDisconnect(&v.surface.Object, v.surfaceScaleFactorHandlerID)
		v.surfaceScaleFactorHandlerID = 0
	}
	// Keep callback function fields alive after disconnect. GTK can still deliver
	// already-queued notify/layout emissions while rapidly mapping/unmapping views;
	// niling these fields lets puregotk's trampoline dereference a nil Go func.
	surface := v.surface
	v.surface = nil
	v.releaseObservedSurface(surface)
}

func sameSurface(a, b *gdk.Surface) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.GoPointer() == b.GoPointer()
}

func (v *View) armSizeTickObservation() {
	if v == nil || v.widget == nil {
		return
	}
	v.sizeTickSettler.Reset()
	if v.sizeTickID != 0 {
		return
	}
	cb := v.sizeTickFunc
	if cb == nil {
		cb = new(gtk.TickCallback)
		*cb = func(_, _, _ uintptr) bool {
			return v.runSizeTickObservation()
		}
		v.sizeTickFunc = cb
	}
	v.sizeTickID = v.registerSizeTickCallback(cb)
}

func (v *View) runSizeTickObservation() bool {
	if v == nil || v.widget == nil {
		return false
	}
	sample := v.currentSizeObservationOnGTKThread()
	keepTicking := v.sizeTickSettler.Next(sample.width, sample.height, sample.scale)
	if !keepTicking {
		v.sizeTickID = 0
	}
	return keepTicking
}

func (v *View) currentSizeObservationOnGTKThread() sizeObservationSample {
	if v == nil {
		return sizeObservationSample{width: 1, height: 1, scale: 1}
	}
	if v.sizeObservationSampleFunc != nil {
		return v.sizeObservationSampleFunc()
	}
	v.updateCachedSizeOnGTKThread()
	return sizeObservationSample{
		width:  v.cachedWidth.Load(),
		height: v.cachedHeight.Load(),
		scale:  v.observedScale(),
	}
}

func (v *View) registerSizeTickCallback(cb *gtk.TickCallback) uint {
	if v == nil || cb == nil {
		return 0
	}
	if v.sizeTickRegistrar != nil {
		return v.sizeTickRegistrar(cb)
	}
	return v.widget.AddTickCallback(cb, 0, nil)
}

func (v *View) updateCachedSizeOnGTKThread() {
	if v == nil || v.widget == nil {
		return
	}
	prevWidth := v.cachedWidth.Load()
	prevHeight := v.cachedHeight.Load()
	width := resolveObservedDimension(prevWidth, int32(v.widget.GetAllocatedWidth()), int32(v.widget.GetWidth()))
	height := resolveObservedDimension(prevHeight, int32(v.widget.GetAllocatedHeight()), int32(v.widget.GetHeight()))
	changed := false
	if width > 0 && width != prevWidth {
		v.cachedWidth.Store(width)
		changed = true
	}
	if height > 0 && height != prevHeight {
		v.cachedHeight.Store(height)
		changed = true
	}
	prevScale := v.observedScale()
	obs := observeWidgetScale(v.widget)
	scaleChanged := prevScale != obs.scale
	v.storeObservedScale(obs.scale)
	if scaleChanged && v.input != nil {
		v.input.SetScale(v.inputScaleForObservedScale(v.effectiveScaleForSurface(obs.scale)))
	}
	if changed || scaleChanged {
		v.traceScaleObservation("widget-update", width, height, prevScale, obs)
	}
	if shouldEmitSizeHooks(changed, scaleChanged) && width > 0 && height > 0 {
		v.emitSizeHooks(width, height)
	}
}

// resolveObservedDimension picks the best observed size from cached,
// allocated, and widget in that order, except that widget==1 is treated as
// the synthetic bootstrap sentinel so it does not overwrite a larger cached
// real size during transient GTK allocation gaps.
func resolveObservedDimension(cached, allocated, widget int32) int32 {
	if allocated > 0 {
		return allocated
	}
	if widget > 1 {
		return widget
	}
	if cached > 0 {
		return cached
	}
	if widget > 0 {
		return widget
	}
	return 0
}

func (v *View) cachedSize() (int32, int32) {
	width, height := v.Size()
	return width, height
}

// RefreshObservedSizeOnGTKThread synchronously refreshes the cached observed
// size from the widget's current GTK allocation and returns the resulting size.
func (v *View) RefreshObservedSizeOnGTKThread() (int32, int32) {
	if v == nil {
		return 1, 1
	}
	v.updateCachedSizeOnGTKThread()
	return v.Size()
}

// Size returns the last positive widget size observed on the GTK thread. Before
// the widget has a real size, CEF requires a non-zero fallback, so Size returns
// 1x1.
func (v *View) Size() (int32, int32) {
	if v == nil {
		return 1, 1
	}
	width := v.cachedWidth.Load()
	height := v.cachedHeight.Load()
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	return width, height
}

// DeviceScaleFactor returns the last GTK surface scale observed for the view.
// Values <= 0 are normalized to 1.
func (v *View) DeviceScaleFactor() float32 {
	if v == nil {
		return 1
	}
	return float32(v.effectiveScaleForSurface(v.observedScale()))
}

func (v *View) observedScale() float64 {
	if v == nil {
		return 1
	}
	bits := v.scaleBits.Load()
	if bits == 0 {
		return 1
	}
	return normalizeDeviceScale(math.Float64frombits(bits))
}

func (v *View) storeObservedScale(scale float64) {
	if v == nil {
		return
	}
	v.scaleBits.Store(math.Float64bits(normalizeDeviceScale(scale)))
}

func (v *View) setScaleMultiplier(scale float64) {
	if v == nil {
		return
	}
	v.scaleMultiplierBits.Store(math.Float64bits(normalizeDeviceScale(scale)))
}

func (v *View) scaleMultiplier() float64 {
	if v == nil {
		return 1
	}
	bits := v.scaleMultiplierBits.Load()
	if bits == 0 {
		return 1
	}
	return normalizeDeviceScale(math.Float64frombits(bits))
}

func (v *View) effectiveScaleForSurface(surfaceScale float64) float64 {
	return normalizeDeviceScale(surfaceScale) * v.scaleMultiplier()
}

func normalizeDeviceScale(scale float64) float64 {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 1
	}
	return scale
}

type widgetScaleObservation struct {
	scale              float64
	widgetScaleFactor  int
	surfaceScale       float64
	surfaceScaleFactor int
	hasSurface         bool
}

func observeWidgetScale(widget *gtk.Widget) widgetScaleObservation {
	obs := widgetScaleObservation{scale: 1}
	if widget == nil {
		return obs
	}
	obs.widgetScaleFactor = widget.GetScaleFactor()
	if surface := widgetSurface(widget); surface != nil {
		obs.hasSurface = true
		if scale := surface.GetScale(); scale > 0 && !math.IsNaN(scale) && !math.IsInf(scale, 0) {
			obs.surfaceScale = scale
		}
		if scaleFactor := surface.GetScaleFactor(); scaleFactor > 0 {
			obs.surfaceScaleFactor = scaleFactor
		}
	}
	switch {
	case obs.surfaceScale > 0:
		obs.scale = obs.surfaceScale
	case obs.surfaceScaleFactor > 0:
		obs.scale = float64(obs.surfaceScaleFactor)
	case obs.widgetScaleFactor > 0:
		obs.scale = float64(obs.widgetScaleFactor)
	}
	obs.scale = normalizeDeviceScale(obs.scale)
	obs.surfaceScale = normalizeDeviceScale(obs.surfaceScale)
	return obs
}

func widgetSurface(widget *gtk.Widget) *gdk.Surface {
	if widget == nil {
		return nil
	}
	native := widget.GetNative()
	if native == nil {
		return nil
	}
	return native.GetSurface()
}

func (v *View) traceScaleObservation(reason string, width, height int32, prevScale float64, obs widgetScaleObservation) {
	if os.Getenv(scaleTraceEnvVar) == "" || v == nil || v.scaleTraceCount.Add(1) > 32 {
		return
	}
	fmt.Fprintf(os.Stderr,
		"cef2gtk-scale reason=%s backend=%s widget_logical=%dx%d scale=%.3f prev_scale=%.3f widget_scale_factor=%d surface_scale=%.3f surface_scale_factor=%d has_surface=%t input_attached=%t\n",
		reason, v.backend.String(), width, height, obs.scale, prevScale, obs.widgetScaleFactor, obs.surfaceScale, obs.surfaceScaleFactor, obs.hasSurface, v.input != nil)
}

func traceOSREnabled() bool {
	return os.Getenv(scaleTraceEnvVar) != "" || os.Getenv(osrTraceEnvVar) != ""
}

func (v *View) traceViewRect(width, height int32) {
	if !traceOSREnabled() || v == nil || v.osrTraceCount.Add(1) > 96 {
		return
	}
	widgetWidth, widgetHeight := v.cachedSize()
	scale := float64(v.DeviceScaleFactor())
	expectedWidth, expectedHeight := expectedDeviceSize(widgetWidth, widgetHeight, scale)
	fmt.Fprintf(os.Stderr,
		"cef2gtk-osr-geometry callback=view-rect backend=%s widget_logical=%dx%d osr_rect=%dx%d device_scale=%.3f backing_scale_enabled=%t expected_device=%dx%d\n",
		v.backend.String(), widgetWidth, widgetHeight, width, height, scale, v.osrBackingScaleEnabled(), expectedWidth, expectedHeight)
}

func (v *View) traceScreenInfo(width, height int32, scale float32) {
	if !traceOSREnabled() || v == nil || v.osrTraceCount.Add(1) > 96 {
		return
	}
	widgetWidth, widgetHeight := v.cachedSize()
	surfaceScale := float64(v.DeviceScaleFactor())
	expectedWidth, expectedHeight := expectedDeviceSize(widgetWidth, widgetHeight, surfaceScale)
	reportedExpectedWidth, reportedExpectedHeight := expectedDeviceSize(width, height, float64(scale))
	fmt.Fprintf(os.Stderr,
		"cef2gtk-screen-info backend=%s widget_logical=%dx%d screen_rect=%dx%d device_scale=%.3f surface_scale=%.3f backing_scale_enabled=%t widget_expected_device=%dx%d reported_expected_device=%dx%d\n",
		v.backend.String(), widgetWidth, widgetHeight, width, height, scale, surfaceScale, v.osrBackingScaleEnabled(), expectedWidth, expectedHeight, reportedExpectedWidth, reportedExpectedHeight)
}

func (v *View) traceScreenGeometryCallback(callback string, viewX, viewY int32, returns bool) {
	if !traceOSREnabled() || v == nil || v.osrTraceCount.Add(1) > 96 {
		return
	}
	widgetWidth, widgetHeight := v.cachedSize()
	osrWidth, osrHeight := v.osrViewRectSize()
	scale := float64(v.DeviceScaleFactor())
	expectedWidth, expectedHeight := expectedDeviceSize(widgetWidth, widgetHeight, scale)
	fmt.Fprintf(os.Stderr,
		"cef2gtk-osr-geometry callback=%s backend=%s widget_logical=%dx%d osr_rect=%dx%d view_point=%d,%d device_scale=%.3f expected_device=%dx%d returns=%t\n",
		callback, v.backend.String(), widgetWidth, widgetHeight, osrWidth, osrHeight, viewX, viewY, scale, expectedWidth, expectedHeight, returns)
}

func expectedDeviceSize(width, height int32, scale float64) (int32, int32) {
	return int32(math.Ceil(float64(width) * scale)), int32(math.Ceil(float64(height) * scale))
}

// AddSizeObserver registers a callback invoked on the GTK thread when the view
// observes a positive size change. It returns a function that unregisters the
// observer. If a real positive size has already been observed, the callback is
// invoked immediately with that size; the synthetic Size() fallback is not
// emitted as an observer event. Register and unregister from the GTK thread.
func (v *View) AddSizeObserver(fn func(width, height int32)) func() {
	if v == nil || fn == nil {
		return func() {}
	}
	v.sizeHooksMu.Lock()
	if v.sizeHooks == nil {
		v.sizeHooks = make(map[uint64]func(width, height int32))
	}
	v.nextSizeHookID++
	id := v.nextSizeHookID
	v.sizeHooks[id] = fn
	v.sizeHooksMu.Unlock()
	width, height, ok := v.observedSize()
	if ok {
		fn(width, height)
	}
	return func() {
		v.sizeHooksMu.Lock()
		delete(v.sizeHooks, id)
		v.sizeHooksMu.Unlock()
	}
}

func (v *View) emitSizeHooks(width, height int32) {
	v.sizeHooksMu.Lock()
	hooks := make([]func(width, height int32), 0, len(v.sizeHooks))
	for _, hook := range v.sizeHooks {
		hooks = append(hooks, hook)
	}
	v.sizeHooksMu.Unlock()
	for _, hook := range hooks {
		hook(width, height)
	}
}

func (v *View) renderOnGTKThread() bool {
	if v == nil || v.renderer == nil {
		return false
	}
	if err := v.renderer.RenderQueuedOnGTKThread(); err != nil {
		v.diag.RecordRenderFailure(err)
		v.recordProfileRenderFailure()
		if v.hooks.OnError != nil {
			v.hooks.OnError(err)
		}
		v.emitProfileIfDue(time.Now())
		return false
	}
	v.recordProfileFrameRendered()
	v.emitProfileIfDue(time.Now())
	return true
}

// HasFocus reports whether the effective GTK input widget currently has focus.
// Call on the GTK main thread.
func (v *View) HasFocus() bool {
	widget := v.effectiveInputWidget()
	if widget == nil {
		return false
	}
	return widget.HasFocus()
}

func (v *View) effectiveInputWidget() *gtk.Widget {
	if v == nil {
		return nil
	}
	if v.input != nil && v.inputWidget != nil {
		return v.inputWidget
	}
	return v.widget
}

// SetCursorFromName applies a named cursor to the underlying GTK widget. Call
// on the GTK main thread.
func (v *View) SetCursorFromName(name string) {
	if v == nil || v.widget == nil {
		return
	}
	v.widget.SetCursorFromName(&name)
}

// Widget returns the GtkWidget for packing into GTK containers.
func (v *View) Widget() *gtk.Widget {
	if v == nil {
		return nil
	}
	return v.widget
}

// Backend returns the selected presentation backend.
func (v *View) Backend() Backend {
	if v == nil {
		return ""
	}
	return v.backend
}

// GLArea returns the underlying GtkGLArea for the GLArea backend.
func (v *View) GLArea() *gtk.GLArea {
	if v == nil {
		return nil
	}
	return v.area
}

// PrepareOnGTKThread initializes renderer resources. Call on GTK main thread.
func (v *View) PrepareOnGTKThread() error {
	if v == nil || v.renderer == nil {
		return ErrNilView
	}
	if v.area == nil {
		return v.renderer.InitializeOnGTKThread()
	}
	if !v.area.GetRealized() {
		return gtkgl.ErrGLAreaNotRealized
	}
	v.area.MakeCurrent()
	return v.renderer.InitializeOnGTKThread()
}

// ConfigureProfiling enables or disables development-only render profiling.
// When enabled, snapshots are emitted at opts.Interval through opts.OnSnapshot
// and/or opts.Writer. Rendering continues if snapshot writing fails.
func (v *View) ConfigureProfiling(opts ProfileOptions) error {
	if v == nil || v.renderer == nil {
		return ErrNilView
	}
	v.profileMu.Lock()
	defer v.profileMu.Unlock()
	if !opts.Enabled {
		v.profileEnabled.Store(false)
		v.profile = nil
		v.profilePtr.Store(nil)
		v.profileOptions = ProfileOptions{}
		v.renderer.SetProfiler(nil)
		if v.input != nil {
			v.input.SetProfiler(nil)
		}
		return nil
	}
	recorder := internalprofile.NewRecorder()
	recorder.SetBackend(v.backend.String())
	recorder.Start(time.Now())
	v.profile = recorder
	v.profilePtr.Store(recorder)
	v.profileOptions = opts
	v.profileEnabled.Store(true)
	v.renderer.SetProfiler(recorder)
	if v.input != nil {
		v.input.SetProfiler(recorder)
	}
	return nil
}

func (v *View) profileRecorder() *internalprofile.Recorder {
	if v == nil || !v.profileEnabled.Load() {
		return nil
	}
	return v.profilePtr.Load()
}

func (v *View) recordProfileFrameReceived() {
	if p := v.profileRecorder(); p != nil {
		p.RecordFrameReceived()
	}
}

func (v *View) recordProfileFrameQueued() {
	if p := v.profileRecorder(); p != nil {
		p.RecordFrameQueued()
	}
}

func (v *View) recordProfileFrameRendered() {
	if p := v.profileRecorder(); p != nil {
		p.RecordFrameRendered()
	}
}

func (v *View) recordProfileImportFailure() {
	if p := v.profileRecorder(); p != nil {
		p.RecordImportFailure()
	}
}

func (v *View) recordProfileRenderFailure() {
	if p := v.profileRecorder(); p != nil {
		p.RecordRenderFailure()
	}
}

func (v *View) recordProfileUnsupportedPaint() {
	if p := v.profileRecorder(); p != nil {
		p.RecordUnsupportedPaint()
	}
}

func (v *View) RecordExternalBeginFrameSent() {
	if p := v.profileRecorder(); p != nil {
		p.RecordExternalBeginFrameSent()
	}
}

func (v *View) emitProfileIfDue(now time.Time) {
	if v == nil || !v.profileEnabled.Load() {
		return
	}
	v.profileMu.Lock()
	p := v.profile
	opts := v.profileOptions
	v.profileMu.Unlock()
	if p == nil || !opts.Enabled {
		return
	}
	snap, ok := p.MaybeSnapshot(now, opts.normalizedInterval())
	if !ok {
		return
	}
	onSnapshot := opts.OnSnapshot
	writer := opts.Writer
	if onSnapshot != nil {
		onSnapshot(snap)
	}
	_ = writeProfileSnapshot(writer, snap)
}

// Diagnostics returns a point-in-time diagnostics snapshot.
func (v *View) Diagnostics() Diagnostics {
	if v == nil || v.diag == nil {
		return Diagnostics{}
	}
	snap := v.diag.Snapshot()
	snap.Backend = v.backend.String()
	if gdkDiag, ok := v.renderer.(interface{ Diagnostics() gtkgdk.Diagnostics }); ok {
		d := gdkDiag.Diagnostics()
		snap.TexturesBuilt = int(d.TexturesBuilt)
		snap.TextureBuildFailures = int(d.TextureBuildFailures)
		snap.FDDupFailures = int(d.FDDupFailures)
		snap.UnsupportedFormats = int(d.UnsupportedFormats)
		snap.PaintableSwaps = int(d.PaintableSwaps)
		snap.PendingFrame = d.PendingFrame
		snap.PendingScheduled = d.PendingScheduled
		snap.PendingAge = d.PendingAge
		snap.PendingSourceID = d.PendingSourceID
		snap.PendingReschedules = int(d.PendingReschedules)
		snap.PendingScheduleFailures = int(d.PendingScheduleFailures)
		snap.PendingIdleCallbacks = int(d.PendingIdleCallbacks)
	}
	return snap
}

// Destroy releases GL resources owned by the view. Call on the GTK main thread;
// it disconnects GTK signal handlers and is not safe for concurrent use.
func (v *View) Destroy() error {
	if v == nil {
		return ErrNilView
	}
	v.renderLifecycleMu.Lock()
	defer v.renderLifecycleMu.Unlock()
	v.destroyed.Store(true)
	if v.input != nil {
		v.input.Detach()
		v.input = nil
	}
	v.inputWidget = nil
	if v.widget != nil && v.sizeTickID != 0 {
		v.widget.RemoveTickCallback(v.sizeTickID)
		v.sizeTickID = 0
	}
	if v.sizeTickFunc != nil {
		_ = glib.UnrefCallback(v.sizeTickFunc)
		v.sizeTickFunc = nil
	}
	v.disconnectSurfaceSignals()
	if v.signalObject != nil {
		if v.scaleHandlerID != 0 {
			gobject.SignalHandlerDisconnect(v.signalObject, v.scaleHandlerID)
			v.scaleHandlerID = 0
		}
		if v.mapHandlerID != 0 {
			gobject.SignalHandlerDisconnect(v.signalObject, v.mapHandlerID)
			v.mapHandlerID = 0
		}
		if v.showHandlerID != 0 {
			gobject.SignalHandlerDisconnect(v.signalObject, v.showHandlerID)
			v.showHandlerID = 0
		}
		if v.realizeHandlerID != 0 {
			gobject.SignalHandlerDisconnect(v.signalObject, v.realizeHandlerID)
			v.realizeHandlerID = 0
		}
		if v.unrealizeHandlerID != 0 {
			gobject.SignalHandlerDisconnect(v.signalObject, v.unrealizeHandlerID)
			v.unrealizeHandlerID = 0
		}
		v.signalObject = nil
	}
	if v.area != nil {
		if v.resizeHandlerID != 0 {
			gobject.SignalHandlerDisconnect(&v.area.Object, v.resizeHandlerID)
			v.resizeHandlerID = 0
		}
		if v.renderHandlerID != 0 {
			gobject.SignalHandlerDisconnect(&v.area.Object, v.renderHandlerID)
			v.renderHandlerID = 0
		}
	}
	if v.renderer != nil {
		if v.area != nil {
			if v.area.GetRealized() {
				v.area.MakeCurrent()
				v.renderer.Close()
			} else {
				v.renderer.InvalidateOnGTKThread()
			}
		} else {
			v.renderer.InvalidateOnGTKThread()
		}
		v.renderer = nil
	}
	v.sizeHooksMu.Lock()
	v.sizeHooks = nil
	v.sizeHooksMu.Unlock()
	v.profileMu.Lock()
	v.profileEnabled.Store(false)
	v.profile = nil
	v.profileOptions = ProfileOptions{}
	v.profileMu.Unlock()
	v.handler = nil
	v.area = nil
	v.widget = nil
	return nil
}
