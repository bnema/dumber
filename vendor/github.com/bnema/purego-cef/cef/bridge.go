// bridge.go provides package-level helper functions that delegate to the
// core Engine. Generated code in cef/ calls these helpers for string
// conversion, refcount management, and pointer extraction.
//
// This file is handwritten — the rest of cef/ is generated.
package cef

import (
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego-cef/internal/capi"
	"github.com/bnema/purego-cef/internal/core"
)

// RawClientWriteSlot provides write-only access to the popup client out-param
// exposed by CEF popup callbacks.
//
// The slot preserves CEF's default raw client until Set or Clear is called.
// It is only valid for the duration of the active callback and must not be
// retained after the callback returns. Popup callbacks may receive a nil slot
// when CEF does not provide a writable client out-param. Set and Clear panic
// if used on a nil, zero-value, or invalidated slot.
type RawClientWriteSlot struct {
	initialRaw unsafe.Pointer
	value      RawClient
	touched    bool
	valid      atomic.Bool
}

func newRawClientWriteSlot(initialRaw unsafe.Pointer) *RawClientWriteSlot {
	s := &RawClientWriteSlot{initialRaw: initialRaw}
	s.valid.Store(true)
	return s
}

func (s *RawClientWriteSlot) invalidate() {
	if s != nil {
		s.valid.Store(false)
	}
}

func (s *RawClientWriteSlot) assertUsable() {
	if s == nil {
		panic("RawClientWriteSlot: slot is nil")
	}
	if !s.valid.Load() {
		panic("RawClientWriteSlot: slot is no longer valid")
	}
}

// Set replaces the popup client out-param with raw.
func (s *RawClientWriteSlot) Set(raw RawClient) {
	s.assertUsable()
	s.value = raw
	s.touched = true
}

// Clear explicitly writes a nil popup client out-param.
func (s *RawClientWriteSlot) Clear() {
	s.assertUsable()
	s.value = nil
	s.touched = true
}

func (s *RawClientWriteSlot) rawPointer() unsafe.Pointer {
	if s == nil {
		return nil
	}
	if s.touched {
		return extractOrWrapRawPointer(s.value, func() any { return NewRawClient(s.value) })
	}
	return s.initialRaw
}

// LifeSpanHandler is the user-facing lifespan handler interface with typed
// out-params.
//
// CEF popup callbacks expose the client as a raw handler out-param. The safe
// API intentionally exposes that out-param as a write-only RawClientWriteSlot:
// handlers that need to replace or clear the client can do so without retaining
// a callback-scoped pointer. The slot may be nil when CEF does not provide a
// writable client out-param, so handlers must check it before calling Set or
// Clear.
type LifeSpanHandler interface {
	OnBeforePopup(browser Browser, frame Frame, popupID int32, targetURL string, targetFrameName string,
		targetDisposition WindowOpenDisposition, userGesture int32, popupFeatures *PopupFeatures,
		windowInfo *WindowInfo, client *RawClientWriteSlot, settings *BrowserSettings,
		extraInfo *DictionaryValue, noJavascriptAccess *bool) bool
	OnBeforePopupAborted(browser Browser, popupID int32)
	OnBeforeDevToolsPopup(browser Browser, windowInfo *WindowInfo, client *RawClientWriteSlot,
		settings *BrowserSettings, extraInfo *DictionaryValue, useDefaultWindow *bool)
	OnAfterCreated(browser Browser)
	DoClose(browser Browser) bool
	OnBeforeClose(browser Browser)
}

// AudioHandler handles audio events with a safe [][]float32 data signature.
// The constructor decodes the raw float** callback input for you.
type AudioHandler interface {
	GetAudioParameters(browser Browser, params *AudioParameters) int32
	OnAudioStreamStarted(browser Browser, params *AudioParameters, channels int32)
	OnAudioStreamPacket(browser Browser, data [][]float32, frames int32, pts int64)
	OnAudioStreamStopped(browser Browser)
	OnAudioStreamError(browser Browser, message string)
}

// ---------------------------------------------------------------------------
// Safe lifespan handler adapter
// ---------------------------------------------------------------------------

type safeLifeSpanHandlerWrapper struct {
	impl   LifeSpanHandler
	rawPtr *capi.CEFLifeSpanHandlerT
}

func (w *safeLifeSpanHandlerWrapper) RawPointer() unsafe.Pointer {
	return unsafe.Pointer(w.rawPtr)
}

// portin.LifeSpanHandler interface compliance — callbacks go through purego,
// so these methods should never be called directly. Panic to catch misuse.
func (w *safeLifeSpanHandlerWrapper) OnBeforePopup(Browser, Frame, int32, string, string, WindowOpenDisposition, int32, *PopupFeatures, *WindowInfo, unsafe.Pointer, *BrowserSettings, unsafe.Pointer, *int32) bool {
	panic("safeLifeSpanHandlerWrapper: raw OnBeforePopup called directly; callbacks go through purego")
}
func (w *safeLifeSpanHandlerWrapper) OnBeforePopupAborted(Browser, int32) {
	panic("safeLifeSpanHandlerWrapper: raw OnBeforePopupAborted called directly; callbacks go through purego")
}
func (w *safeLifeSpanHandlerWrapper) OnBeforeDevToolsPopup(Browser, *WindowInfo, unsafe.Pointer, *BrowserSettings, unsafe.Pointer, *int32) {
	panic("safeLifeSpanHandlerWrapper: raw OnBeforeDevToolsPopup called directly; callbacks go through purego")
}
func (w *safeLifeSpanHandlerWrapper) OnAfterCreated(Browser) {
	panic("safeLifeSpanHandlerWrapper: raw OnAfterCreated called directly; callbacks go through purego")
}
func (w *safeLifeSpanHandlerWrapper) DoClose(Browser) bool {
	panic("safeLifeSpanHandlerWrapper: raw DoClose called directly; callbacks go through purego")
}
func (w *safeLifeSpanHandlerWrapper) OnBeforeClose(Browser) {
	panic("safeLifeSpanHandlerWrapper: raw OnBeforeClose called directly; callbacks go through purego")
}

var lifeSpanHandlerOnBeforePopupOnce sync.Once
var lifeSpanHandlerOnBeforePopupCallback uintptr
var lifeSpanHandlerOnBeforePopupAbortedOnce sync.Once
var lifeSpanHandlerOnBeforePopupAbortedCallback uintptr
var lifeSpanHandlerOnBeforeDevToolsPopupOnce sync.Once
var lifeSpanHandlerOnBeforeDevToolsPopupCallback uintptr
var lifeSpanHandlerOnAfterCreatedOnce sync.Once
var lifeSpanHandlerOnAfterCreatedCallback uintptr
var lifeSpanHandlerDoCloseOnce sync.Once
var lifeSpanHandlerDoCloseCallback uintptr
var lifeSpanHandlerOnBeforeCloseOnce sync.Once
var lifeSpanHandlerOnBeforeCloseCallback uintptr

// NewLifeSpanHandler creates a raw lifespan handler from the user-facing
// typed lifespan handler interface. It converts raw callback out-params to
// typed Go values and writes back any changes the consumer makes.
func NewLifeSpanHandler(impl LifeSpanHandler) RawLifeSpanHandler {
	if isNilImpl(impl) {
		return nil
	}
	r := new(capi.CEFLifeSpanHandlerT)
	w := &safeLifeSpanHandlerWrapper{rawPtr: r, impl: impl}
	initRefCount(unsafe.Pointer(r), unsafe.Sizeof(*r), w)

	r.OverrideOnBeforePopup(sharedCEFCallback(&lifeSpanHandlerOnBeforePopupOnce, &lifeSpanHandlerOnBeforePopupCallback, func(self uintptr, arg0, arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9, arg10, arg11, arg12 uintptr) uintptr {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return 0
		}
		browser := wrapBrowser(unsafe.Pointer(arg0))
		frame := wrapFrame(unsafe.Pointer(arg1))
		popupID := int32(arg2)
		targetURL := goString(unsafe.Pointer(arg3))
		targetFrameName := goString(unsafe.Pointer(arg4))
		targetDisposition := WindowOpenDisposition(arg5)
		userGesture := int32(arg6)
		popupFeatures := (*PopupFeatures)(unsafe.Pointer(arg7))
		windowInfo := (*WindowInfo)(unsafe.Pointer(arg8))
		settings := (*BrowserSettings)(unsafe.Pointer(arg10))

		// Provide write-only access to the popup client out-param.
		var clientSlot *RawClientWriteSlot
		if arg9 != 0 {
			clientSlot = newRawClientWriteSlot(*(*unsafe.Pointer)(unsafe.Pointer(arg9)))
		}
		defer clientSlot.invalidate()

		// Decode out-param: extraInfo (cef_dictionary_value_t**)
		var extraInfoVal DictionaryValue
		if arg11 != 0 {
			if ep := *(*unsafe.Pointer)(unsafe.Pointer(arg11)); ep != nil {
				extraInfoVal = wrapDictionaryValue(ep)
			}
		}

		// Decode out-param: noJavascriptAccess (int*)
		var noJS bool
		if arg12 != 0 {
			noJS = *(*int32)(unsafe.Pointer(arg12)) != 0
		}

		blocked := owner.impl.OnBeforePopup(browser, frame, popupID, targetURL, targetFrameName,
			targetDisposition, userGesture, popupFeatures, windowInfo,
			clientSlot, settings, &extraInfoVal, &noJS)

		// Write back out-params.
		if arg9 != 0 {
			*(*unsafe.Pointer)(unsafe.Pointer(arg9)) = clientSlot.rawPointer()
		}
		if arg11 != 0 {
			*(*unsafe.Pointer)(unsafe.Pointer(arg11)) = extractRawPointer(extraInfoVal)
		}
		if arg12 != 0 {
			if noJS {
				*(*int32)(unsafe.Pointer(arg12)) = 1
			} else {
				*(*int32)(unsafe.Pointer(arg12)) = 0
			}
		}

		if blocked {
			return 1
		}
		return 0
	}))

	r.OverrideOnBeforePopupAborted(sharedCEFCallback(&lifeSpanHandlerOnBeforePopupAbortedOnce, &lifeSpanHandlerOnBeforePopupAbortedCallback, func(self uintptr, arg0, arg1 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return
		}
		owner.impl.OnBeforePopupAborted(wrapBrowser(unsafe.Pointer(arg0)), int32(arg1))
	}))

	r.OverrideOnBeforeDevToolsPopup(sharedCEFCallback(&lifeSpanHandlerOnBeforeDevToolsPopupOnce, &lifeSpanHandlerOnBeforeDevToolsPopupCallback, func(self uintptr, arg0, arg1, arg2, arg3, arg4, arg5 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return
		}
		browser := wrapBrowser(unsafe.Pointer(arg0))
		windowInfo := (*WindowInfo)(unsafe.Pointer(arg1))
		settings := (*BrowserSettings)(unsafe.Pointer(arg3))

		// Provide write-only access to the popup client out-param.
		var clientSlot *RawClientWriteSlot
		if arg2 != 0 {
			clientSlot = newRawClientWriteSlot(*(*unsafe.Pointer)(unsafe.Pointer(arg2)))
		}
		defer clientSlot.invalidate()

		// Decode out-param: extraInfo (cef_dictionary_value_t**)
		var extraInfoVal DictionaryValue
		if arg4 != 0 {
			if ep := *(*unsafe.Pointer)(unsafe.Pointer(arg4)); ep != nil {
				extraInfoVal = wrapDictionaryValue(ep)
			}
		}

		// Decode out-param: useDefaultWindow (int*)
		var useDefault bool
		if arg5 != 0 {
			useDefault = *(*int32)(unsafe.Pointer(arg5)) != 0
		}

		owner.impl.OnBeforeDevToolsPopup(browser, windowInfo, clientSlot, settings, &extraInfoVal, &useDefault)

		// Write back out-params.
		if arg2 != 0 {
			*(*unsafe.Pointer)(unsafe.Pointer(arg2)) = clientSlot.rawPointer()
		}
		if arg4 != 0 {
			*(*unsafe.Pointer)(unsafe.Pointer(arg4)) = extractRawPointer(extraInfoVal)
		}
		if arg5 != 0 {
			if useDefault {
				*(*int32)(unsafe.Pointer(arg5)) = 1
			} else {
				*(*int32)(unsafe.Pointer(arg5)) = 0
			}
		}
	}))

	r.OverrideOnAfterCreated(sharedCEFCallback(&lifeSpanHandlerOnAfterCreatedOnce, &lifeSpanHandlerOnAfterCreatedCallback, func(self uintptr, arg0 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return
		}
		owner.impl.OnAfterCreated(wrapBrowser(unsafe.Pointer(arg0)))
	}))

	r.OverrideDoClose(sharedCEFCallback(&lifeSpanHandlerDoCloseOnce, &lifeSpanHandlerDoCloseCallback, func(self uintptr, arg0 uintptr) uintptr {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return 0
		}
		if owner.impl.DoClose(wrapBrowser(unsafe.Pointer(arg0))) {
			return 1
		}
		return 0
	}))

	r.OverrideOnBeforeClose(sharedCEFCallback(&lifeSpanHandlerOnBeforeCloseOnce, &lifeSpanHandlerOnBeforeCloseCallback, func(self uintptr, arg0 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*safeLifeSpanHandlerWrapper](self)
		if !ownerOK {
			return
		}
		owner.impl.OnBeforeClose(wrapBrowser(unsafe.Pointer(arg0)))
	}))

	return w
}

// ---------------------------------------------------------------------------
// AudioHandler constructor — decodes float** to [][]float32
// ---------------------------------------------------------------------------

type audioHandlerWrapper struct {
	impl     AudioHandler
	rawPtr   *capi.CEFAudioHandlerT
	mu       sync.Mutex
	channels int32
}

func (w *audioHandlerWrapper) RawPointer() unsafe.Pointer {
	return unsafe.Pointer(w.rawPtr)
}

// Satisfy the generated portin.AudioHandler interface for extractRawPointer.
func (w *audioHandlerWrapper) GetAudioParameters(browser Browser, params *AudioParameters) int32 {
	return w.impl.GetAudioParameters(browser, params)
}
func (w *audioHandlerWrapper) OnAudioStreamStarted(browser Browser, params *AudioParameters, channels int32) {
	w.impl.OnAudioStreamStarted(browser, params, channels)
}
func (w *audioHandlerWrapper) OnAudioStreamPacket(browser Browser, _ unsafe.Pointer, frames int32, pts int64) {
	panic("audioHandlerWrapper: raw OnAudioStreamPacket called directly; use the purego callback path or NewRawAudioHandler for raw audio packets")
}
func (w *audioHandlerWrapper) OnAudioStreamStopped(browser Browser) {
	w.impl.OnAudioStreamStopped(browser)
}
func (w *audioHandlerWrapper) OnAudioStreamError(browser Browser, message string) {
	w.impl.OnAudioStreamError(browser, message)
}

var audioHandlerGetAudioParametersOnce sync.Once
var audioHandlerGetAudioParametersCallback uintptr
var audioHandlerOnAudioStreamStartedOnce sync.Once
var audioHandlerOnAudioStreamStartedCallback uintptr
var audioHandlerOnAudioStreamPacketOnce sync.Once
var audioHandlerOnAudioStreamPacketCallback uintptr
var audioHandlerOnAudioStreamStoppedOnce sync.Once
var audioHandlerOnAudioStreamStoppedCallback uintptr
var audioHandlerOnAudioStreamErrorOnce sync.Once
var audioHandlerOnAudioStreamErrorCallback uintptr

// NewAudioHandler creates a CEF handler with decoded [][]float32 audio packets.
func NewAudioHandler(impl AudioHandler) RawAudioHandler {
	if isNilImpl(impl) {
		return nil
	}
	r := new(capi.CEFAudioHandlerT)
	w := &audioHandlerWrapper{rawPtr: r, impl: impl}
	initRefCount(unsafe.Pointer(r), unsafe.Sizeof(*r), w)

	r.OverrideGetAudioParameters(sharedCEFCallback(&audioHandlerGetAudioParametersOnce, &audioHandlerGetAudioParametersCallback, func(self uintptr, arg0, arg1 uintptr) uintptr {
		owner, ownerOK := cefCallbackOwnerAs[*audioHandlerWrapper](self)
		if !ownerOK {
			return 0
		}
		browser := wrapBrowser(unsafe.Pointer(arg0))
		params := (*AudioParameters)(unsafe.Pointer(arg1))
		return uintptr(owner.impl.GetAudioParameters(browser, params))
	}))

	r.OverrideOnAudioStreamStarted(sharedCEFCallback(&audioHandlerOnAudioStreamStartedOnce, &audioHandlerOnAudioStreamStartedCallback, func(self uintptr, arg0, arg1, arg2 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*audioHandlerWrapper](self)
		if !ownerOK {
			return
		}
		browser := wrapBrowser(unsafe.Pointer(arg0))
		params := (*AudioParameters)(unsafe.Pointer(arg1))
		channels := int32(arg2)
		owner.mu.Lock()
		owner.channels = channels
		owner.mu.Unlock()
		owner.impl.OnAudioStreamStarted(browser, params, channels)
	}))

	r.OverrideOnAudioStreamPacket(sharedCEFCallback(&audioHandlerOnAudioStreamPacketOnce, &audioHandlerOnAudioStreamPacketCallback, func(self uintptr, arg0, arg1, arg2, arg3 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*audioHandlerWrapper](self)
		if !ownerOK {
			return
		}
		browser := wrapBrowser(unsafe.Pointer(arg0))
		frames := int32(arg2)
		pts := int64(arg3)
		owner.mu.Lock()
		ch := owner.channels
		owner.mu.Unlock()
		decoded := core.DecodeAudioPacket(unsafe.Pointer(arg1), ch, frames)
		owner.impl.OnAudioStreamPacket(browser, decoded, frames, pts)
	}))

	r.OverrideOnAudioStreamStopped(sharedCEFCallback(&audioHandlerOnAudioStreamStoppedOnce, &audioHandlerOnAudioStreamStoppedCallback, func(self uintptr, arg0 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*audioHandlerWrapper](self)
		if !ownerOK {
			return
		}
		owner.impl.OnAudioStreamStopped(wrapBrowser(unsafe.Pointer(arg0)))
	}))

	r.OverrideOnAudioStreamError(sharedCEFCallback(&audioHandlerOnAudioStreamErrorOnce, &audioHandlerOnAudioStreamErrorCallback, func(self uintptr, arg0, arg1 uintptr) {
		owner, ownerOK := cefCallbackOwnerAs[*audioHandlerWrapper](self)
		if !ownerOK {
			return
		}
		owner.impl.OnAudioStreamError(wrapBrowser(unsafe.Pointer(arg0)), goString(unsafe.Pointer(arg1)))
	}))

	return w
}

// ---------------------------------------------------------------------------
// Engine and refcount access — initialised once via sync.Once in Init()
// ---------------------------------------------------------------------------

var (
	eng                   *core.Engine
	initOnce              sync.Once
	initErr               error
	refManagerMu          sync.RWMutex
	currentRefManager     *core.RefManager
	registeredRefManagers []*core.RefManager
	debugRefCountf        func(format string, args ...any)
)

func mustEng() *core.Engine {
	if eng == nil {
		panic("cef: engine not initialized; call cef.Init() first")
	}
	return eng
}

// cefString converts a Go string to a CEF UTF-16 string.
func cefString(s string) core.CEFStringT {
	return mustEng().CefString(s)
}

// freeCefString releases a CEF string's backing memory.
func freeCefString(cs *core.CEFStringT) {
	mustEng().FreeCefString(cs)
}

// goString converts a pointer to a CEF string to a Go string.
func goString(cs unsafe.Pointer) string {
	return core.GoString(cs)
}

// goStringUserfree converts a cef_string_userfree_t to a Go string and frees it.
func goStringUserfree(ptr unsafe.Pointer) string {
	return mustEng().GoStringUserfree(ptr)
}

type typedCallbackKey struct {
	raw uintptr
	typ reflect.Type
}

var typedCallbackCache sync.Map

// registerTypedCallback binds a raw CEF function pointer to a typed Go function.
// Generated wrappers use this when ABI correctness requires non-uintptr arguments
// such as float64 or int64.
func registerTypedCallback(fn any, raw uintptr) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Func {
		purego.RegisterFunc(fn, raw)
		return
	}

	key := typedCallbackKey{raw: raw, typ: v.Elem().Type()}
	if cached, ok := typedCallbackCache.Load(key); ok {
		v.Elem().Set(reflect.ValueOf(cached))
		return
	}

	purego.RegisterFunc(fn, raw)
	actual, loaded := typedCallbackCache.LoadOrStore(key, v.Elem().Interface())
	if loaded {
		v.Elem().Set(reflect.ValueOf(actual))
	}
}

func setCurrentRefManager(rm *core.RefManager) {
	refManagerMu.Lock()
	defer refManagerMu.Unlock()
	currentRefManager = rm
	registerRefManagerLocked(rm)
}

func withCurrentRefManager(rm *core.RefManager, fn func()) {
	refManagerMu.Lock()
	prev := currentRefManager
	currentRefManager = rm
	refManagerMu.Unlock()
	defer func() {
		refManagerMu.Lock()
		currentRefManager = prev
		refManagerMu.Unlock()
	}()
	fn()
}

func registerRefManager(rm *core.RefManager) {
	refManagerMu.Lock()
	defer refManagerMu.Unlock()
	registerRefManagerLocked(rm)
}

func registerRefManagerLocked(rm *core.RefManager) {
	if rm == nil {
		return
	}
	for _, existing := range registeredRefManagers {
		if existing == rm {
			return
		}
	}
	registeredRefManagers = append(registeredRefManagers, rm)
}

func unregisterRefManager(rm *core.RefManager) {
	if rm == nil {
		return
	}
	refManagerMu.Lock()
	defer refManagerMu.Unlock()
	for i, existing := range registeredRefManagers {
		if existing != rm {
			continue
		}
		registeredRefManagers = append(registeredRefManagers[:i], registeredRefManagers[i+1:]...)
		return
	}
}

func mustCurrentRefManager() *core.RefManager {
	refManagerMu.RLock()
	rm := currentRefManager
	refManagerMu.RUnlock()
	if rm == nil {
		panic("cef: ref manager not initialized; call cef.Init() first")
	}
	return rm
}

// initRefCount wires refcount callbacks into a CEF base struct header.
func initRefCount(base unsafe.Pointer, size uintptr, owner any) {
	mustCurrentRefManager().InitRefCount(base, size, owner)
}

// sharedCEFCallback creates a process-lifetime callback trampoline once. It is
// used for generated handler methods that can dispatch by their CEF self
// pointer instead of capturing per-object Go state in a fresh closure.
func sharedCEFCallback(once *sync.Once, slot *uintptr, fn any) uintptr {
	once.Do(func() {
		*slot = purego.NewCallback(fn)
	})
	return *slot
}

// cefCallbackPointer converts a native CEF callback uintptr to a pointer. CEF
// may synchronously invoke callbacks with Go-backed pointers in tests, so keep
// this checkptr exception at the native callback boundary.
//
//go:nocheckptr
func cefCallbackPointer(value uintptr) unsafe.Pointer {
	return unsafe.Pointer(value)
}

//go:nocheckptr
func cefCallbackOwnerAs[T any](self uintptr) (T, bool) {
	var zero T
	if self == 0 {
		return zero, false
	}
	base := unsafe.Pointer(self)

	// Hold the registry read lock through Owner. This keeps a manager visible
	// for the entire lookup while register/unregister updates the slice. Owner
	// only reads RefManager's sync.Map; no user callback is invoked under this
	// lock.
	refManagerMu.RLock()
	defer refManagerMu.RUnlock()
	for i := len(registeredRefManagers) - 1; i >= 0; i-- {
		owner, ok := registeredRefManagers[i].Owner(base)
		if !ok {
			continue
		}
		typed, ok := owner.(T)
		if !ok {
			return zero, false
		}
		return typed, true
	}
	return zero, false
}

// addRef increments the refcount for the object at base.
func addRef(base unsafe.Pointer) {
	if base == nil {
		return
	}
	refManagerMu.RLock()
	for i := len(registeredRefManagers) - 1; i >= 0; i-- {
		if registeredRefManagers[i].Has(base) {
			registeredRefManagers[i].AddRef(base)
			refManagerMu.RUnlock()
			return
		}
	}
	debugf := debugRefCountf
	refManagerMu.RUnlock()
	if debugf != nil {
		debugf("cef: addRef: no RefManager found for pointer %p", base)
	}
}

// extractRawPointer returns the underlying raw CEF pointer from an interface.
func extractRawPointer(v any) unsafe.Pointer {
	return core.ExtractRawPointer(v)
}

func isNilImpl(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// extractOrWrapRawPointer returns the raw pointer for v, calling wrap if needed.
func extractOrWrapRawPointer(v any, wrap func() any) unsafe.Pointer {
	return core.ExtractOrWrapRawPointer(v, wrap)
}

// decodeSlice converts a raw pointer and count into a Go slice of T.
func decodeSlice[T any](ptr uintptr, count int) []T {
	return core.DecodeSlice[T](ptr, count)
}

// ---------------------------------------------------------------------------
// User-facing Client with safe handler types
// ---------------------------------------------------------------------------

// Client is the user-facing client interface. It differs from RawClient in two ways:
//   - GetAudioHandler() returns cef.AudioHandler (decoded [][]float32) instead of RawAudioHandler
//   - GetLifeSpanHandler() returns cef.LifeSpanHandler (typed out-params) instead of RawLifeSpanHandler
//
// Use NewClient to create a raw CEF client from a Client implementation.
type Client interface {
	GetAudioHandler() AudioHandler
	GetCommandHandler() CommandHandler
	GetContextMenuHandler() ContextMenuHandler
	GetDialogHandler() DialogHandler
	GetDisplayHandler() DisplayHandler
	GetDownloadHandler() DownloadHandler
	GetDragHandler() DragHandler
	GetFindHandler() FindHandler
	GetFocusHandler() FocusHandler
	GetFrameHandler() FrameHandler
	GetPermissionHandler() PermissionHandler
	GetJsdialogHandler() JsdialogHandler
	GetKeyboardHandler() KeyboardHandler
	GetLifeSpanHandler() LifeSpanHandler
	GetLoadHandler() LoadHandler
	GetPrintHandler() PrintHandler
	GetRenderHandler() RenderHandler
	GetRequestHandler() RequestHandler
	OnProcessMessageReceived(browser Browser, frame Frame, sourceProcess ProcessID, message ProcessMessage) int32
}

// clientAdapter wraps a Client to satisfy RawClient by converting safe handler
// types to their raw equivalents.
type clientAdapter struct {
	impl Client
}

// GetAudioHandler wraps the safe audio handler on demand. Callers that may
// invoke this repeatedly are expected to cache the returned raw wrapper, as
// generated NewRawClient code already does, because clientAdapter does not
// memoize it itself.
func (a *clientAdapter) GetAudioHandler() RawAudioHandler {
	h := a.impl.GetAudioHandler()
	if h == nil {
		return nil
	}
	return NewAudioHandler(h)
}

// GetLifeSpanHandler wraps the safe life span handler on demand. Callers that
// may invoke this repeatedly are expected to cache the returned raw wrapper, as
// generated NewRawClient code already does, because clientAdapter does not
// memoize it itself.
func (a *clientAdapter) GetLifeSpanHandler() RawLifeSpanHandler {
	h := a.impl.GetLifeSpanHandler()
	if h == nil {
		return nil
	}
	return NewLifeSpanHandler(h)
}

func (a *clientAdapter) GetCommandHandler() CommandHandler {
	return a.impl.GetCommandHandler()
}
func (a *clientAdapter) GetContextMenuHandler() ContextMenuHandler {
	return a.impl.GetContextMenuHandler()
}
func (a *clientAdapter) GetDialogHandler() DialogHandler {
	return a.impl.GetDialogHandler()
}
func (a *clientAdapter) GetDisplayHandler() DisplayHandler {
	return a.impl.GetDisplayHandler()
}
func (a *clientAdapter) GetDownloadHandler() DownloadHandler {
	return a.impl.GetDownloadHandler()
}
func (a *clientAdapter) GetDragHandler() DragHandler {
	return a.impl.GetDragHandler()
}
func (a *clientAdapter) GetFindHandler() FindHandler {
	return a.impl.GetFindHandler()
}
func (a *clientAdapter) GetFocusHandler() FocusHandler {
	return a.impl.GetFocusHandler()
}
func (a *clientAdapter) GetFrameHandler() FrameHandler {
	return a.impl.GetFrameHandler()
}
func (a *clientAdapter) GetPermissionHandler() PermissionHandler {
	return a.impl.GetPermissionHandler()
}
func (a *clientAdapter) GetJsdialogHandler() JsdialogHandler {
	return a.impl.GetJsdialogHandler()
}
func (a *clientAdapter) GetKeyboardHandler() KeyboardHandler {
	return a.impl.GetKeyboardHandler()
}
func (a *clientAdapter) GetLoadHandler() LoadHandler {
	return a.impl.GetLoadHandler()
}
func (a *clientAdapter) GetPrintHandler() PrintHandler {
	return a.impl.GetPrintHandler()
}
func (a *clientAdapter) GetRenderHandler() RenderHandler {
	return a.impl.GetRenderHandler()
}
func (a *clientAdapter) GetRequestHandler() RequestHandler {
	return a.impl.GetRequestHandler()
}
func (a *clientAdapter) OnProcessMessageReceived(browser Browser, frame Frame, sourceProcess ProcessID, message ProcessMessage) int32 {
	return a.impl.OnProcessMessageReceived(browser, frame, sourceProcess, message)
}

// NewClient creates a raw CEF client from a user-facing Client implementation.
// It wraps AudioHandler via NewAudioHandler ([][]float32 decoding) and
// LifeSpanHandler via NewLifeSpanHandler (typed out-params), then delegates
// to the generated raw client constructor.
func NewClient(impl Client) RawClient {
	if isNilImpl(impl) {
		return nil
	}
	return NewRawClient(&clientAdapter{impl: impl})
}
